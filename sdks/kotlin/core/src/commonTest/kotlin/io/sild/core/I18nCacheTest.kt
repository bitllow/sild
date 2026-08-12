package io.sild.core

import kotlin.io.encoding.Base64
import kotlin.io.encoding.ExperimentalEncodingApi
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertNull
import kotlin.test.assertTrue

// A store that lives in the test, standing in for NSUserDefaults or SharedPreferences.
private class MemoryStore : SildStringStore {
    val slots = mutableMapOf<String, String>()
    override fun read(key: String): String? = slots[key]
    override fun write(key: String, value: String) {
        slots[key] = value
    }
}

// A JWT is three dot-separated parts; only the payload matters here, and only its
// tenant claim. Unsigned: the server verifies, this does not.
@OptIn(ExperimentalEncodingApi::class)
private fun tokenFor(tenant: String): String {
    val payload = "{\"tid\":\"$tenant\",\"sub\":\"u_1\"}".encodeToByteArray()
    return "header.${Base64.UrlSafe.encode(payload)}.signature"
}

class I18nCacheTest {
    @Test
    fun theTenantIsReadOffTheToken() {
        assertEquals("t_acme", tenantOf(tokenFor("t_acme")))
        assertEquals("", tenantOf("not-a-jwt"))
        assertEquals("", tenantOf(""))
    }

    // Two tenants in one process render their own wording, not whichever downloaded
    // first — a messenger embedding two Sild tenants used to see one bleed through.
    @Test
    fun twoTenantsDoNotShareABundle() {
        val downloads = I18nDownloads(MemoryStore())
        downloads.put("t_acme", PLATFORM_PROJECT, "lv", HeldBundle(1, mapOf("widget.home.cta" to "Acme")))
        downloads.put("t_other", PLATFORM_PROJECT, "lv", HeldBundle(1, mapOf("widget.home.cta" to "Other")))

        val acme = SildI18n(source = null, explicitLocale = "lv", downloads = downloads)
        acme.adopt("t_acme")
        assertEquals("Acme", acme.t("widget.home.cta"))

        val other = SildI18n(source = null, explicitLocale = "lv", downloads = downloads)
        other.adopt("t_other")
        assertEquals("Other", other.t("widget.home.cta"))
    }

    // A killed app keeps what it downloaded: the store is what the next cold start
    // reads, so the tenant's published text survives a restart.
    @Test
    fun aColdStartReadsWhatTheLastOneKept() {
        val store = MemoryStore()
        I18nDownloads(store).put("t_acme", PLATFORM_PROJECT, "lv", HeldBundle(3, mapOf("widget.home.cta" to "Kept")))

        val fresh = SildI18n(source = null, explicitLocale = "lv", downloads = I18nDownloads(store))
        assertTrue(fresh.t("widget.home.cta") != "Kept", "adopted before the tenant was named")
        fresh.adopt("t_acme")
        assertEquals("Kept", fresh.t("widget.home.cta"))
    }

    // Without a store the text lives for the process, which is what a JVM host that
    // supplied none gets — and it must still not leak across tenants.
    @Test
    fun withNoStoreTheBundleLivesForTheProcess() {
        val downloads = I18nDownloads(null)
        downloads.put("t_acme", PLATFORM_PROJECT, "lv", HeldBundle(1, mapOf("widget.home.cta" to "Acme")))
        assertEquals("Acme", downloads.get("t_acme", PLATFORM_PROJECT, "lv")?.strings?.get("widget.home.cta"))
        assertNull(downloads.get("t_other", PLATFORM_PROJECT, "lv"))
    }

    // The pointer and the bundles are one slot, so a pointer can never name a bundle
    // that is not there.
    @Test
    fun whatIsKeptIsOneSlotPerTenantAndProject() {
        val store = MemoryStore()
        val downloads = I18nDownloads(store)
        downloads.put("t_acme", PLATFORM_PROJECT, "lv", HeldBundle(1, mapOf("a" to "b")))
        downloads.put("t_acme", PLATFORM_PROJECT, "et", HeldBundle(2, mapOf("a" to "c")))
        assertEquals(1, store.slots.size, "two locales took ${store.slots.size} slots: ${store.slots.keys}")
        assertEquals("et", downloads.settledLocale("t_acme", PLATFORM_PROJECT))

        val fresh = I18nDownloads(store)
        assertEquals("b", fresh.get("t_acme", PLATFORM_PROJECT, "lv")?.strings?.get("a"))
        assertEquals("c", fresh.get("t_acme", PLATFORM_PROJECT, "et")?.strings?.get("a"))
    }

    // A device asking for a language the tenant does not publish negotiates to one
    // they do; the next cold start has to adopt that, not guess from the device again.
    @Test
    fun aColdStartAdoptsTheLanguageTheTenantSettledOn() {
        val store = MemoryStore()
        I18nDownloads(store).put("t_acme", PLATFORM_PROJECT, "lv", HeldBundle(2, mapOf("widget.home.cta" to "Latviski")))

        // "fi" is what the device asks for and what Sild ships no text for.
        val fresh = SildI18n(source = null, devicePrefs = listOf("fi"), downloads = I18nDownloads(store))
        assertTrue(fresh.adopt("t_acme"), "adopting a kept bundle is a change worth publishing")
        assertEquals("lv", fresh.locale)
        assertEquals("Latviski", fresh.t("widget.home.cta"))
    }

    // A host that named the language is giving an instruction, not a preference: a
    // remembered negotiation must not override it.
    @Test
    fun anExplicitLocaleOutranksWhatWasSettled() {
        val store = MemoryStore()
        I18nDownloads(store).put("t_acme", PLATFORM_PROJECT, "lv", HeldBundle(2, mapOf("widget.home.cta" to "Latviski")))

        val named = SildI18n(source = null, explicitLocale = "et", downloads = I18nDownloads(store))
        named.adopt("t_acme")
        assertEquals("et", named.locale)
    }

    // A bundle downloaded before the token landed is nobody's to keep: written to the
    // store it would be read by whichever tenant came next.
    @Test
    fun anUnidentifiedTenantIsNotPersisted() {
        val store = MemoryStore()
        I18nDownloads(store).put("", PLATFORM_PROJECT, "lv", HeldBundle(1, mapOf("a" to "b")))
        assertTrue(store.slots.isEmpty(), "an unnamed tenant's bundle reached the store: ${store.slots.keys}")
    }
}
