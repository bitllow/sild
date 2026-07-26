import Foundation
import SildCore

/// Why a pending call ended without the Kotlin callback firing.
public enum SildError: Error {
    /// The session was closed while the call was in flight.
    case closed
}

/// Resumes a continuation exactly once, whichever outcome arrives first.
private final class Once<T>: @unchecked Sendable {
    private let lock = NSLock()
    private var continuation: CheckedContinuation<T, Error>?

    init(_ continuation: CheckedContinuation<T, Error>) {
        self.continuation = continuation
    }

    func resume(_ result: Result<T, Error>) {
        lock.lock()
        let c = continuation
        continuation = nil
        lock.unlock()
        c?.resume(with: result)
    }
}

/// Hands the cancellation handler a reference to the in-flight `Once`.
private final class OnceBox<T>: @unchecked Sendable {
    private let lock = NSLock()
    private var once: Once<T>?

    func set(_ value: Once<T>) {
        lock.lock()
        once = value
        lock.unlock()
    }

    func get() -> Once<T>? {
        lock.lock()
        defer { lock.unlock() }
        return once
    }
}

/// Tracks calls waiting on a Kotlin callback so `close()` can finish them.
///
/// Kotlin drops the callback when its scope is cancelled — `openSupportRequest`
/// rethrows CancellationException without calling back, and a `launch` on an already
/// cancelled scope never runs at all — so without this every such await hangs forever.
private final class PendingCalls: @unchecked Sendable {
    private let lock = NSLock()
    private var waiters: [UUID: () -> Void] = [:]
    private var isClosed = false

    /// Register a waiter; false when the session is already closed.
    func track(_ id: UUID, abandon: @escaping () -> Void) -> Bool {
        lock.lock()
        defer { lock.unlock() }
        if isClosed { return false }
        waiters[id] = abandon
        return true
    }

    func finish(_ id: UUID) {
        lock.lock()
        waiters[id] = nil
        lock.unlock()
    }

    func abandonAll() {
        lock.lock()
        isClosed = true
        let pending = Array(waiters.values)
        waiters.removeAll()
        lock.unlock()
        pending.forEach { $0() }
    }
}

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
    private let pending = PendingCalls()

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
        pending.abandonAll()
    }

    public var state: SildState? { client.state.value as? SildState }

    /// Start the client and wait until branding has loaded.
    public func startAndAwaitBrand(timeout: TimeInterval = 15) async -> SildState? {
        client.start(conversationId: nil)
        return await poll(timeout: timeout) { $0.ready && !$0.brandName.isEmpty }
    }

    /// Open a support request; nil when creation failed.
    public func openSupportRequest() async throws -> String? {
        try await bridged { done in client.openSupportRequest { done($0) } }
    }

    /// Send to the active conversation; false when the send did not land.
    public func send(_ text: String) async throws -> Bool {
        try await bridged { done in
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

    /// Await a Kotlin callback-style call, resuming exactly once on every path.
    ///
    /// A cancelled Swift task stops waiting but does NOT cancel the Kotlin work: the
    /// client exposes no per-call handle (only `backToList`/`destroy`), and adding one
    /// would change the shared Android API.
    private func bridged<T>(_ start: (@escaping (T) -> Void) -> Void) async throws -> T {
        let id = UUID()
        let box = OnceBox<T>()
        return try await withTaskCancellationHandler {
            try await withCheckedThrowingContinuation { continuation in
                let once = Once<T>(continuation)
                box.set(once)
                guard pending.track(id, abandon: { once.resume(.failure(SildError.closed)) }) else {
                    once.resume(.failure(SildError.closed))
                    return
                }
                start { value in
                    self.pending.finish(id)
                    once.resume(.success(value))
                }
            }
        } onCancel: {
            pending.finish(id)
            box.get()?.resume(.failure(CancellationError()))
        }
    }
}
