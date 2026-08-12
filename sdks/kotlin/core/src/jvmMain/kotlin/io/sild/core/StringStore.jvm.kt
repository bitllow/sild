package io.sild.core

// No ambient store on the JVM: an Android app's lives behind a Context, and writing
// to the working directory would be wrong on every host. `Sild.init(context, config)`
// supplies one on Android; without it, downloaded text lives for the process.
internal actual fun platformStringStore(): SildStringStore? = null
