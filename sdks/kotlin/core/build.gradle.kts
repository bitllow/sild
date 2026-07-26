import org.jetbrains.kotlin.gradle.targets.native.tasks.KotlinNativeSimulatorTest

// :core — the Sild client: REST + realtime policy + models + brand, shared by the
// Android :ui layer and the iOS SDK. Only the realtime socket is platform-specific;
// everything else is commonMain, so the wire contract is tested once and runs on
// a plain JVM (fast CI, no emulator) and on the iOS simulator.
plugins {
    id("org.jetbrains.kotlin.multiplatform")
    id("org.jetbrains.kotlin.plugin.serialization")
    `maven-publish`
}

fun firstAvailableIphone(): String =
    providers.exec { commandLine("xcrun", "simctl", "list", "devices", "available") }
        .standardOutput.asText.get()
        .lineSequence().map { it.trim() }
        .firstOrNull { it.startsWith("iPhone") && it.contains(" (") }
        ?.substringBefore(" (")
        ?: error("no iPhone simulator is available")

kotlin {
    jvmToolchain(17)

    jvm()
    iosArm64()
    iosSimulatorArm64 {
        // The plugin's default simulator does not exist on every Xcode, and it just
        // disables the test task when it is missing. Override with -Psild.iosDevice=... .
        if (System.getProperty("os.name").startsWith("Mac")) testRuns.configureEach {
            deviceId = findProperty("sild.iosDevice") as String? ?: firstAvailableIphone()
        }
    }

    sourceSets {
        commonMain.dependencies {
            api("org.jetbrains.kotlinx:kotlinx-coroutines-core:1.10.2")
            implementation("org.jetbrains.kotlinx:kotlinx-serialization-json:1.9.0")
            implementation("io.ktor:ktor-client-core:3.4.3")
        }
        jvmMain.dependencies {
            implementation("io.ktor:ktor-client-okhttp:3.4.3")
            // Official Centrifugo client (WebSocket) — same realtime protocol the inbox
            // and web widget use, so subscriptions + reconnect behave identically.
            implementation("io.github.centrifugal:centrifuge-java:0.5.0")
        }
        iosMain.dependencies {
            implementation("io.ktor:ktor-client-darwin:3.4.3")
            // Apple-only: on the JVM it is java.time, which Android hosts below API 26
            // cannot load without core-library desugaring.
            implementation("org.jetbrains.kotlinx:kotlinx-datetime:0.8.0")
        }
        commonTest.dependencies {
            implementation(kotlin("test"))
            implementation("org.jetbrains.kotlinx:kotlinx-coroutines-test:1.10.2")
            // Offline REST tests (auth retry, upload grant → PUT) — see SildApiTest.
            implementation("io.ktor:ktor-client-mock:3.4.3")
        }
        jvmTest.dependencies {
            // The mock engine cannot show what the OkHttp engine puts on the wire —
            // see SildApiWireTest.
            // Matches the OkHttp the Ktor engine resolves; 4.x fails on it at runtime.
            implementation("com.squareup.okhttp3:mockwebserver:5.3.2")
        }
    }
}

publishing {
    publications.withType<MavenPublication>().configureEach {
        // groupId/version come from the root allprojects block. Keep SDK_VERSION
        // (Version.kt) in step. Per-target publications keep their own suffix.
        artifactId = artifactId.replace(project.name, "sild-core")
    }
}

// The integration tests hit a running sild-dev; point them at it via env.
// Unset → they no-op (see Dev.baseUrl), so an offline run stays green.
val sildBaseUrl: String = System.getenv("SILD_BASE_URL") ?: ""

tasks.named<Test>("jvmTest") {
    useJUnitPlatform()
    environment("SILD_BASE_URL", sildBaseUrl)
    testLogging { events("passed", "failed", "skipped") }
}

tasks.withType<KotlinNativeSimulatorTest>().configureEach {
    // simctl only forwards variables to the spawned test binary under this prefix.
    environment("SIMCTL_CHILD_SILD_BASE_URL", sildBaseUrl)
    testLogging { events("passed", "failed", "skipped") }
}
