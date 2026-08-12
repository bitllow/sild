package io.sild.core

import kotlin.concurrent.Volatile
import kotlin.time.TimeSource
import kotlinx.serialization.Serializable
import kotlinx.serialization.json.Json

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
    /** The SDK line Sild's own key set belongs to, so a host can tell what the
     *  bundles it is serving target. Empty for a tenant's own project. */
    val sdk_version: String = "",
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

/** A bundle as it is kept between sessions. */
@Serializable
internal data class HeldBundle(val version: Int, val strings: Map<String, String>)

// Downloaded bundles outlive the session: a messenger closed and reopened builds a
// new client, and without this its predecessor's download would be thrown away, so
// "activates at next start" would never activate anything. Keyed by TENANT as well
// as project and locale — two tenants in one process must not read each other's
// wording — and written through to a platform store where there is one, so the next
// cold start has it too.
internal class I18nDownloads(internal val store: SildStringStore? = null) {
    // Replaced whole rather than mutated: this is read on the main thread and
    // written from whichever dispatcher the fetch ran on.
    @Volatile
    private var byLocale: Map<String, HeldBundle> = emptyMap()

    fun get(tenant: String, project: String, locale: String): HeldBundle? {
        val key = slot(tenant, project, locale)
        byLocale[key]?.let { return it }
        val raw = store?.read(key) ?: return null
        val held = runCatching { Json.decodeFromString<HeldBundle>(raw) }.getOrNull() ?: return null
        byLocale = byLocale + (key to held)
        return held
    }

    fun put(tenant: String, project: String, locale: String, held: HeldBundle) {
        val key = slot(tenant, project, locale)
        byLocale = byLocale + (key to held)
        // A tenant with no identity yet is held in memory only: a slot that cannot
        // name whose text it is would be read by the next tenant along.
        if (tenant.isEmpty()) return
        runCatching { store?.write(key, Json.encodeToString(held)) }
    }

    private fun slot(tenant: String, project: String, locale: String) =
        "sild_i18n_${tenant}_${project}_$locale"

    companion object {
        /** The process-wide one, which every client built from a config shares. */
        val shared = I18nDownloads(platformStringStore())

        // One per store, so two clients sharing a config share their downloads and a
        // host that supplied its own place to write is honoured.
        private val byStore = mutableMapOf<SildStringStore, I18nDownloads>()

        fun forStore(store: SildStringStore?): I18nDownloads {
            if (store == null || store === shared.store) return shared
            return byStore.getOrPut(store) { I18nDownloads(store) }
        }
    }
}

/** A downloaded bundle and the language it belongs to. */
internal class ReadyBundle(val locale: String, val version: Int, val strings: Map<String, String>)

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

/** The category every language has, and the last readable text before giving up. */
private const val CAT_OTHER = "other"

/** Which categories a language has — Latvian's zero, Estonian's lack of one. */
fun pluralCategories(locale: String): List<String> {
    val family = I18N_PLURAL_LOCALES[normalizeLocale(locale)] ?: I18N_PLURAL_DEFAULT_FAMILY
    return I18N_PLURAL_FAMILIES[family] ?: I18N_PLURAL_FAMILIES.getValue(I18N_PLURAL_DEFAULT_FAMILY)
}

/** The plural category for a count. Integers only, per ADR 0004. The families here
 *  are the ones backend/internal/i18n/plurals.json names. */
