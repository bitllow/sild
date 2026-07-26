import Foundation
import Observation
import SildCore

/// Owns the shared client and republishes its state to SwiftUI.
///
/// `watchState` delivers on Kotlin's main dispatcher, so assignments land on the main
/// actor already; the model adds no logic of its own — every decision stays in the
/// shared `SildClient`.
///
/// Lifetime matches the Android `SildSession` ViewModel: the session survives a
/// transient disappearance (a tab switch or a pushed screen — the peer of a
/// configuration change) and is torn down when the view that owns it is released.
/// `close()` is still safe to call early; `start()` afterwards builds a fresh session.
@Observable
public final class SildModel {
    public private(set) var state: SildState
    /// True until the first send creates the support request (the lazy-create draft).
    public private(set) var draft = false

    @ObservationIgnored private let config: SildConfig
    @ObservationIgnored private let onChime: () -> Void
    @ObservationIgnored private let bridge = CallbackBridge()
    @ObservationIgnored private var session: SildSession?
    @ObservationIgnored private var watch: SildCancellable?

    public init(config: SildConfig, onChime: @escaping () -> Void = {}) {
        self.config = config
        self.onChime = onChime
        state = InteropKt.initialState()
    }

    deinit {
        watch?.cancel()
        session?.close()
    }

    /// Load branding, connect realtime, then open [conversationId] or list all. Runs
    /// once per session — a view reappearing must not re-open the target.
    public func start(conversationId: String? = nil) {
        guard session == nil else { return }
        bridge.reopen()
        let created = SildSession(
            config: config,
            onChime: onChime,
            transport: centrifugeTransportFactory()
        )
        session = created
        watch = created.watchState { [weak self] next in self?.state = next }
        created.client.start(conversationId: conversationId)
    }

    public func close() {
        watch?.cancel()
        watch = nil
        session?.close()
        session = nil
        bridge.closeAll()
    }

    // ── intents (thin pass-throughs; the policy lives in SildClient) ──────────

    public func beginDraft() { draft = true }

    public func openConversation(_ id: String) {
        session?.client.openConversation(id: id)
    }

    /// Leave the thread for the list, cancelling any in-flight support-request creation.
    public func backToList() {
        draft = false
        session?.client.backToList()
    }

    public func toggleSound() { session?.client.toggleSound() }

    /// Send into the active thread, creating the support request first when this is a
    /// draft — the same lazy-create order the Android `ThreadScreen` uses.
    ///
    /// Every await goes through the bridge: `backToList()` cancels creation and Kotlin
    /// then drops its callback, which would otherwise strand the composer's send.
    public func send(_ body: String, attachments: [PendingAttachment]) async -> Bool {
        guard let client = session?.client else { return false }
        do {
            if draft && state.activeId == nil {
                let created = try await bridge.run { (done: @escaping (String?) -> Void) in
                    client.openSupportRequest { done($0) }
                }
                guard created != nil else { return false }
            }
            let ok = try await bridge.run { (done: @escaping (Bool) -> Void) in
                client.send(text: body, attachments: attachments) { done($0.boolValue) }
            }
            if ok { draft = false }
            return ok
        } catch {
            // Abandoned (the user left the draft, or the session closed) — the composer
            // keeps the text, exactly as on a failed send.
            return false
        }
    }

    public func upload(bytes: Data, filename: String, mimeType: String) async throws -> PendingAttachment {
        guard let client = session?.client else { throw SildError.closed }
        return try await client.upload(
            bytes: KotlinByteArray.from(bytes),
            filename: filename,
            mimeType: mimeType
        )
    }

    public var uploadSizeLimitBytes: Int64 {
        session?.client.uploadSizeLimitBytes ?? config.uploadSizeLimitBytes
    }
}

extension KotlinByteArray {
    /// Kotlin/Native has no Data bridge, so copy through the boxed byte array.
    static func from(_ data: Data) -> KotlinByteArray {
        let array = KotlinByteArray(size: Int32(data.count))
        for (i, byte) in data.enumerated() {
            array.set(index: Int32(i), value: Int8(bitPattern: byte))
        }
        return array
    }
}
