package io.sild.core

import kotlin.io.encoding.Base64
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.jsonPrimitive

/**
 * Where a client keeps the strings it downloaded, so a cold start renders the text
 * the tenant published rather than the defaults bundled in the app.
 *
 * iOS has one by default (NSUserDefaults). On the JVM there is no ambient place to
 * write — an Android app's is behind a Context — so `Sild.init(context, config)` in
 * `:ui` supplies it, and a host that does neither keeps text for the process only.
 */
interface SildStringStore {
    fun read(key: String): String?
    fun write(key: String, value: String)
}

internal expect fun platformStringStore(): SildStringStore?

/** Install where downloaded strings live, for a platform that has no ambient place
 *  :core can reach. Android's is behind a Context, so `Sild.init(context, config)`
 *  calls this; iOS needs nothing. */
fun installStringStore(store: SildStringStore) = I18nDownloads.install(store)

// tenantOf reads the tenant out of the user JWT the host's token provider minted.
// Not verification — the server does that — only the discriminator a cache needs:
// two tenants in one process must not read each other's wording.
internal fun tenantOf(token: String): String {
    val payload = token.split(".").getOrNull(1) ?: return ""
    return runCatching {
        val json = Base64.UrlSafe.withPadding(Base64.PaddingOption.ABSENT_OPTIONAL).decode(payload)
        Json.parseToJsonElement(json.decodeToString()).jsonObject["tid"]?.jsonPrimitive?.content ?: ""
    }.getOrDefault("")
}
