package io.sild.core


// clock formats an ISO-8601 timestamp as 12-hour "h:mm AM/PM" in the device's local
// time, matching the web client's clock() (which reads local hours off a Date).
// Backend timestamps carry an explicit offset (usually UTC "Z"); we parse them to an
// instant and format in [zoneId], or the device zone when it is null. Empty string on
// null/blank/unparseable input.
//
// Per-platform because the JVM side must stay on java.util (API 1): java.time would
// oblige every Android host below API 26 to turn on core-library desugaring.
internal expect fun clock(iso: String?, zoneId: String? = null): String

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
