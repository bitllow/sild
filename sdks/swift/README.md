# Sild iOS SDK

Swift package wrapping `SildCore.xcframework` — the same Kotlin Multiplatform `:core`
the Android SDK uses (`sdks/kotlin`), compiled for `ios-arm64` and `ios-arm64-simulator`
— plus the SwiftUI messenger and the Centrifugo realtime transport on top of it.

The manifest lives at the **repository root** (`/Package.swift`), because SPM only reads
a manifest there; the sources stay here under `sdks/swift/` via `path:` targets.

## Install

```swift
.package(url: "https://github.com/bitllow/sild.git", from: "0.1.1")
```

```swift
import Sild

SildMessenger(config: config, target: .support) { dismiss() }
```

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
