# Sild iOS SDK

Swift package wrapping `SildCore.xcframework` — the same Kotlin Multiplatform `:core`
the Android SDK uses (`sdks/kotlin`), compiled for `ios-arm64` and `ios-arm64-simulator`.
Only the REST/state layer is wired up so far; the SwiftUI messenger and the Centrifugo
`RealtimeTransport` are still to come.

## Requirements

**iOS 17+.** The messenger is built on `@Observable`, `TextField(axis:)` and the
two-parameter `onChange`. Dropping to iOS 14 means `ObservableObject` and shims for
those three; it is a deliberate floor for a greenfield SDK, not an accident.

## Layout

| Target | What it is |
|---|---|
| `SildCore` | binary target — the XCFramework, built by Gradle |
| `Sild` | the Swift layer; today a `SildCoreSmoke` helper proving the interop |
| `SildTests` | XCTest running on the simulator against a live `sild-dev` |

## Build & test

The `binaryTarget` points at a locally built framework, so build it first:

```bash
cd ../kotlin && ./gradlew :core:assembleSildCoreReleaseXCFramework
```

The tests need a running `sild-dev` (`cd backend && make dev`) and read `SILD_BASE_URL`.
`xcodebuild` does not forward the shell environment into the simulator, so publish the
variable on the booted device instead:

```bash
udid=$(xcrun simctl list devices available | grep -m1 -o '[0-9A-F-]\{36\}')
xcrun simctl bootstatus "$udid" -b
xcrun simctl spawn "$udid" launchctl setenv SILD_BASE_URL http://localhost:8080
xcodebuild test -scheme Sild -destination "platform=iOS Simulator,id=$udid"
```

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
xcodegen generate
xcodebuild test -project SildSample.xcodeproj -scheme SildSample \
  -destination "platform=iOS Simulator,id=$udid"
```

That scheme runs both suites: `SildUnitTests` (transport seams, state bridge, REST
smoke) and `SildSampleUITests`, which drives the real messenger and asserts a driver's
message arrives over realtime as a rendered bubble — the iOS peer of the Android
instrumented `SildMessengerE2ETest`.
