// Root build — plugins are declared here (apply false) and applied per module,
// so every module shares one set of plugin versions.
plugins {
    id("com.android.library") version "8.7.3" apply false
    id("com.android.application") version "8.7.3" apply false
    id("org.jetbrains.kotlin.android") version "2.3.21" apply false
    id("org.jetbrains.kotlin.multiplatform") version "2.3.21" apply false
    id("org.jetbrains.kotlin.plugin.compose") version "2.3.21" apply false
    id("org.jetbrains.kotlin.plugin.serialization") version "2.3.21" apply false
}

// One source of truth for the published coordinates; keep SDK_VERSION in step.
allprojects {
    group = "io.sild"
    version = "0.1.0"
}
