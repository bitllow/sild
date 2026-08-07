import Foundation
import SildCore

// Push (§5.5). The decoding and the suppression rule live in the shared core;
// these are the bridges from what APNs actually hands a host app.
//
// The SDK does not register for remote notifications itself — the host owns that
// and forwards the token it was issued. See
// docs/adr/0001-host-app-owns-fcm-sdk-takes-the-token.md.

/// Sild's half of a push integration. Kotlin's `object` exports as a class whose
/// members hang off `shared`; these statics are the Swift-shaped way in.
public enum SildPushKit {
    /// The Android notification channel our payloads name. Here for parity —
    /// iOS has no channels.
    public static var notificationChannel: String { SildPush.shared.NOTIFICATION_CHANNEL }

    /// APNs delivers `[AnyHashable: Any]`; the core reads `[String: String]`.
    /// Numbers are stringified, so `unread_count` survives whichever form it takes.
    public static func data(from userInfo: [AnyHashable: Any]) -> [String: String] {
        var out: [String: String] = [:]
        for (key, value) in userInfo {
            guard let key = key as? String else { continue }
            if let s = value as? String {
                out[key] = s
            } else if let n = value as? NSNumber {
                out[key] = n.stringValue
            }
        }
        return out
    }

    /// Whether a notification came from Sild rather than from the host's own.
    public static func isSildPush(_ userInfo: [AnyHashable: Any]) -> Bool {
        SildPush.shared.isSildPush(data: data(from: userInfo))
    }

    /// Decodes a nudge, or nil when the notification is not ours.
    public static func parse(_ userInfo: [AnyHashable: Any]) -> SildPushMessage? {
        SildPush.shared.parse(data: data(from: userInfo))
    }
}

public extension SildClient {
    /// Whether the host should present this notification. Answered against what
    /// is on screen, so a conversation the user is reading stays quiet.
    func shouldShow(userInfo: [AnyHashable: Any]) -> Bool {
        shouldShow(data: SildPushKit.data(from: userInfo))
    }
}

// Registration takes the FCM token (`Messaging.messaging().fcmToken`), not the
// APNs one: delivery goes through the tenant's Firebase project, which addresses
// devices by registration token.
