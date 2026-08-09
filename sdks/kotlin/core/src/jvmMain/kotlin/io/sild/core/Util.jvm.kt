package io.sild.core

import java.text.SimpleDateFormat
import java.util.Date
import java.util.Locale
import java.util.TimeZone

internal actual fun clock(iso: String?, zoneId: String?): String {
    val instant = parseIso(iso) ?: return ""
    // SimpleDateFormat isn't thread-safe, so build per call (clock runs at message-
    // mapping time, not in a tight loop). No explicit zone → formats in the device zone.
    val fmt = SimpleDateFormat("h:mm a", Locale.US)
    if (zoneId != null) fmt.timeZone = TimeZone.getTimeZone(zoneId)
    return fmt.format(instant)
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

// Android hosts reach the device's ordered preference list through LocaleList; the
// plain JVM has one default, which is the whole preference on a desktop or in a test.
internal actual fun deviceLocales(): List<String> = listOf(Locale.getDefault().toLanguageTag())
