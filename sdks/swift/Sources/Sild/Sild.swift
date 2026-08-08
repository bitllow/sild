import Foundation
import SildCore
import UserNotifications

/// The SDK entry point, and the peer of the Android `Sild` object: configure once
/// at launch, then present ``SildMessenger`` and hand push over here.
///
/// Every decision below belongs to `SildHost` in the shared core; these are the
/// Swift shapes for it — `async` instead of a callback, `UNNotification` instead
/// of a payload map.
@MainActor
public enum Sild {
    /// Configure once at launch (e.g. `application(_:didFinishLaunchingWithOptions:)`).
    public static func initialize(_ config: SildConfig) { SildHost.shared.config = config }

    /// As above, from a base URL and a way to mint a token — the same pair
    /// `SildMessenger(baseURL:token:)` takes.
    public static func initialize(
        baseURL: String,
        token: @escaping () async throws -> String,
        userId: String? = nil
    ) {
        initialize(.make(baseUrl: baseURL, token: token, userId: userId))
    }

    // Push (§5.5). Your app owns its Firebase registration and the SDK adds none
    // (docs/adr/0001-host-app-owns-fcm-sdk-takes-the-token.md); it takes the FCM
    // token from you, tells its own messages apart, and offers to route them.

    /// Forwards the token your app was issued. Call it again on every rotation.
    public static func setPushToken(_ token: String) async -> Bool {
        await awaitResult { SildHost.shared.setPushToken(token: token, onResult: $0) }
    }

    /// Releases this device on sign-out. Retry until it reports true — a token that
    /// outlives the session keeps delivering the previous user's messages.
    public static func clearPushToken(_ token: String) async -> Bool {
        await awaitResult { SildHost.shared.clearPushToken(token: token, onResult: $0) }
    }

    /// Whether to present a notification that arrived while the app was in the
    /// foreground: false for the conversation the messenger is showing, and for
    /// anything that is not ours.
    public static func shouldShow(_ userInfo: [AnyHashable: Any]) -> Bool {
        SildHost.shared.shouldShow(data: SildPushKit.data(from: userInfo))
    }

    /// The optional renderer: what `willPresent` hands back, so a foreground alert
    /// looks like the one the system draws when the app is not running.
    public static func presentationOptions(
        for notification: UNNotification
    ) -> UNNotificationPresentationOptions {
        shouldShow(notification.request.content.userInfo) ? [.banner, .list, .sound] : []
    }

    private static func awaitResult(_ call: (@escaping (KotlinBoolean) -> Void) -> Void) async -> Bool {
        await withCheckedContinuation { (cont: CheckedContinuation<Bool, Never>) in
            call { cont.resume(returning: $0.boolValue) }
        }
    }
}
