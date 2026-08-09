import Foundation
import SildCore

// Kotlin default arguments export as unavailable, so the generated `SildConfig` init
// demands every field. These go through Kotlin, keeping the defaults in one place.

public extension SildConfig {
    /// A config for [baseUrl] whose token comes from [token]; everything else keeps the
    /// SDK's default.
    static func make(
        baseUrl: String,
        token: @escaping () async throws -> String,
        userId: String? = nil,
        locale: String? = nil
    ) -> SildConfig {
        make(baseUrl: baseUrl, tokenProvider: ClosureTokenProvider(token), userId: userId, locale: locale)
    }

    /// As above, for a host that already has a `TokenProvider`.
    static func make(
        baseUrl: String,
        tokenProvider: TokenProvider,
        userId: String? = nil,
        locale: String? = nil
    ) -> SildConfig {
        InteropKt.sildConfig(baseUrl: baseUrl, tokenProvider: tokenProvider, userId: userId, locale: locale)
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
        userId: String? = nil,
        locale: String? = nil,
        target: SildTarget = .list,
        onChime: @escaping () -> Void = {},
        onClose: @escaping () -> Void = {}
    ) {
        self.init(
            config: .make(baseUrl: baseURL, token: token, userId: userId, locale: locale),
            target: target,
            onChime: onChime,
            onClose: onClose
        )
    }
}
