import Foundation
import SildCore
import SwiftCentrifuge

/// The realtime socket for iOS, mirroring the Android `CentrifugeTransport`: the same
/// broker and protocol the inbox and web widget use, so server-derived channel
/// subscriptions and the reconnect-after-create flow behave identically.
///
/// Server-side subscriptions mean publications arrive on the client delegate's
/// `onPublication`; this never subscribes to a channel itself.
public final class CentrifugeRealtimeTransport: NSObject, RealtimeTransport {
    private let config: SildConfig
    private let onConnection: (ConnectionState) -> Void
    private let onEnvelope: (RealtimeEnvelope) -> Void

    private let lock = NSLock()
    private var client: CentrifugeClient?
    private var tokenTasks: [Task<Void, Never>] = []

    public init(
        config: SildConfig,
        onConnection: @escaping (ConnectionState) -> Void,
        onEnvelope: @escaping (RealtimeEnvelope) -> Void
    ) {
        self.config = config
        self.onConnection = onConnection
        self.onEnvelope = onEnvelope
    }

    // Centrifuge calls its delegate on a private queue. Kotlin's own watchState
    // delivers on the main dispatcher, and these callbacks reach host code
    // (SildClient's onChime, the observable state), so hop before crossing over.
    private func onMain(_ work: @escaping () -> Void) {
        if Thread.isMainThread { work() } else { DispatchQueue.main.async(execute: work) }
    }

    public func connect() {
        lock.lock()
        guard client == nil else { return lock.unlock() }

        // Centrifuge asks for a token on connect and on refresh; delegate to the host
        // token provider, forced-fresh each time like the web getToken(true).
        // weak self + a retained handle: a token provider that never returns must not
        // pin the session, and destroy() has to be able to abandon it.
        let centrifugeConfig = CentrifugeClientConfig(
            tokenGetter: { [weak self] _, completion in
                guard let self else { return completion(.failure(CancellationError())) }
                let task = Task { [weak self] in
                    guard let self else { return completion(.failure(CancellationError())) }
                    do {
                        let token = try await self.config.tokenProvider.token()
                        try Task.checkCancellation()
                        completion(.success(token))
                    } catch {
                        completion(.failure(error))
                    }
                }
                self.lock.lock()
                self.tokenTasks.append(task)
                self.tokenTasks.removeAll { $0.isCancelled }
                self.lock.unlock()
            }
        )
        let created = CentrifugeClient(
            endpoint: RealtimeKt.realtimeEndpoint(base: config.base),
            config: centrifugeConfig,
            delegate: self
        )
        client = created
        lock.unlock()
        created.connect()
    }

    /// Force the server to re-derive this connection's channel set — needed right after
    /// creating a conversation, whose channel the live socket predates.
    public func reconnect() {
        lock.lock()
        let current = client
        lock.unlock()
        guard let current else { return }
        current.disconnect()
        current.connect()
    }

    public func destroy() {
        lock.lock()
        let current = client
        let tasks = tokenTasks
        client = nil
        tokenTasks = []
        lock.unlock()
        tasks.forEach { $0.cancel() }
        current?.disconnect()
    }
}

extension CentrifugeRealtimeTransport: CentrifugeClientDelegate {
    public func onConnecting(_ client: CentrifugeClient, _ event: CentrifugeConnectingEvent) {
        onMain { self.onConnection(.connecting) }
    }

    public func onConnected(_ client: CentrifugeClient, _ event: CentrifugeConnectedEvent) {
        onMain { self.onConnection(.connected) }
    }

    public func onDisconnected(_ client: CentrifugeClient, _ event: CentrifugeDisconnectedEvent) {
        onMain { self.onConnection(.disconnected) }
    }

    public func onPublication(_ client: CentrifugeClient, _ event: CentrifugeServerPublicationEvent) {
        guard let payload = String(data: event.data, encoding: .utf8),
              let envelope = RealtimeKt.parseRealtimeEnvelope(payload: payload) else { return }
        onMain { self.onEnvelope(envelope) }
    }
}

/// Builds the transport for a `SildSession`, matching the platform default the Android
/// SDK gets from `defaultRealtimeTransport()`.
public func centrifugeTransportFactory() -> RealtimeTransportFactory {
    RealtimeTransportFactoryAdapter()
}

private final class RealtimeTransportFactoryAdapter: NSObject, RealtimeTransportFactory {
    func create(
        cfg: SildConfig,
        scope: Kotlinx_coroutines_coreCoroutineScope,
        onConnection: @escaping (ConnectionState) -> Void,
        onEnvelope: @escaping (RealtimeEnvelope) -> Void
    ) -> RealtimeTransport {
        CentrifugeRealtimeTransport(config: cfg, onConnection: onConnection, onEnvelope: onEnvelope)
    }
}
