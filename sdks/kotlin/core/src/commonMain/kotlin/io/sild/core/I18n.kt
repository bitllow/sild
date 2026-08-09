package io.sild.core

import kotlin.concurrent.Volatile
import kotlin.time.TimeSource
import kotlinx.serialization.Serializable

/** The reserved project Sild's own strings live in. */
const val PLATFORM_PROJECT: String = "sild"

/** How long a held manifest is trusted before a foreground re-poll. */
internal const val MANIFEST_TTL_MS = 60L * 60 * 1000

/** GET /v1/translations/manifest → the published version per locale. */
@Serializable
data class TranslationManifest(
    val project: String = "",
    val fallback_locale: String = "",
    val locales: Map<String, Int> = emptyMap(),
)

/** GET /v1/translations/bundle → one published locale in full. */
@Serializable
data class TranslationBundle(
    val project: String = "",
    val locale: String = "",
    val version: Int = 0,
    val strings: Map<String, String> = emptyMap(),
)

/** A manifest read and the validator that revalidates it; null body means 304. */
internal class ManifestRead(val manifest: TranslationManifest?, val etag: String)

// Downloaded bundles live for the process, not the session. A messenger closed and
// reopened builds a new client, and without this its predecessor's download would
// be thrown away — "activates at next start" would never activate anything.
internal class I18nDownloads {
    private val byLocale = mutableMapOf<String, Pair<Int, Map<String, String>>>()

    fun get(project: String, locale: String): Pair<Int, Map<String, String>>? =
        byLocale["$project\n$locale"]

    fun put(project: String, locale: String, ready: Pair<Int, Map<String, String>>) {
        byLocale["$project\n$locale"] = ready
    }

    /** The process-wide one, which every client built from a config shares. */
    companion object {
        val shared = I18nDownloads()
    }
}

/** The calls the i18n runtime needs — implemented by SildApi. */
internal interface TranslationSource {
    suspend fun fetchTranslationManifest(project: String, etag: String?): ManifestRead
    suspend fun fetchTranslationBundle(project: String, locale: String, version: Int): TranslationBundle
}

// Sild ships flat languages, so "en-GB" and "en" are one locale.
fun normalizeLocale(tag: String?): String {
    val t = (tag ?: "").trim()
    val i = t.indexOfFirst { it == '-' || it == '_' }
    return (if (i > 0) t.substring(0, i) else t).lowercase()
}

/** The caller's most preferred locale that is actually offered, or "" if none. */
fun negotiateLocale(prefs: List<String>, offered: List<String>): String {
    val have = offered.map { normalizeLocale(it) }.toSet()
    for (p in prefs) {
        val n = normalizeLocale(p)
        if (n in have) return n
    }
    return ""
}

// Both braces escaped: Android's regex engine rejects a bare `}`, and a pattern
// that only the desktop JVM accepts fails at class-init time on a device.
private val PLACEHOLDER = Regex("""\{(\w+)\}""")

internal fun interpolate(text: String, vars: Map<String, Any>?): String {
    if (vars.isNullOrEmpty()) return text
    return PLACEHOLDER.replace(text) { m ->
        val name = m.groupValues[1]
        vars[name]?.toString() ?: m.value
    }
}

