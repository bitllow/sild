import org.jetbrains.kotlin.gradle.dsl.JvmTarget

// :ui — the Android messenger UI (Jetpack Compose) that renders :core's state,
// themed from the server BrandConfig so it looks identical to the web widget.
plugins {
    id("com.android.library")
    id("org.jetbrains.kotlin.android")
    id("org.jetbrains.kotlin.plugin.compose")
    `maven-publish`
}

android {
    namespace = "io.sild.ui"
    compileSdk = 34

    defaultConfig {
        minSdk = 24
        consumerProguardFiles("consumer-rules.pro")
    }
    compileOptions {
        sourceCompatibility = JavaVersion.VERSION_17
        targetCompatibility = JavaVersion.VERSION_17
    }
    buildFeatures { compose = true }
    publishing { singleVariant("release") { withSourcesJar() } }
}

kotlin {
    compilerOptions { jvmTarget = JvmTarget.JVM_17 }
}

dependencies {
    api(project(":core"))

    val composeBom = platform("androidx.compose:compose-bom:2024.06.00")
    implementation(composeBom)
    implementation("androidx.compose.ui:ui")
    implementation("androidx.compose.foundation:foundation")
    implementation("androidx.compose.material3:material3")
    // No material-icons: SildIcons builds the widget's own Feather glyphs.
    implementation("androidx.activity:activity-compose:1.9.0")
    implementation("androidx.lifecycle:lifecycle-runtime-compose:2.8.2")
    implementation("androidx.lifecycle:lifecycle-viewmodel-ktx:2.8.2")
    implementation("io.coil-kt:coil-compose:2.6.0")
}

publishing {
    publications {
        register<MavenPublication>("release") {
            artifactId = "sild-ui"
            afterEvaluate { from(components["release"]) }
        }
    }
}
