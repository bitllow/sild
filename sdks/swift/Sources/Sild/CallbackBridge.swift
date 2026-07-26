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

/// Couples the cancellation handler to the in-flight `Once`, in either order.
///
/// `onCancel` can run before the continuation exists — the task may already be cancelled
/// when `run` is entered. Recording that fact means the `Once` installed afterwards is
/// failed immediately instead of waiting for a Kotlin callback that may never come.
private final class OnceBox<T>: @unchecked Sendable {
    private let lock = NSLock()
    private var once: Once<T>?
    private var cancelled = false

    /// Install the continuation; false when cancellation already won, and the caller
    /// must not start the underlying call.
    func set(_ value: Once<T>) -> Bool {
        lock.lock()
        if cancelled {
            lock.unlock()
            value.resume(.failure(CancellationError()))
            return false
        }
        once = value
        lock.unlock()
        return true
    }

    /// Fail the continuation now, or mark it to be failed on arrival.
    func cancel() {
        lock.lock()
        cancelled = true
        let current = once
        once = nil
        lock.unlock()
        current?.resume(.failure(CancellationError()))
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
                guard box.set(once) else { return }
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
            box.cancel()
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
