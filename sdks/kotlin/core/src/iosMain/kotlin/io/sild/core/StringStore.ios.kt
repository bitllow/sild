package io.sild.core

import platform.Foundation.NSUserDefaults

// NSUserDefaults is the platform's own small-value store, and survives the app being
// killed — which is what "activates at next start" needs to mean anything.
private val userDefaults = object : SildStringStore {
    override fun read(key: String): String? = NSUserDefaults.standardUserDefaults.stringForKey(key)
    override fun write(key: String, value: String) {
        NSUserDefaults.standardUserDefaults.setObject(value, key)
    }
}

// One per process: two clients holding different objects for one store would each
// keep their own copy of what the other downloaded.
internal actual fun platformStringStore(): SildStringStore? = userDefaults
