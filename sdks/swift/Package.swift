// swift-tools-version: 5.9
import PackageDescription

// The Sild iOS SDK. `SildCore` is the Kotlin Multiplatform core shipped as a binary
// XCFramework; `Sild` is the Swift layer on top of it.
//
// On a branch the binary target points at a locally built framework, so a checkout can
// build and test without a published release:
//
//     cd ../kotlin && ./gradlew :core:assembleSildCoreReleaseXCFramework
//
// At a `v*` tag the sdk-release workflow rewrites this one target to the release asset
// (`url:` + `checksum:`) and moves the tag onto that commit, so resolving the tag from
// SPM gets a real binary package. Keep exactly one local-path binaryTarget here — the
// rewrite refuses to run otherwise.
let package = Package(
    name: "Sild",
    platforms: [.iOS(.v17)],
    products: [
        .library(name: "Sild", targets: ["Sild"]),
    ],
    dependencies: [
        // The official Centrifugo client — the same broker + protocol the inbox, the web
        // widget and the Android SDK use, so subscriptions and reconnect behave alike.
        .package(url: "https://github.com/centrifugal/centrifuge-swift.git", from: "0.6.0"),
    ],
    targets: [
        .binaryTarget(
            name: "SildCore",
            path: "../kotlin/core/build/XCFrameworks/release/SildCore.xcframework"
        ),
        .target(
            name: "Sild",
            dependencies: [
                "SildCore",
                .product(name: "SwiftCentrifuge", package: "centrifuge-swift"),
            ]
        ),
        .testTarget(name: "SildTests", dependencies: ["Sild"]),
    ]
)
