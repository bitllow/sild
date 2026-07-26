package io.sild.core

// Kotlin default arguments do not cross into Swift — an all-defaults constructor is
// exported as unavailable — so hand Swift the defaults it needs.

/** The server's default brand, for rendering before the real config has loaded. */
fun defaultBrandConfig(): BrandConfig = BrandConfig()

/** The state a client holds before it has started. */
fun initialState(): SildState = SildState()

/** A config carrying [SildConfig]'s own defaults for every optional field, so Swift
 *  never restates them — a host supplies only what it must. */
fun sildConfig(baseUrl: String, tokenProvider: TokenProvider): SildConfig =
    SildConfig(baseUrl = baseUrl, tokenProvider = tokenProvider)
