# Sild Android SDK

The native Android messenger for the Sild platform (Phase 4, `sdks/kotlin`). It is
the Kotlin counterpart of the web drop-in widget (`web/`): same backend contract,
same appearance system, same functional scope — support conversations **and** peer
conversations (direct chats between end-user parties, e.g. rider↔driver, created by
the host backend).

## Modules

| Module | What it is | Depends on |
|---|---|---|
| `:core` | Pure-Kotlin/JVM client — REST (§4.2) + realtime (§5, Centrifuge over WebSocket) + models + brand/theme derivation. No Android types, so it unit-tests on a plain JVM. | — |
| `:ui` | Jetpack Compose messenger UI, themed from the server `BrandConfig` so it renders like the web widget. | `:core` |
| `:sample` | "Acme Rides" demo app — the SDK's counterpart to `web/demo.html`. | `:ui` |

## Install (Gradle)

Published as `io.sild:sild-core` (JAR) and `io.sild:sild-ui` (AAR):

```kotlin
dependencies {
    implementation("io.sild:sild-ui:0.1.0") // pulls in :core
}
```

## Quick start

```kotlin
// Once, e.g. in Application.onCreate. The token provider mints a user JWT via YOUR
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

Peer conversations are created by the host backend via `POST /v1/conversations`
(`open_assignment: false`); the SDK opens them by id. It never creates peer chats
itself — matching the widget and the platform's design.

## Appearance

The messenger is themed from the tenant's active brand (`GET /v1/me/brand`): brand
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

# Pure-JVM core tests. Integration tests need a running sild-dev; without
# SILD_BASE_URL only the offline logic tests run.
./gradlew :core:test                                   # logic tests only
SILD_BASE_URL=http://localhost:8080 ./gradlew :core:test   # + wire/realtime tests

# Build the library + sample APK
./gradlew :ui:assembleRelease :sample:assembleDebug

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

CI (`.github/workflows/android-sdk.yml`) runs two jobs on every change under
`sdks/kotlin`: `sdk` boots `sild-dev`, runs the core tests against it, and assembles
the AAR + sample APK; `e2e` boots `sild-dev` and runs the instrumented test above on
a hardware-accelerated emulator (free on `ubuntu-latest`).
