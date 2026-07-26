import XCTest
import Sild
import SildCore

// Runs on the iOS simulator against a live sild-dev, calling through the XCFramework
// into the shared Kotlin REST code — the end-to-end proof that the packaging scheme
// works. Without SILD_BASE_URL each test skips, so a skip never reads as a pass.
final class SildCoreSmokeTests: XCTestCase {
    private var baseURL: String? {
        ProcessInfo.processInfo.environment["SILD_BASE_URL"].flatMap { $0.isEmpty ? nil : $0 }
    }

    private func uid(_ prefix: String) -> String { "\(prefix)_\(UUID().uuidString.prefix(12))" }

    /// The dev token-mint endpoint stands in for the host backend.
    private func mintToken(_ base: String, _ userId: String) async throws -> String {
        let url = URL(string: "\(base)/v1/dev/widget-token?user_id=\(userId)")!
        let (data, _) = try await URLSession.shared.data(from: url)
        let json = try JSONSerialization.jsonObject(with: data) as! [String: Any]
        return json["token"] as! String
    }

    private func smoke(_ base: String, _ user: String) -> SildCoreSmoke {
        SildCoreSmoke(baseURL: base, userId: user) { try await self.mintToken(base, user) }
    }

    // The shared Kotlin client fetches branding over Ktor/Darwin, using a token minted
    // by Swift — so the suspend bridge works in both directions.
    func testFetchesBrandThroughTheFramework() async throws {
        guard let base = baseURL else {
            throw XCTSkip("set SILD_BASE_URL to run the smoke tests")
        }
        let s = smoke(base, uid("u_swift_brand"))
        defer { s.close() }

        let state = await s.startAndAwaitBrand()
        XCTAssertNotNil(state)
        XCTAssertTrue(state!.ready, "client should become ready")
        XCTAssertTrue(state!.brand.brand.hasPrefix("#"), "brand color, got \(state!.brand.brand)")
        XCTAssertFalse(state!.brandName.isEmpty, "brand name should load")
    }

    func testOpensSupportRequestAndSendsAMessage() async throws {
        guard let base = baseURL else {
            throw XCTSkip("set SILD_BASE_URL to run the smoke tests")
        }
        let s = smoke(base, uid("u_swift_support"))
        defer { s.close() }

        _ = await s.startAndAwaitBrand()
        let convId = try await s.openSupportRequest()
        XCTAssertNotNil(convId, "support request should be created")

        let body = "hello from swift \(uid("m"))"
        let sent = try await s.send(body)
        XCTAssertTrue(sent, "send should report success")

        // The message comes back mapped by the shared Kotlin code, as our own outgoing.
        let state = await s.poll(timeout: 15) { st in st.messages.contains { $0.body == body } }
        let msg = state?.messages.first { $0.body == body }
        XCTAssertNotNil(msg, "the sent message should appear in the thread")
        XCTAssertEqual(msg?.direction, Direction.out)
    }

    // The same abandonment, through SildModel: a send awaiting a support request that
    // close() cancels must report failure so the composer keeps the draft, not hang.
    func testModelSendDuringCloseReportsFailureInsteadOfHanging() async throws {
        let model = SildModel(config: SildConfig(
            baseUrl: "http://127.0.0.1:1",
            tokenProvider: ClosureTokenProvider {
                try await Task.sleep(nanoseconds: 60_000_000_000)
                return "never-arrives"
            },
            userId: "u_model_close",
            metadata: [:],
            uploadSizeLimitBytes: 1024
        ))
        model.start()
        model.beginDraft()

        let resumed = expectation(description: "the abandoned send resumes")
        let outcome = SendOutcomeBox()
        Task {
            outcome.value = await model.send("hello", attachments: [])
            resumed.fulfill()
        }

        try? await Task.sleep(nanoseconds: 200_000_000) // let it reach the token request
        model.close()

        await fulfillment(of: [resumed], timeout: 5)
        XCTAssertEqual(outcome.value, false, "an abandoned send must report failure")
    }

    // Kotlin drops the callback when its scope is cancelled, so closing mid-flight must
    // still complete the Swift await — otherwise the caller waits forever.
    //
    // A token that never arrives pins the call in flight: the client asks for one before
    // it makes any request, so this needs no backend and cannot race a fast reply.
    func testCloseDuringAwaitResumesInsteadOfHanging() async throws {
        let s = SildCoreSmoke(baseURL: "http://127.0.0.1:1", userId: uid("u_swift_close")) {
            try await Task.sleep(nanoseconds: 60_000_000_000)
            return "never-arrives"
        }

        // An expectation, not a task-group race: awaiting the task itself would block
        // this test forever on the very regression it is guarding against.
        let resumed = expectation(description: "the abandoned call resumes")
        let outcome = OutcomeBox()
        Task {
            do { outcome.value = .success(try await s.openSupportRequest()) }
            catch { outcome.value = .failure(error) }
            resumed.fulfill()
        }

        try? await Task.sleep(nanoseconds: 200_000_000) // let it reach the token request
        s.close()

        await fulfillment(of: [resumed], timeout: 5)
        guard case .failure = outcome.value else {
            return XCTFail("expected the abandoned call to fail, got \(String(describing: outcome.value))")
        }
    }
}

/// Carries the awaited outcome out of the detached task.
private final class OutcomeBox: @unchecked Sendable {
    var value: Result<String?, Error>?
}

private final class SendOutcomeBox: @unchecked Sendable {
    var value: Bool?
}
