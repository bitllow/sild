import Foundation

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

/// Turns the client's callback-style calls into `async` ones that always finish.
///
/// Kotlin drops the callback when its scope is cancelled — `openSupportRequest`
/// rethrows CancellationException without calling back, and a `launch` on an already
/// cancelled scope never runs at all — so without `close()` completing the waiters,
/// every such await would hang forever.
final class CallbackBridge: @unchecked Sendable {
    private let lock = NSLock()
    private var waiters: [UUID: () -> Void] = [:]
    private var isClosed = false

    /// Await a callback-style call, resuming exactly once on every path.
    ///
    /// A cancelled Swift task stops waiting but does NOT cancel the Kotlin work: the
    /// client exposes no per-call handle (only `backToList`/`destroy`), and adding one
    /// would change the shared Android API.
    func run<T>(_ start: (@escaping (T) -> Void) -> Void) async throws -> T {
        let id = UUID()
        let box = OnceBox<T>()
        return try await withTaskCancellationHandler {
            try await withCheckedThrowingContinuation { continuation in
                let once = Once<T>(continuation)
                box.set(once)
                guard track(id, abandon: { once.resume(.failure(SildError.closed)) }) else {
                    once.resume(.failure(SildError.closed))
                    return
                }
                start { value in
                    self.finish(id)
                    once.resume(.success(value))
                }
            }
        } onCancel: {
            finish(id)
            box.get()?.resume(.failure(CancellationError()))
        }
    }

    /// Fail everything still waiting; every later call fails immediately too.
    func closeAll() {
        lock.lock()
        isClosed = true
        let pending = Array(waiters.values)
        waiters.removeAll()
        lock.unlock()
        pending.forEach { $0() }
    }

    /// Reopen after `closeAll` — a session that is started again must be usable.
    func reopen() {
        lock.lock()
        isClosed = false
        lock.unlock()
    }

    private func track(_ id: UUID, abandon: @escaping () -> Void) -> Bool {
        lock.lock()
        defer { lock.unlock() }
        if isClosed { return false }
        waiters[id] = abandon
        return true
    }

    private func finish(_ id: UUID) {
        lock.lock()
        waiters[id] = nil
        lock.unlock()
    }
}
