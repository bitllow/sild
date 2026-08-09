package io.sild.core

import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFalse
import kotlin.test.assertTrue

private const val TITLE = "widget.home.title"

// A scripted backend: counts what was asked for, so a test can assert that a held
// validator stopped a download rather than only that the text is right.
private class FakeTranslations(
    var manifest: TranslationManifest? = null,
    var bundles: Map<Pair<String, Int>, Map<String, String>> = emptyMap(),
    var etag: String = "v1",
    var fail: Boolean = false,
) : TranslationSource {
    var manifestReads = 0
    var bundleReads = 0
    var lastEtag: String? = null

    override suspend fun fetchTranslationManifest(project: String, etag: String?): ManifestRead {
        manifestReads++
        lastEtag = etag
        if (fail) throw SildApiException(500, "boom")
        if (etag == this.etag) return ManifestRead(null, this.etag)
        return ManifestRead(manifest ?: TranslationManifest(project = project), this.etag)
    }

    override suspend fun fetchTranslationBundle(project: String, locale: String, version: Int): TranslationBundle {
        bundleReads++
        if (fail) throw SildApiException(500, "boom")
        return TranslationBundle(
            project = project,
            locale = locale,
            version = version,
            strings = bundles[locale to version] ?: emptyMap(),
        )
    }
}

class SildI18nTest {
    // A store per test, so one test's download is not another's starting text.
    private val downloads = I18nDownloads()

    private fun i18n(
        source: TranslationSource? = null,
        explicit: String? = null,
        prefs: List<String> = listOf("en"),
        now: () -> Long = { 0 },
    ) = SildI18n(source, explicit, prefs, PLATFORM_PROJECT, now, downloads)

    @Test fun rendersBundledTextBeforeAnyNetworkCall() {
        assertEquals("Chat with us", i18n().t(TITLE))
    }

    @Test fun theDevicePreferenceChoosesTheLanguage() {
        val lv = i18n(prefs = listOf("lv-LV", "en"))
        assertEquals("lv", lv.locale)
        assertEquals("Sazinieties ar mums", lv.t(TITLE))
    }

    @Test fun anExplicitLocaleBeatsTheDevice() {
        assertEquals("ru", i18n(explicit = "ru", prefs = listOf("lv")).locale)
    }

    // A tenant may add a language Sild ships no text for. The host's choice stands,
    // and until that language's bundle arrives the source shows — never a raw key.
    @Test fun aLanguageSildDoesNotShipIsStillHonoured() {
        val fi = i18n(explicit = "fi", prefs = listOf("fi"))
        assertEquals("fi", fi.locale)
        assertEquals("Chat with us", fi.t(TITLE))
    }

    @Test fun aTenantAddedLanguageRendersOnceItIsPublished() = runBlockingTest {
        val src = FakeTranslations(
            manifest = TranslationManifest(locales = mapOf("fi" to 4)),
            bundles = mapOf(("fi" to 4) to mapOf(TITLE to "Juttele kanssamme")),
        )
        val i = i18n(src, explicit = "fi")
        i.refresh()
        assertEquals("Juttele kanssamme", i.t(TITLE))
    }

    // No explicit choice: the device asked for a language Sild does not bundle, and
    // the tenant publishes it, so it is on offer after all.
    @Test fun aDeviceIsAdoptedIntoATenantAddedLanguage() = runBlockingTest {
        val src = FakeTranslations(
            manifest = TranslationManifest(locales = mapOf("fi" to 4)),
            bundles = mapOf(("fi" to 4) to mapOf(TITLE to "Juttele kanssamme")),
        )
        val i = i18n(src, prefs = listOf("fi"))
        assertEquals("en", i.locale, "nothing bundled matches, so it starts on the source")
        i.refresh()
        assertEquals("fi", i.locale)
        assertEquals("Juttele kanssamme", i.t(TITLE))
    }

    @Test fun placeholdersAreFilledFromTheirNames() {
        assertEquals("Direct chat · trip_9021", i18n().t("widget.thread.directRef", mapOf("ref" to "trip_9021")))
    }

    @Test fun anUnknownPlaceholderIsLeftAloneRatherThanBlanked() {
        assertEquals("Direct chat · {ref}", i18n().t("widget.thread.directRef", mapOf("other" to "x")))
    }

    @Test fun aDownloadedBundleWaitsForTheNextStart() = runBlockingTest {
        val src = FakeTranslations(
            manifest = TranslationManifest(locales = mapOf("en" to 3)),
            bundles = mapOf(("en" to 3) to mapOf(TITLE to "Talk to us")),
        )
        val i = i18n(src)
        i.refreshStaged()
        assertEquals("Chat with us", i.t(TITLE), "a background fetch must not change text mid-task")
        assertTrue(i.activateStaged())
        assertEquals("Talk to us", i.t(TITLE))
        assertFalse(i.activateStaged(), "there is nothing left to activate")
    }

