package io.sild.core

import kotlin.time.Instant
import kotlinx.datetime.TimeZone
import kotlinx.datetime.toLocalDateTime

internal actual fun clock(iso: String?, zoneId: String?): String {
    val instant = parseIso(iso) ?: return ""
    val zone = if (zoneId != null) TimeZone.of(zoneId) else TimeZone.currentSystemDefault()
    val t = instant.toLocalDateTime(zone)
    val hour12 = ((t.hour + 11) % 12) + 1
    val marker = if (t.hour < 12) "AM" else "PM"
    return "$hour12:${t.minute.toString().padStart(2, '0')} $marker"
}

// An offset is required — a bare local time names no instant, and the JVM side
// rejects it too.
private fun parseIso(iso: String?): Instant? {
    if (iso.isNullOrBlank()) return null
    return runCatching { Instant.parse(iso.trim()) }.getOrNull()
}