// SildI18n is the SDK's active language and strings, the Kotlin peer of the web
// widget's I18n. It renders from the bundled defaults at once — first paint never
// waits on the network, and a failed fetch leaves the text already on screen.
//
// A downloaded bundle is STAGED rather than applied, and takes effect at the next
// start() in this process; an app that wants it sooner calls refresh(). Text does
// not change under the user's finger unless the host asked for it. Nothing is held
// across a process restart, so a cold start renders the bundled defaults again.
class SildI18n internal constructor(
    private val source: TranslationSource?,
    explicitLocale: String? = null,
    private val devicePrefs: List<String> = deviceLocales(),
    private val project: String = PLATFORM_PROJECT,
    private val now: () -> Long = ::elapsedMillis,
    private val downloads: I18nDownloads = I18nDownloads.shared,
) {
    /** The language being rendered. */
    @Volatile
    var locale: String = ""
        private set

    // True when the host named the locale: an instruction, not a guess, so what the
    // tenant offers never overrides it.
    private val explicit: Boolean

    @Volatile
    private var strings: Map<String, String> = emptyMap()

    @Volatile
    private var version: Int = 0

    @Volatile
    private var staged: Pair<Int, Map<String, String>>? = null

    @Volatile
    private var heldEtag: String? = null

    @Volatile
    private var heldAt: Long = 0

    @Volatile
    private var heldManifest: TranslationManifest? = null

    init {
        val named = explicitLocale?.let { normalizeLocale(it) } ?: ""
        // A named language is honoured even when Sild ships no bundled text for it:
        // the tenant may have added it, and its published bundle is what renders.
        // Until that arrives the source language shows, never a raw key.
        explicit = named.isNotEmpty()
        locale = (if (explicit) named else negotiateLocale(devicePrefs, I18N_LOCALES))
            .ifEmpty { I18N_SOURCE_LOCALE }
        adoptDownloaded()
    }

    // What an earlier session in this process fetched is this session's starting
    // text — the "activates at next SDK start" half of the delivery contract.
    private fun adoptDownloaded() {
        val ready = downloads.get(project, locale) ?: return
        version = ready.first
        strings = ready.second
    }

    /** The text for [key] in the active language, with `{name}` placeholders filled. */
    fun t(key: String, vars: Map<String, Any>? = null): String {
        val text = strings[key]
            ?: I18N_DEFAULTS[locale]?.get(key)
            ?: I18N_DEFAULTS[I18N_SOURCE_LOCALE]?.get(key)
            ?: key
        return interpolate(text, vars)
    }

    /** Render the language the host asks for, from this call on. Unknown tags are
     *  ignored, so a host may pass the device's setting through unchecked. */
    fun setLocale(tag: String) {
        val next = normalizeLocale(tag)
        if (next.isEmpty() || next == locale) return
        locale = next
        // The staged bundle was another language's; the bundled defaults are this one's.
        staged = null
        strings = emptyMap()
        version = 0
        adoptDownloaded()
    }

    /** Fetch the published bundle and apply it now. Never throws: an offline or
     *  failed fetch leaves the text already on screen. */
    suspend fun refresh() {
        val ready = download() ?: return
        version = ready.first
        strings = ready.second
        staged = null
    }

    /** Fetch in the background and hold the result for the next [activateStaged]. */
    suspend fun refreshStaged() {
        staged = download() ?: return
    }

    /** refreshStaged() only once the held manifest has aged out — the foreground poll. */
    suspend fun refreshStagedIfStale() {
        if (heldEtag != null && now() - heldAt < MANIFEST_TTL_MS) return
        refreshStaged()
    }

    /** Apply what a background fetch downloaded. Called at start, so new text
     *  appears between sessions rather than mid-task. Reports whether it changed. */
    fun activateStaged(): Boolean {
        val ready = staged ?: return false
        staged = null
        if (ready.first == version) return false
        version = ready.first
        strings = ready.second
        return true
    }

    // download returns the published bundle for the active locale, or null when
    // there is nothing new, nothing published, or the network is gone.
    private suspend fun download(): Pair<Int, Map<String, String>>? = runCatching {
        val src = source ?: return null
        val read = src.fetchTranslationManifest(project, heldEtag)
        val manifest = read.manifest ?: heldManifest ?: return null
        heldEtag = read.etag.ifEmpty { heldEtag }
        heldAt = now()
        heldManifest = manifest
        adopt(manifest)
        val v = manifest.locales[locale] ?: return null
        if (v == version) return null
        val ready = v to src.fetchTranslationBundle(project, locale, v).strings
        downloads.put(project, locale, ready)
        ready
    }.getOrNull()

    // Which locales the tenant offers is unknowable until the manifest arrives, so a
    // guess negotiated against the bundled catalog is re-negotiated against the offering.
    private fun adopt(manifest: TranslationManifest) {
        if (explicit) return
        // Whatever the tenant publishes is on offer, including a language Sild
        // ships no bundled text for — its bundle carries every key.
        val offered = manifest.locales.keys.toList()
        if (offered.isEmpty() || locale in offered) return
        val next = negotiateLocale(devicePrefs, offered)
            .ifEmpty { normalizeLocale(manifest.fallback_locale) }
        if (next !in offered) return
        locale = next
        strings = emptyMap()
        version = 0
        adoptDownloaded()
    }
}

/** Sild's bundled strings with no backend behind them — for a surface that renders
 *  before (or without) a client, such as a preview or a launcher. */
fun bundledStrings(locale: String? = null): SildI18n = SildI18n(source = null, explicitLocale = locale)

/** The device's preferred languages, most preferred first. */
internal expect fun deviceLocales(): List<String>

// Elapsed rather than wall-clock: the manifest TTL only ever measures a gap, and a
// device whose clock jumps must not be able to freeze the poll.
private val processStart = TimeSource.Monotonic.markNow()

internal fun elapsedMillis(): Long = processStart.elapsedNow().inWholeMilliseconds
