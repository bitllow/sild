package io.sild.core

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
private fun tokenFor(tenant: String): String =
    "header.${base64Url("{\"tid\":\"$tenant\",\"sub\":\"u_1\"}")}.signature"

private fun base64Url(s: String): String {
    val alphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-_"
    val bytes = s.encodeToByteArray()
    val out = StringBuilder()
    var i = 0
    while (i < bytes.size) {
        val b0 = bytes[i].toInt() and 0xFF
        val b1 = if (i + 1 < bytes.size) bytes[i + 1].toInt() and 0xFF else -1
        val b2 = if (i + 2 < bytes.size) bytes[i + 2].toInt() and 0xFF else -1
        out.append(alphabet[b0 shr 2])
        out.append(alphabet[((b0 and 0x03) shl 4) or (if (b1 >= 0) b1 shr 4 else 0)])
        if (b1 >= 0) out.append(alphabet[((b1 and 0x0F) shl 2) or (if (b2 >= 0) b2 shr 6 else 0)])
        if (b2 >= 0) out.append(alphabet[b2 and 0x3F])
        i += 3
    }
    return out.toString()
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

    // A bundle downloaded before the token landed is nobody's to keep: written to the
    // store it would be read by whichever tenant came next.
    @Test
    fun anUnidentifiedTenantIsNotPersisted() {
        val store = MemoryStore()
        I18nDownloads(store).put("", PLATFORM_PROJECT, "lv", HeldBundle(1, mapOf("a" to "b")))
        assertTrue(store.slots.isEmpty(), "an unnamed tenant's bundle reached the store: ${store.slots.keys}")
    }
}
