import XCTest
import Sild
import SildCore

// The iOS SDK's end-to-end smoke, mirroring the Android instrumented
// SildMessengerE2ETest: launch the real sample app on a simulator, drive the actual SDK
// entry points, and round-trip messages through a live sild-dev. It exercises the whole
// stack a host app hits — SildMessenger, the shared SildClient's REST + realtime, and
// the SwiftUI render — so a launch crash, a broken wire contract or a render failure
// fails CI instead of shipping.
//
// Requires a running sild-dev reachable from the simulator (it shares the host's
// network, so localhost works). Run locally with:
//   (backend) make dev
//   (sdk)     xcodebuild test -scheme SildSample -destination 'platform=iOS Simulator,name=…'
final class SildMessengerUITests: XCTestCase {
    private var app: XCUIApplication!

    private var baseURL: String {
        ProcessInfo.processInfo.environment["SILD_BASE_URL"].flatMap { $0.isEmpty ? nil : $0 }
            ?? "http://localhost:8080"
    }

    override func setUp() {
        continueAfterFailure = false
        app = XCUIApplication()
        // The app is a separate process, so hand it the backend explicitly rather than
        // relying on it inheriting the runner's environment.
        app.launchEnvironment["SILD_BASE_URL"] = baseURL
    }

    private func uid(_ prefix: String) -> String { "\(prefix)-\(UUID().uuidString.prefix(8))" }

    /// Skip rather than fail when there is no backend to talk to.
    private func requireBackend() async throws {
        let url = URL(string: "\(baseURL)/healthz")!
        let healthy = (try? await URLSession.shared.data(from: url))
            .map { ($0.1 as? HTTPURLResponse)?.statusCode == 200 } ?? false
        try XCTSkipUnless(healthy, "sild-dev is not reachable at \(baseURL)")
    }

    // Support flow: the sample launches, the messenger opens on Home, a draft
    // conversation starts, and a typed message lands in the thread.
    func testOpenSupportAndSendMessageRoundTrips() async throws {
        try await requireBackend()
        app.launch()

        // The sample rendered without crashing on launch.
        XCTAssertTrue(app.staticTexts["Acme Rides"].waitForExistence(timeout: 20))

        // Open support — the messenger lands on Home (welcome + New conversation),
        // matching the web widget.
        app.buttons["sample.openSupport"].tap()
        XCTAssertTrue(app.staticTexts["New conversation"].waitForExistence(timeout: 20))

        // Start a draft; the composer appears (thread screen rendered).
        app.buttons["sild.home.new"].tap()
        let input = app.textFields["sild.composer.input"]
        XCTAssertTrue(input.waitForExistence(timeout: 20), "the composer should render")

        // Round-trip a message: type, send, assert it lands in the thread.
        let body = uid("e2e-hello")
        input.tap()
        input.typeText(body)
        app.buttons["sild.composer.send"].tap()
        XCTAssertTrue(
            app.staticTexts[body].waitForExistence(timeout: 30),
            "the sent message should render in the thread"
        )
    }

    // Peer chat, incoming direction, on-device: the sample's "Message driver" card opens
    // the trip's rider↔driver conversation; a driver-side client in this test process
    // (the other party) sends a message, and it must appear as a rendered bubble in the
    // rider's SwiftUI thread — the realtime path the REST-only smoke tests cannot see.
    func testDriverReplyArrivesOverRealtime() async throws {
        try await requireBackend()

        // The same fixed reference the sample's card opens, so both sides meet in one
        // conversation; sild-dev derives the driver id from it.
        let ref = "trip_ios_9021"
        let convId = try await ensurePeerConversation(reference: ref)

        // Bring the driver online first, so it is subscribed before the rider looks.
        let driver = DriverClient(base: baseURL, userId: "u_driver_\(ref)", conversationId: convId)
        defer { driver.close() }
        try await driver.start()

        // Sent before the rider opens, so it arrives in the thread's REST load. Seeing it
        // render is how we know the load has settled — no assumption about seed content.
        let preload = uid("preload")
        try await driver.send(preload)

        app.launch()
        XCTAssertTrue(app.staticTexts["Acme Rides"].waitForExistence(timeout: 20))
        app.buttons["sample.messageDriver"].tap()
        XCTAssertTrue(
            app.textFields["sild.composer.input"].waitForExistence(timeout: 30),
            "the peer thread should render"
        )

        // Gate on the socket, not on REST timing: a message can render from the thread
        // load or a catch-up refetch, so without this the test would pass with a dead
        // transport.
        let connected = expectation(for: NSPredicate(format: "value == %@", "CONNECTED"),
                                    evaluatedWith: app.otherElements["sild.connection"])
        await fulfillment(of: [connected], timeout: 45)
        // And the initial load has settled, so the only way the next one can arrive is
        // the publication we are about to trigger.
        XCTAssertTrue(
            app.staticTexts[preload].waitForExistence(timeout: 30),
            "the pre-sent message should have loaded over REST first"
        )

        let reply = uid("driver-reply")
        try await driver.send(reply)

        XCTAssertTrue(
            app.staticTexts[reply].waitForExistence(timeout: 45),
            "the driver's message should arrive over realtime and render"
        )
    }

    private func ensurePeerConversation(reference: String) async throws -> String {
        let url = URL(string: "\(baseURL)/v1/dev/peer-conversation?user_id=u_demo_ios&reference=\(reference)")!
        let (data, _) = try await URLSession.shared.data(from: url)
        let json = try JSONSerialization.jsonObject(with: data) as? [String: Any]
        guard let id = json?["conversation_id"] as? String else {
            throw NSError(domain: "UITest", code: 1, userInfo: [NSLocalizedDescriptionKey: "no conversation_id"])
        }
        return id
    }
}

/// The other party: a real shared client, exactly as the Android E2E uses a driver-side
/// SildClient to prove the incoming realtime path against the rider's rendered UI.
final class DriverClient {
    private let session: SildSession
    private let conversationId: String

    init(base: String, userId: String, conversationId: String) {
        self.conversationId = conversationId
        let config = SildConfig(
            baseUrl: base,
            tokenProvider: ClosureTokenProvider {
                let url = URL(string: "\(base)/v1/dev/widget-token?user_id=\(userId)")!
                let (data, _) = try await URLSession.shared.data(from: url)
                let json = try JSONSerialization.jsonObject(with: data) as? [String: Any]
                return json?["token"] as? String ?? ""
            },
            userId: userId,
            metadata: [:],
            uploadSizeLimitBytes: 10 * 1024 * 1024
        )
        session = SildSession(config: config, onChime: {}, transport: centrifugeTransportFactory())
    }

    private var state: SildState? { session.client.state.value as? SildState }

    func start() async throws {
        session.client.start(conversationId: conversationId)
        let deadline = Date().addingTimeInterval(30)
        while Date() < deadline {
            if let s = state, s.ready, s.activeId == conversationId { return }
            try await Task.sleep(nanoseconds: 200_000_000)
        }
        throw NSError(domain: "DriverClient", code: 1, userInfo: [NSLocalizedDescriptionKey: "driver never became ready"])
    }

    func send(_ body: String) async throws {
        let ok = await withCheckedContinuation { (c: CheckedContinuation<Bool, Never>) in
            session.client.send(text: body, attachments: []) { c.resume(returning: $0.boolValue) }
        }
        guard ok else {
            throw NSError(domain: "DriverClient", code: 2, userInfo: [NSLocalizedDescriptionKey: "driver send failed"])
        }
    }

    func close() { session.close() }
}