fun pluralCategory(locale: String, count: Int): String {
    val n = if (count < 0) -count else count
    val m10 = n % 10
    val m100 = n % 100
    return when (I18N_PLURAL_LOCALES[normalizeLocale(locale)] ?: I18N_PLURAL_DEFAULT_FAMILY) {
        "other" -> CAT_OTHER
        "french" -> if (n <= 1) "one" else CAT_OTHER
        "czech" -> when {
            n == 1 -> "one"
            n in 2..4 -> "few"
            else -> CAT_OTHER
        }
        "romanian" -> when {
            n == 1 -> "one"
            n == 0 || m100 in 1..19 -> "few"
            else -> CAT_OTHER
        }
        "lithuanian" -> when {
            m100 in 11..19 -> CAT_OTHER
            m10 == 1 -> "one"
            m10 in 2..9 -> "few"
            else -> CAT_OTHER
        }
        "latvian" -> when {
            m10 == 0 || m100 in 11..19 -> "zero"
            m10 == 1 && m100 != 11 -> "one"
            else -> CAT_OTHER
        }
        "hebrew" -> when {
            n == 1 -> "one"
            n == 2 -> "two"
            else -> CAT_OTHER
        }
        "slovenian" -> when (m100) {
            1 -> "one"
            2 -> "two"
            3, 4 -> "few"
            else -> CAT_OTHER
        }
        "arabic" -> when {
            n == 0 -> "zero"
            n == 1 -> "one"
            n == 2 -> "two"
            m100 in 3..10 -> "few"
            m100 in 11..99 -> "many"
            else -> CAT_OTHER
        }
        "welsh" -> when (n) {
            0 -> "zero"
            1 -> "one"
            2 -> "two"
            3 -> "few"
            6 -> "many"
            else -> CAT_OTHER
        }
        "irish" -> when {
            n == 1 -> "one"
            n == 2 -> "two"
            n in 3..6 -> "few"
            n in 7..10 -> "many"
            else -> CAT_OTHER
        }
        "maltese" -> when {
            n == 1 -> "one"
            n == 0 || m100 in 2..10 -> "few"
            m100 in 11..19 -> "many"
            else -> CAT_OTHER
        }
        "polish" -> when {
            n == 1 -> "one"
            m10 in 2..4 && m100 !in 12..14 -> "few"
            else -> "many"
        }
        "slavic" -> when {
            m10 == 1 && m100 != 11 -> "one"
            m10 in 2..4 && m100 !in 12..14 -> "few"
            else -> "many"
        }
        else -> if (n == 1) "one" else CAT_OTHER
    }
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
    // Empty until the host's token names the tenant; a bundle keyed by nobody is
    // nobody's to render.
    private var tenant: String = "",
    // A key that resolves nowhere is a programming error: it is reported, and only
    // a debug build puts the key itself on screen.
    private val debug: Boolean = false,
    private val onMissingKey: ((String) -> Unit)? = null,
) {
    /** The language being rendered. */
    @Volatile
    var locale: String = ""
        private set

    // True when the host named the locale: an instruction, not a guess, so what the
    // tenant offers never overrides it. setLocale is that same instruction, later.
    @Volatile
    private var explicit: Boolean

    @Volatile
    private var strings: Map<String, String> = emptyMap()

    @Volatile
    private var version: Int = 0

    @Volatile
    private var staged: ReadyBundle? = null

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

    /** Name the tenant whose text this client renders, and take up what was kept for
     *  them. Called once the host's token has been minted — which is the "activates
     *  at next SDK start" half of the delivery contract. */
    internal fun adopt(tenant: String) {
        if (tenant.isEmpty() || tenant == this.tenant) return
        this.tenant = tenant
        adoptDownloaded()
    }

    // What an earlier session kept is this session's starting text.
    private fun adoptDownloaded() {
        val ready = downloads.get(tenant, project, locale) ?: return
        version = ready.version
        strings = ready.strings
    }

    /** The text for [key] in the active language, with `{name}` placeholders filled. */
    fun t(key: String, vars: Map<String, Any>? = null): String = interpolate(lookup(key), vars)

    /** The text for a count, from the category this language uses for it. `count`
     *  is available to the text as `{count}`. */
    fun tPlural(base: String, count: Int, vars: Map<String, Any>? = null): String {
        val category = pluralCategory(locale, count)
        val text = lookup("$base.$category", "$base.$CAT_OTHER")
        return interpolate(text, mapOf("count" to count) + (vars ?: emptyMap()))
    }

    // lookup walks the published bundle, then this language's bundled text, then the
    // source language's; alt is the plural's other form, where there is one.
    private fun lookup(key: String, alt: String? = null): String {
        val text = textFor(key) ?: alt?.let { textFor(it) }
        if (text != null) return text
        onMissingKey?.invoke(key)
        return if (debug) key else ""
    }

    private fun textFor(key: String): String? =
        strings[key]
            ?: I18N_DEFAULTS[locale]?.get(key)
            ?: I18N_DEFAULTS[I18N_SOURCE_LOCALE]?.get(key)

    /** Render the language the host asks for, from this call on. Unknown tags are
     *  ignored, so a host may pass the device's setting through unchecked. */
    fun setLocale(tag: String) {
        val next = normalizeLocale(tag)
        if (next.isEmpty()) return
        explicit = true
        if (next == locale) return
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
        locale = ready.locale
        version = ready.version
        strings = ready.strings
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
        if (ready.locale == locale && ready.version == version) return false
        locale = ready.locale
        version = ready.version
        strings = ready.strings
        return true
    }

    // download returns the published bundle to render next, or null when there is
    // nothing new, nothing published, or the network is gone. The language it carries
    // may not be the active one: a re-negotiation lands with the text, not before it.
    private suspend fun download(): ReadyBundle? = runCatching {
        val src = source ?: return null
        val at = locale
        val read = src.fetchTranslationManifest(project, heldEtag)
        val manifest = read.manifest ?: heldManifest ?: return null
        heldEtag = read.etag.ifEmpty { heldEtag }
        heldAt = now()
        heldManifest = manifest
        val want = negotiate(manifest)
        val v = manifest.locales[want] ?: return null
        if (want == locale && v == version) return null
        val strings = src.fetchTranslationBundle(project, want, v).strings
        downloads.put(tenant, project, want, HeldBundle(v, strings))
        // setLocale may have moved on while the bundle was in flight; it is held for
        // whoever asks for that language next, but it is not this language's text.
        if (at != locale) return null
        ReadyBundle(want, v, strings)
    }.getOrNull()

    // Which locales the tenant offers is unknowable until the manifest arrives, so a
    // guess negotiated against the bundled catalog is re-negotiated against the offering.
    private fun negotiate(manifest: TranslationManifest): String {
        if (explicit) return locale
        // Whatever the tenant publishes is on offer, including a language Sild
        // ships no bundled text for — its bundle carries every key.
        val offered = manifest.locales.keys.toList()
        if (offered.isEmpty() || locale in offered) return locale
        val next = negotiateLocale(devicePrefs, offered)
            .ifEmpty { normalizeLocale(manifest.fallback_locale) }
        return if (next in offered) next else locale
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