    // Closing and reopening the messenger builds a new client. What the last one
    // downloaded is this one's starting text — otherwise nothing ever activates
    // "at next start", because the runtime that fetched it is gone.
    @Test fun aNewSessionStartsOnWhatTheLastOneDownloaded() = runBlockingTest {
        val src = FakeTranslations(
            manifest = TranslationManifest(locales = mapOf("es" to 7)),
            bundles = mapOf(("es" to 7) to mapOf(TITLE to "Hablemos")),
        )
        i18n(src, explicit = "es").refreshStaged()

        val next = i18n(explicit = "es")
        assertEquals("Hablemos", next.t(TITLE))
    }

    @Test fun refreshAppliesImmediatelyForAHostThatAsks() = runBlockingTest {
        val src = FakeTranslations(
            manifest = TranslationManifest(locales = mapOf("en" to 3)),
            bundles = mapOf(("en" to 3) to mapOf(TITLE to "Talk to us")),
        )
        val i = i18n(src)
        i.refresh()
        assertEquals("Talk to us", i.t(TITLE))
    }

    @Test fun anUnchangedVersionCostsOneConditionalRequestAndNoBundle() = runBlockingTest {
        val src = FakeTranslations(
            manifest = TranslationManifest(locales = mapOf("en" to 3)),
            bundles = mapOf(("en" to 3) to mapOf(TITLE to "Talk to us")),
        )
        val i = i18n(src)
        i.refresh()
        assertEquals(1, src.bundleReads)
        i.refresh()
        assertEquals("v1", src.lastEtag, "the held validator must be offered back")
        assertEquals(1, src.bundleReads, "a 304 must not re-download the bundle")
        assertEquals("Talk to us", i.t(TITLE))
    }

    @Test fun aHeldManifestIsNotRePolledUntilItAgesOut() = runBlockingTest {
        val src = FakeTranslations(manifest = TranslationManifest(locales = mapOf("en" to 1)))
        var clock = 0L
        val i = i18n(src, now = { clock })
        i.refreshStagedIfStale()
        assertEquals(1, src.manifestReads)
        i.refreshStagedIfStale()
        assertEquals(1, src.manifestReads, "a fresh manifest must not be re-polled")
        clock = MANIFEST_TTL_MS + 1
        i.refreshStagedIfStale()
        assertEquals(2, src.manifestReads)
    }

    @Test fun aFailedFetchLeavesTheTextOnScreen() = runBlockingTest {
        val src = FakeTranslations(fail = true)
        val i = i18n(src)
        i.refresh()
        assertEquals("Chat with us", i.t(TITLE))
    }

    @Test fun nothingPublishedLeavesTheBundledDefaults() = runBlockingTest {
        val i = i18n(FakeTranslations(manifest = TranslationManifest(locales = emptyMap())))
        i.refresh()
        assertEquals("Chat with us", i.t(TITLE))
    }

    @Test fun settingTheLanguageSwitchesToItsBundledText() {
        val i = i18n()
        i.setLocale("lv")
        assertEquals("lv", i.locale)
        assertEquals("Sazinieties ar mums", i.t(TITLE))
    }

    // The published text belongs to the language it was fetched for; switching away
    // must not leave the previous language's strings on screen.
    @Test fun switchingLanguageDropsThePreviousOnesFetchedText() = runBlockingTest {
        val src = FakeTranslations(
            manifest = TranslationManifest(locales = mapOf("en" to 3)),
            bundles = mapOf(("en" to 3) to mapOf(TITLE to "Talk to us")),
        )
        val i = i18n(src)
        i.refresh()
        assertEquals("Talk to us", i.t(TITLE))
        i.setLocale("lv")
        assertEquals("Sazinieties ar mums", i.t(TITLE))
    }

    // The tenant decides which languages exist; a device guess made against the
    // bundled catalog is re-negotiated once the manifest says what is on offer.
    @Test fun theTenantsOfferingNarrowsADeviceGuess() = runBlockingTest {
        val src = FakeTranslations(
            manifest = TranslationManifest(fallback_locale = "lv", locales = mapOf("lv" to 2)),
            bundles = mapOf(("lv" to 2) to mapOf(TITLE to "Runā ar mums")),
        )
        val i = i18n(src, prefs = listOf("ru"))
        assertEquals("ru", i.locale)
        i.refresh()
        assertEquals("lv", i.locale)
        assertEquals("Runā ar mums", i.t(TITLE))
    }

    @Test fun anExplicitLocaleSurvivesTheTenantsOffering() = runBlockingTest {
        val src = FakeTranslations(manifest = TranslationManifest(fallback_locale = "lv", locales = mapOf("lv" to 2)))
        val i = i18n(src, explicit = "ru")
        i.refresh()
        assertEquals("ru", i.locale, "a host that named the language must be obeyed")
    }

    @Test fun regionalTagsCollapseToTheirLanguage() {
        assertEquals("en", normalizeLocale("en-GB"))
        assertEquals("pt", normalizeLocale("pt_BR"))
        assertEquals("", normalizeLocale(null))
    }

    @Test fun negotiationTakesTheFirstOfferedPreference() {
        assertEquals("lv", negotiateLocale(listOf("fi", "lv-LV", "en"), listOf("en", "lv")))
        assertEquals("", negotiateLocale(listOf("fi"), listOf("en", "lv")))
    }
}
