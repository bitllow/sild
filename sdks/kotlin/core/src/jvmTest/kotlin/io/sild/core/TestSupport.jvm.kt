package io.sild.core

import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.runBlocking

internal actual fun envVar(name: String): String? = System.getenv(name)

internal actual fun runBlockingTest(block: suspend CoroutineScope.() -> Unit) = runBlocking(block = block)
