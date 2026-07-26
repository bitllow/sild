import Foundation
import SildCore

/// A `TokenProvider` backed by a Swift closure. Kotlin awaits `token()` as a suspend
/// function, so the host can mint a JWT with its own async networking.
public final class ClosureTokenProvider: NSObject, TokenProvider {
    private let mint: () async throws -> String

    public init(_ mint: @escaping () async throws -> String) {
        self.mint = mint
    }

    public func token() async throws -> String {
        try await mint()
    }
}

/// Proves the packaging scheme end to end: Swift → XCFramework → shared Kotlin REST.
/// The real SDK will expose the messenger UI over the same `SildSession`.
public final class SildCoreSmoke {
    private let session: SildSession
    private let bridge = CallbackBridge()

    public var client: SildClient { session.client }

    public init(baseURL: String, userId: String, token: @escaping () async throws -> String) {
        let config = SildConfig(
            baseUrl: baseURL,
            tokenProvider: ClosureTokenProvider(token),
            userId: userId,
            metadata: [:],
            uploadSizeLimitBytes: 10 * 1024 * 1024
        )
        session = SildSession(config: config, onChime: {}, transport: nil)
    }

    public func close() {
        session.close()
        bridge.closeAll()
    }

    public var state: SildState? { client.state.value as? SildState }

    /// Start the client and wait until branding has loaded.
    public func startAndAwaitBrand(timeout: TimeInterval = 15) async -> SildState? {
        client.start(conversationId: nil)
        return await poll(timeout: timeout) { $0.ready && !$0.brandName.isEmpty }
    }

    /// Open a support request; nil when creation failed.
    public func openSupportRequest() async throws -> String? {
        try await bridge.run { done in client.openSupportRequest { done($0) } }
    }

    /// Send to the active conversation; false when the send did not land.
    public func send(_ text: String) async throws -> Bool {
        try await bridge.run { done in
            client.send(text: text, attachments: []) { done($0.boolValue) }
        }
    }

    /// Poll the client's state until [predicate] holds, or the timeout elapses.
    public func poll(timeout: TimeInterval, until predicate: (SildState) -> Bool) async -> SildState? {
        let deadline = Date().addingTimeInterval(timeout)
        while Date() < deadline {
            if let s = state, predicate(s) { return s }
            try? await Task.sleep(nanoseconds: 100_000_000)
        }
        return state
    }
}
