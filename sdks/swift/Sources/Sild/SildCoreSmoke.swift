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

/// Drives a bare `SildSession` for the packaging smoke tests: Swift → XCFramework →
/// shared Kotlin REST, without the messenger UI in the way.
final class SildCoreSmoke {
    private let session: SildSession
    private let bridge = CallbackBridge()

    var client: SildClient { session.client }

    init(baseURL: String, userId: String, token: @escaping () async throws -> String) {
        session = SildSession(
            config: .make(baseUrl: baseURL, token: token, userId: userId),
            onChime: {},
            transport: nil
        )
    }

    func close() {
        session.close()
        bridge.closeAll()
    }

    var state: SildState? { client.state.value as? SildState }

    /// Start the client and wait until branding has loaded.
    func startAndAwaitBrand(timeout: TimeInterval = 15) async -> SildState? {
        client.start(conversationId: nil)
        return await poll(timeout: timeout) { $0.ready && !$0.brandName.isEmpty }
    }

    /// Open a support request; nil when creation failed.
    func openSupportRequest() async throws -> String? {
        try await bridge.run { done in client.openSupportRequest { done($0) } }
    }

    /// Send to the active conversation; false when the send did not land.
    func send(_ text: String) async throws -> Bool {
        try await bridge.run { done in
            client.send(text: text, attachments: []) { done($0.boolValue) }
        }
    }

    /// Poll the client's state until [predicate] holds, or the timeout elapses.
    func poll(timeout: TimeInterval, until predicate: (SildState) -> Bool) async -> SildState? {
        let deadline = Date().addingTimeInterval(timeout)
        while Date() < deadline {
            if let s = state, predicate(s) { return s }
            try? await Task.sleep(nanoseconds: 100_000_000)
        }
        return state
    }
}
