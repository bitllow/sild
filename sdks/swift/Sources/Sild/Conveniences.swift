import Foundation
import SildCore

// Kotlin default arguments export as unavailable, so the generated `SildConfig` init
// demands all five fields — including a nil, an empty map and a byte ceiling. These
// conveniences cover the common case; the optional values still come from Kotlin, so
// the defaults live in exactly one place (`SildConfig`'s own declaration).

public extension SildConfig {
    /// A config for [baseUrl] whose token comes from [token]; everything else keeps the
    /// SDK's default.
    static func make(
        baseUrl: String,
        token: @escaping () async throws -> String
    ) -> SildConfig {
        InteropKt.sildConfig(baseUrl: baseUrl, tokenProvider: ClosureTokenProvider(token))
    }

    /// As above, for a host that already has a `TokenProvider`.
    static func make(baseUrl: String, tokenProvider: TokenProvider) -> SildConfig {
        InteropKt.sildConfig(baseUrl: baseUrl, tokenProvider: tokenProvider)
    }
}

public extension SildMessenger {
    /// The one-liner entry point: a base URL and a way to mint a token.
    ///
    /// ```swift
    /// SildMessenger(baseURL: base, token: { try await mintToken() }, target: .support) {
    ///     showing = false
    /// }
    /// ```
    init(
        baseURL: String,
        token: @escaping () async throws -> String,
        target: SildTarget = .list,
        onChime: @escaping () -> Void = {},
        onClose: @escaping () -> Void = {}
    ) {
        self.init(
            config: .make(baseUrl: baseURL, token: token),
            target: target,
            onChime: onChime,
            onClose: onClose
        )
    }
}
