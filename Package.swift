// swift-tools-version: 5.9
import PackageDescription

// The Sild iOS SDK: `SildCore` is the Kotlin Multiplatform core as a binary
// XCFramework, `Sild` the Swift layer on top. The manifest must sit at the repo root
// for SPM to find it; the sources stay under sdks/swift/ via `path:`.
//
// On a branch the binary target points at a locally built framework:
//
//     ./gradlew -p sdks/kotlin :core:assembleSildCoreReleaseXCFramework
//
// At a `v*` tag sdk-release rewrites it to the release asset. Keep exactly one
// local-path binaryTarget — the rewrite refuses to run otherwise.
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
            path: "sdks/kotlin/core/build/XCFrameworks/release/SildCore.xcframework"
        ),
        .target(
            name: "Sild",
            dependencies: [
                "SildCore",
                .product(name: "SwiftCentrifuge", package: "centrifuge-swift"),
            ],
            path: "sdks/swift/Sources/Sild"
        ),
        .testTarget(
            name: "SildTests",
            dependencies: ["Sild"],
            path: "sdks/swift/Tests/SildTests"
        ),
    ]
)
