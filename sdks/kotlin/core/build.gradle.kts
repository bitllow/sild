// :core — the pure-Kotlin/JVM Sild client: REST + realtime + models + brand.
// No Android dependencies, so it builds and tests on a plain JVM (fast CI, no
// emulator). The :ui module and any future JVM host consume this.
plugins {
    id("org.jetbrains.kotlin.jvm")
    id("org.jetbrains.kotlin.plugin.serialization")
    `java-library`
    `maven-publish`
}

java {
    sourceCompatibility = JavaVersion.VERSION_17
    targetCompatibility = JavaVersion.VERSION_17
    withSourcesJar()
}

kotlin {
    jvmToolchain(17)
}

dependencies {
    api("org.jetbrains.kotlinx:kotlinx-coroutines-core:1.8.1")
    implementation("org.jetbrains.kotlinx:kotlinx-serialization-json:1.6.3")
    implementation("com.squareup.okhttp3:okhttp:4.12.0")
    // Official Centrifugo client (WebSocket) — same realtime protocol the inbox
    // and web widget use, so subscriptions + reconnect behave identically.
    implementation("io.github.centrifugal:centrifuge-java:0.5.0")

    testImplementation("org.jetbrains.kotlin:kotlin-test")
    testImplementation("org.junit.jupiter:junit-jupiter:5.10.2")
    testImplementation("org.jetbrains.kotlinx:kotlinx-coroutines-test:1.8.1")
    // Offline REST tests (auth retry, upload grant → PUT) — see SildApiTest.
    testImplementation("com.squareup.okhttp3:mockwebserver:4.12.0")
}

publishing {
    publications {
        register<MavenPublication>("release") {
            // groupId/version come from the root allprojects block. Keep SDK_VERSION
            // (Version.kt) in step.
            artifactId = "sild-core"
            from(components["java"])
        }
    }
}

tasks.test {
    useJUnitPlatform()
    // The integration tests hit a running sild-dev; point them at it via env.
    // Skipped automatically when SILD_BASE_URL is unset (see harness).
    environment("SILD_BASE_URL", System.getenv("SILD_BASE_URL") ?: "")
    testLogging { events("passed", "failed", "skipped") }
}
