# Sild Android SDK

The native Android messenger for the Sild platform (Phase 4, `sdks/kotlin`). It is
the Kotlin counterpart of the web drop-in widget (`web/`): same backend contract,
same appearance system, same functional scope — support conversations **and** peer
conversations (direct chats between end-user parties, e.g. rider↔driver, created by
the host backend).

## Modules

| Module | What it is | Depends on |
|---|---|---|
| `:core` | Kotlin Multiplatform client (jvm + iosArm64 + iosSimulatorArm64) — REST (§4.2) + realtime policy (§5) + models + brand/theme derivation. Only the realtime socket and timestamp formatting are per-platform, so it unit-tests on a plain JVM and on the iOS simulator. | — |
| `:ui` | Jetpack Compose messenger UI, themed from the server `BrandConfig` so it renders like the web widget. | `:core` |
| `:sample` | "Acme Rides" demo app — the SDK's counterpart to `web/demo.html`. | `:ui` |

## Install (Gradle)

Published as `io.sild:sild-core` (multiplatform; Gradle picks the `-jvm` artifact for
Android) and `io.sild:sild-ui` (AAR):

```kotlin
dependencies {
    implementation("io.sild:sild-ui:0.1.1") // pulls in :core
}
```

Works on `minSdk` 24+ with no extra setup — the SDK avoids `java.time`, so no
core-library desugaring is required in your app.

## Quick start

Call `Sild.init` from `Application.onCreate`, not from an Activity: the config holds a
`TokenProvider` (not parcelable), so it lives in memory rather than in saved state. If
Android restarts your process while the messenger is in the back stack — a routine
low-memory event — an `Application.onCreate` init is back in place before the messenger
is recreated, whereas an Activity-scoped one isn't and the messenger closes itself.

```kotlin
// Once, in Application.onCreate. The token provider mints a user JWT via YOUR
// backend (which holds the Sild API key) — never ship the API key in the app.
Sild.init(
    SildConfig(
        baseUrl = "https://sild.example.com",
        tokenProvider = { myBackend.mintSildToken(currentUserId) },
        userId = currentUserId, // lets the SDK tell "you" from the other party in a peer chat
    ),
)

// Open support (starts/opens a support request):
SildMessenger.openSupportRequest(context)

// Open a peer chat your backend created (e.g. the trip's driver conversation):
SildMessenger.openConversation(context, conversationId)

// Or the conversation list:
SildMessenger.openList(context)
```

The messenger keeps its state across configuration changes (rotation, system dark-mode
toggle, locale or font-size change): the client lives in a `ViewModel`, and the open
thread plus the composer's text and attachments are restored, so nothing the user typed
is lost. No `android:configChanges` is needed in your manifest.

Peer conversations are created by the host backend via `POST /v1/conversations`
(`open_assignment: false`); the SDK opens them by id. It never creates peer chats
itself — matching the widget and the platform's design.

## Appearance

The messenger is themed from the tenant's active brand (`GET /v1/brands/active`): brand
color (accents + hover derive from it), light/dark/auto theme, corner radius, and
the welcome heading/subtext — the same `BrandConfig` the web widget consumes, so
the two surfaces look consistent. Launcher geometry (`launcherPos`/`launcherSize`/
`launcherIcon`) is web-only and intentionally ignored — a native app hosts the
messenger in its own navigation.

> Font note: the color/radius/theme system is pixel-faithful to the web. The
> typeface currently maps to the platform sans-serif rather than downloading the
> web's Google font — a deliberate first-cut simplification (see `Fonts.kt`). Drop
> a font into `res/font` and extend `familyFor()` to match a specific brand face.

## Build & test

```bash
cd sdks/kotlin

# Core tests. commonTest holds the whole suite, so the same assertions run on the JVM
# and on an iOS simulator; the REST surface (auth retry, upload grant → PUT) is covered
# offline with a mock engine (plus one JVM test over a real socket, for the headers the
# signed upload PUT must not gain), and the wire/realtime tests no-op without SILD_BASE_URL.
./gradlew :core:jvmTest :core:iosSimulatorArm64Test    # logic + offline REST tests
SILD_BASE_URL=http://localhost:8080 ./gradlew :core:jvmTest :core:iosSimulatorArm64Test
# The simulator tests need an arm64 JDK: on an x86_64 one the Kotlin plugin disables
# the task and the build still passes green.

# Build the library + sample APK. :sample:assembleRelease is minified (R8) — it is
# what validates ui/consumer-rules.pro against the shrinker a real host app runs.
./gradlew :ui:assembleRelease :sample:assembleDebug :sample:assembleRelease

# Publish the artifacts locally
./gradlew :core:publishToMavenLocal :ui:publishToMavenLocal
```

Run the sample against a local backend: start `sild-dev` (`cd backend && make dev`
or `go run ./cmd/sild-dev`), then run the `:sample` app. On the Android emulator the
backend's `localhost:8080` is reached at `10.0.2.2:8080` (see `DevBackend.kt`).

### End-to-end (emulator)

`SildMessengerE2ETest` launches the sample and round-trips messages through a live
`sild-dev` — a smoke test for launch crashes, the wire contract, and Compose
rendering. Two flows: **support** (open a request, send, assert it lands) and **peer**
(open the trip's driver chat; a driver-side `core` client receives the rider's UI
message, then replies and the reply renders in the rider's Compose thread — a
two-sided realtime round-trip on-device). It needs a running emulator/device and
backend:

```bash
# 1. backend (separate shell): reachable from the emulator at 10.0.2.2:8080
cd backend && make dev

# 2. an emulator or device attached (`adb devices` lists it), then:
cd sdks/kotlin
./gradlew :sample:connectedDebugAndroidTest
```

CI (`.github/workflows/android-sdk.yml`) runs three jobs on every change under
`sdks/kotlin`: `sdk` boots `sild-dev`, runs the JVM core tests against it, and
assembles the AAR + sample APK; `ios-core` does the same on `macos-15`, running the
identical `commonTest` suite on an iOS simulator; `e2e` boots `sild-dev` and runs the
instrumented test above on a hardware-accelerated emulator (free on `ubuntu-latest`).
