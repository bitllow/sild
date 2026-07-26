package io.sild.core

// "android", not "jvm": the JVM variant is what Android hosts consume, and existing
// server-side traffic is already tagged this way.
internal actual val SDK_PLATFORM: String = "android"
