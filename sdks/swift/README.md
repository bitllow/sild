# Sild iOS SDK

Swift package wrapping `SildCore.xcframework` — the same Kotlin Multiplatform `:core`
the Android SDK uses (`sdks/kotlin`), compiled for `ios-arm64` and `ios-arm64-simulator`
— plus the SwiftUI messenger and the Centrifugo realtime transport on top of it.

The manifest lives at the **repository root** (`/Package.swift`), because SPM only reads
a manifest there; the sources stay here under `sdks/swift/` via `path:` targets.

## Install

```swift
.package(url: "https://github.com/bitllow/sild.git", from: "0.1.2")
```

One import is enough — `Sild` re-exports the core module, so `SildConfig`, `SildState`
and the rest are in scope without a second import.

The block below is what the release workflow compiles against the published tag, as a
throwaway consumer package. Keep the two in sync: everything between the markers is
extracted verbatim.

<!-- consumer-smoke:start -->
```swift
import SwiftUI
import Sild

struct SupportButton: View {
    @State private var showing = false

    var body: some View {
        Button("Support") { showing = true }
            .sheet(isPresented: $showing) {
                SildMessenger(
                    baseURL: "https://api.example.com",
                    token: { try await mintToken() },
                    target: .support
                ) { showing = false }
            }
    }
}

// Your backend mints the user JWT; the SDK never sees your API key.
func mintToken() async throws -> String { "jwt-from-your-backend" }

// Core types reach a host through `Sild` alone.
func isLive(_ state: SildState, _ config: SildConfig) -> Bool {
    state.connection == ConnectionState.connected && !config.base.isEmpty
}
```
<!-- consumer-smoke:end -->

`SildConfig.make(baseUrl:token:)` builds a config directly when a host needs one; every
optional field keeps the SDK default, which is declared once on the Kotlin side.

## Push notifications

Your app keeps its own Firebase integration and the SDK adds none
(`docs/adr/0001-host-app-owns-fcm-sdk-takes-the-token.md`). Registration takes the
**FCM** token — delivery goes through the tenant's Firebase project, which
addresses devices by registration token, not the raw APNs one.

```swift
// Once at launch — the peer of the Android `Sild.init`. Presenting the messenger
// still takes a config directly; this is for the calls that happen without one.
Sild.initialize(baseURL: base, token: { try await mintToken() })

// Whenever FCM issues or rotates the token:
await Sild.setPushToken(token)

// On sign-out. Retry until it returns true, or that device keeps receiving.
await Sild.clearPushToken(token)

// In UNUserNotificationCenterDelegate — the foreground path:
func userNotificationCenter(
    _ center: UNUserNotificationCenter,
    willPresent notification: UNNotification
) async -> UNNotificationPresentationOptions {
    guard SildPushKit.isSildPush(notification.request.content.userInfo) else { return myOwn() }
    return Sild.presentationOptions(for: notification)
}

// And the tap:
if let nudge = SildPushKit.parse(response.notification.request.content.userInfo) {
    open(.conversation(id: nudge.conversationId))
}
```

`presentationOptions` is the optional renderer: the system draws the alert exactly
as it would have in the background, and hands back nothing when the messenger is
on screen showing that conversation. Deciding yourself is equally supported —
`Sild.shouldShow(userInfo:)` is the same answer without the options. `SildPushKit`
stays the payload half (`isSildPush`, `parse`), the peer of the Android `SildPush`
object; the payload carries the conversation id, its kind and the unread count, so
a tap routes with no round-trip.

## Requirements

**iOS 17+.** The messenger is built on `@Observable`, `TextField(axis:)` and the
two-parameter `onChange`. Dropping to iOS 14 means `ObservableObject` and shims for
those three; it is a deliberate floor for a greenfield SDK, not an accident.

## Layout

| Target | What it is |
|---|---|
| `SildCore` | binary target — the XCFramework, built by Gradle |
| `Sild` | the Swift layer: `SildMessenger`, `SildModel`, the Centrifugo transport |
| `SildTests` | XCTest running on the simulator against a live `sild-dev` |

## Build & test

On a branch the `binaryTarget` points at a locally built framework, so **assemble it
first** — without it, resolving the package fails on the missing artifact:

```bash
./gradlew -p sdks/kotlin :core:assembleSildCoreReleaseXCFramework
```

At a `v*` tag the target instead points at the release asset, so consumers need no
Gradle. Only path-based/local development has this step.

The tests need a running `sild-dev` (`cd backend && make dev`) and read `SILD_BASE_URL`.
`xcodebuild` does not forward the shell environment into the simulator, so publish the
variable on the booted device instead:

```bash
udid=$(xcrun simctl list devices available | grep -m1 -o '[0-9A-F-]\{36\}')
xcrun simctl bootstatus "$udid" -b
xcrun simctl spawn "$udid" launchctl setenv SILD_BASE_URL http://localhost:8080
xcodebuild test -scheme Sild -destination "platform=iOS Simulator,id=$udid"
```

The package is iOS-only, so a bare `swift build`/`swift test` at the repo root will not
work — everything goes through an iOS simulator destination.

Without `SILD_BASE_URL` the tests report as skipped rather than passing silently.

## Releases

`sdk-release.yml` runs on a `v*` tag: it checks the tag against the Gradle version and
`SDK_VERSION`, builds the XCFramework, zips it reproducibly, rewrites the `binaryTarget`
to the release `url:` + `checksum:`, commits that, moves the tag onto the new commit,
and publishes the release with the zip attached.

So `Package.swift` on a branch uses the local path, and at a tag it points at the asset
— `.package(url:from:)` against a tag resolves to a real binary package. Nothing to
paste by hand.

## Sample app + UI tests

`project.yml` is an [xcodegen](https://github.com/yonaskolb/XcodeGen) spec for the
"Acme Rides" sample and its XCUITest — targets SwiftPM cannot express. The `.xcodeproj`
is generated, not committed:

```bash
brew install xcodegen
xcodegen generate --spec sdks/swift/project.yml
xcodebuild test -project sdks/swift/SildSample.xcodeproj -scheme SildSample \
  -destination "platform=iOS Simulator,id=$udid"
```

That scheme runs both suites: `SildUnitTests` (transport seams, state bridge, REST
smoke) and `SildSampleUITests`, which drives the real messenger and asserts a driver's
message arrives over realtime as a rendered bubble — the iOS peer of the Android
instrumented `SildMessengerE2ETest`.
