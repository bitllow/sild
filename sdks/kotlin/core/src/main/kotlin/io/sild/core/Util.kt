package io.sild.core

import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.jsonPrimitive
import java.text.SimpleDateFormat
import java.util.Date
import java.util.Locale
import java.util.TimeZone

// clock formats an ISO-8601 timestamp as 12-hour "h:mm AM/PM" in the device's
// local time, matching the web client's clock() (which reads local hours off a
// Date). Backend timestamps carry an explicit offset (usually UTC "Z"); we parse
// them to an instant and format in the default zone. Uses java.util (API 1) rather
// than java.time so consumers on minSdk < 26 need no core-library desugaring.
// Empty string on null/blank/unparseable input.
internal fun clock(iso: String?): String {
    val instant = parseIso(iso) ?: return ""
    // SimpleDateFormat isn't thread-safe, so build per call (clock runs at message-
    // mapping time, not in a tight loop). No explicit zone → formats in the device zone.
    return SimpleDateFormat("h:mm a", Locale.US).format(instant)
}

// parseIso reads an RFC 3339 timestamp into an instant. SimpleDateFormat can't handle
// variable-length fractional seconds or a "Z" zone via one pattern, so normalize first:
// drop the fraction and express the offset in RFC 822 form (+0000) for the "Z" pattern.
private fun parseIso(iso: String?): Date? {
    if (iso.isNullOrBlank()) return null
    val s = iso.trim()
        .replace(Regex("\\.\\d+"), "")                    // strip ".123456789"
        .replace(Regex("Z$"), "+0000")                    // Z -> +0000
        .replace(Regex("([+-]\\d{2}):(\\d{2})$"), "$1$2") // +02:00 -> +0200
    val fmt = SimpleDateFormat("yyyy-MM-dd'T'HH:mm:ssZ", Locale.US).apply {
        timeZone = TimeZone.getTimeZone("UTC")
        isLenient = false
    }
    return runCatching { fmt.parse(s) }.getOrNull()
}

/** Read a member metadata object's "name" field, or null. */
internal fun JsonObject?.name(): String? =
    this?.get("name")?.let { runCatching { it.jsonPrimitive.content }.getOrNull() }

// rebaseLocalUrl re-bases a server URL that points at local-dev object storage
// (contains "/v1/uploads/local/") onto [base], so images/files the backend hands
// back (brand logo, message attachments) are reachable from wherever the SDK runs
// — e.g. an emulator, where the server's "localhost:8080" is actually 10.0.2.2.
// Real cloud URLs (no marker) pass through unchanged. This is the incoming-URL
// counterpart of the rewrite SildApi.upload already applies to signed PUT URLs.
internal fun rebaseLocalUrl(base: String, url: String?): String? {
    if (url == null) return null
    val marker = "/v1/uploads/local/"
    val i = url.indexOf(marker)
    return if (i >= 0) base + url.substring(i) else url
}
