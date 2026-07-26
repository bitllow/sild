import XCTest
import Sild
import SildCore

// The transport's pure parts and the state-observation bridge. The rendered realtime
// round-trip lives in the sample's XCUITest; these cover the seams underneath it.
final class RealtimeBridgeTests: XCTestCase {
    private var baseURL: String? {
        ProcessInfo.processInfo.environment["SILD_BASE_URL"].flatMap { $0.isEmpty ? nil : $0 }
    }

    // The endpoint and envelope parsing come from shared Kotlin, so iOS and Android
    // cannot drift on either.
    func testDerivesWebSocketEndpointFromTheBase() {
        XCTAssertEqual(RealtimeKt.realtimeEndpoint(base: "http://localhost:8080"), "ws://localhost:8080/v1/ws")
        XCTAssertEqual(RealtimeKt.realtimeEndpoint(base: "https://api.sild.io"), "wss://api.sild.io/v1/ws")
    }

    func testParsesPublicationEnvelope() {
        let env = RealtimeKt.parseRealtimeEnvelope(
            payload: #"{"type":"message.created","conversation_id":"c1","data":{"id":"m1"}}"#
        )
        XCTAssertEqual(env?.type, "message.created")
        XCTAssertEqual(env?.conversationId, "c1")
        XCTAssertNil(RealtimeKt.parseRealtimeEnvelope(payload: "not json"))
    }

    // connect() must be idempotent: SildClient.start() can run more than once over a
    // session's life, and a second socket would double every publication.
    func testConnectIsIdempotent() {
        let config = SildConfig(
            baseUrl: "http://127.0.0.1:1",
            tokenProvider: ClosureTokenProvider { "t" },
            userId: "u",
            metadata: [:],
            uploadSizeLimitBytes: 1024
        )
        let transport = CentrifugeRealtimeTransport(config: config, onConnection: { _ in }, onEnvelope: { _ in })
        transport.connect()
        transport.connect()
        transport.destroy()
        // destroy() twice must not trap either.
        transport.destroy()
    }

    // watchState replaces a Flow, which Swift cannot consume: the current value must
    // arrive immediately and cancelling must stop delivery.
    func testWatchStateDeliversCurrentValueAndStops() async throws {
        guard let base = baseURL else { throw XCTSkip("set SILD_BASE_URL to run this test") }
        let model = SildModel(config: SildConfig(
            baseUrl: base,
            tokenProvider: ClosureTokenProvider { try await self.mintToken(base, "u_swift_watch") },
            userId: "u_swift_watch",
            metadata: [:],
            uploadSizeLimitBytes: 1024
        ))
        defer { model.close() }

        // The initial snapshot is there before anything has started.
        XCTAssertFalse(model.state.ready)

        model.start()
        let deadline = Date().addingTimeInterval(20)
        while Date() < deadline, !model.state.ready {
            try await Task.sleep(nanoseconds: 100_000_000)
        }
        XCTAssertTrue(model.state.ready, "observing the shared state should surface start()'s result")
        XCTAssertFalse(model.state.brandName.isEmpty, "brand should have loaded through the bridge")

        model.close()
        let afterClose = model.state.ready
        try await Task.sleep(nanoseconds: 300_000_000)
        XCTAssertEqual(model.state.ready, afterClose, "a cancelled watch must stop updating")
    }

    private func mintToken(_ base: String, _ userId: String) async throws -> String {
        let url = URL(string: "\(base)/v1/dev/widget-token?user_id=\(userId)")!
        let (data, _) = try await URLSession.shared.data(from: url)
        let json = try JSONSerialization.jsonObject(with: data) as! [String: Any]
        return json["token"] as! String
    }
}
