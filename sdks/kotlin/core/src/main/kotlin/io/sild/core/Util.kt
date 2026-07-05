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
