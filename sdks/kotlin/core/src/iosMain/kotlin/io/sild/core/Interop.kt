package io.sild.core

import kotlinx.cinterop.ExperimentalForeignApi
import kotlinx.cinterop.addressOf
import kotlinx.cinterop.usePinned
import platform.Foundation.NSData
import platform.posix.memcpy

// Kotlin default arguments do not cross into Swift — an all-defaults constructor is
// exported as unavailable — so hand Swift the defaults it needs.

/** The server's default brand, for rendering before the real config has loaded. */
fun defaultBrandConfig(): BrandConfig = BrandConfig()

/** The state a client holds before it has started. */
fun initialState(): SildState = SildState()

/** A config carrying [SildConfig]'s own defaults for every optional field, so Swift
 *  never restates them — a host supplies only what it must. */
fun sildConfig(
    baseUrl: String,
    tokenProvider: TokenProvider,
    userId: String?,
): SildConfig = SildConfig(baseUrl = baseUrl, tokenProvider = tokenProvider, userId = userId)

/** Copy [data] in one pass. Swift has no bridge to ByteArray, and filling one through
 *  the generated setter costs a bridged call per byte. */
@OptIn(ExperimentalForeignApi::class)
fun byteArray(data: NSData): ByteArray {
    val size = data.length.toInt()
    if (size == 0) return ByteArray(0)
    val out = ByteArray(size)
    out.usePinned { memcpy(it.addressOf(0), data.bytes, data.length) }
    return out
}
