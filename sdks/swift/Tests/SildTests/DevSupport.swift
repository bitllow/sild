import Foundation

/// The dev endpoints the suites talk to. Unset SILD_BASE_URL means no backend, and the
/// tests that need one skip.
enum Dev {
    static let baseURL = ProcessInfo.processInfo.environment["SILD_BASE_URL"]
        .flatMap { $0.isEmpty ? nil : $0 }

    static func uid(_ prefix: String) -> String { "\(prefix)_\(UUID().uuidString.prefix(12))" }

    /// The dev token-mint endpoint stands in for the host backend.
    static func mintToken(_ base: String, _ userId: String) async throws -> String {
        let url = URL(string: "\(base)/v1/dev/widget-token?user_id=\(userId)")!
        let (data, _) = try await URLSession.shared.data(from: url)
        let json = try JSONSerialization.jsonObject(with: data) as! [String: Any]
        return json["token"] as! String
    }
}

/// Carries an awaited outcome out of a detached task.
final class Box<T>: @unchecked Sendable {
    var value: T?
}
