package io.sild.core

import kotlinx.cinterop.ExperimentalForeignApi
import kotlinx.cinterop.toKString
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.runBlocking
import platform.posix.getenv

// simctl only forwards SIMCTL_CHILD_-prefixed variables into the simulator, and it
// strips the prefix — so the test binary sees the plain name.
@OptIn(ExperimentalForeignApi::class)
internal actual fun envVar(name: String): String? = getenv(name)?.toKString()

internal actual fun runBlockingTest(block: suspend CoroutineScope.() -> Unit) = runBlocking(block = block)
