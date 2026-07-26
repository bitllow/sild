import Foundation
import Sild
import SildCore

/// Stands in for the host app's own backend, exactly like the Android sample's
/// DevBackend: it mints a user token and (for the driver chat) asks the dev server to
/// ensure a peer conversation. A real host would mint tokens with its API key
/// server-side and create peer conversations via POST /v1/conversations.
enum DevBackend {
    /// The simulator shares the host's network, so localhost is the dev server.
    /// SILD_BASE_URL overrides it (CI publishes it on the simulator's launchd).
    static let base = ProcessInfo.processInfo.environment["SILD_BASE_URL"].flatMap {
        $0.isEmpty ? nil : $0
    } ?? "http://localhost:8080"

    static let userId = "u_demo_ios"

    /// The trip whose driver↔rider peer chat the "Message driver" card opens. sild-dev
    /// derives the driver's id as "u_driver_<reference>", so a test can act as the
    /// driver and round-trip a message against the rider's UI.
    static let driverTripRef = "trip_ios_9021"

    static func tokenProvider(for userId: String) -> TokenProvider {
        ClosureTokenProvider { try await mintToken(userId) }
    }

    static func mintToken(_ userId: String) async throws -> String {
        let url = URL(string: "\(base)/v1/dev/widget-token?user_id=\(userId)")!
        let (data, _) = try await URLSession.shared.data(from: url)
        let json = try JSONSerialization.jsonObject(with: data) as? [String: Any]
        guard let token = json?["token"] as? String else {
            throw NSError(domain: "DevBackend", code: 1)
        }
        return token
    }

    /// Ensure the trip's driver↔rider peer conversation exists; return its id.
    static func ensureDriverConversation(_ reference: String) async throws -> String {
        let url = URL(string: "\(base)/v1/dev/peer-conversation?user_id=\(userId)&reference=\(reference)")!
        let (data, _) = try await URLSession.shared.data(from: url)
        let json = try JSONSerialization.jsonObject(with: data) as? [String: Any]
        guard let id = json?["conversation_id"] as? String else {
            throw NSError(domain: "DevBackend", code: 2)
        }
        return id
    }
}
