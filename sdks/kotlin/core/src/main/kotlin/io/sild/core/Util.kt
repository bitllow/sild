package io.sild.core

import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.jsonPrimitive
import java.time.OffsetDateTime
import java.time.format.DateTimeFormatter
import java.util.Locale

// clock formats an ISO-8601 timestamp as 12-hour "h:mm AM/PM" in local time,
// matching the web client's clock(). Empty string on null/blank/unparseable input.
internal fun clock(iso: String?): String {
    if (iso.isNullOrBlank()) return ""
    val dt = runCatching { OffsetDateTime.parse(iso) }.getOrNull() ?: return ""
    return dt.toZonedDateTime()
        .format(DateTimeFormatter.ofPattern("h:mm a", Locale.US))
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
