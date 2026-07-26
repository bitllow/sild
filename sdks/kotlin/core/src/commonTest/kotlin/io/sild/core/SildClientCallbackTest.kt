package io.sild.core

import kotlinx.coroutines.CompletableDeferred
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.cancel
import kotlinx.coroutines.delay
import kotlinx.coroutines.withTimeout
import kotlin.test.AfterTest
import kotlin.test.Test
import kotlin.test.assertFalse
import kotlin.test.assertNull
import kotlin.test.assertTrue

// Offline tests for the send/create result-callback contract that lets the UI clear
// a draft ONLY once delivery is confirmed. A token provider that throws makes every
// REST call fail before touching the network, so failures are exercised deterministically
// with no backend.
class SildClientCallbackTest {
    private val scope = CoroutineScope(Dispatchers.Default + SupervisorJob())

    @AfterTest fun tearDown() {
        scope.cancel()
    }

    // baseUrl is never contacted — the token provider throws first, failing the call.
    private fun failingClient() = SildClient(
        SildConfig(
            baseUrl = "http://127.0.0.1:1",
            tokenProvider = TokenProvider { throw RuntimeException("no token") },
            userId = "u_test",
        ),
        scope,
    )

    /** Open [id] and wait until it is the active thread (loadThread sets activeId before
     *  its network call, so this holds even when the backend is unreachable). */
    private suspend fun SildClient.openAndAwaitActive(id: String) {
        openConversation(id)
        withTimeout(2_000) { while (state.value.activeId != id) delay(20) }
    }

    @Test fun sendWithNoActiveThreadReportsFailure() = runBlockingTest {
        val client = failingClient()
        val result = CompletableDeferred<Boolean>()
        client.send("hello") { result.complete(it) }
        assertFalse(withTimeout(2_000) { result.await() }, "no active conversation → failure")
        assertTrue(client.state.value.messages.isEmpty())
    }

    @Test fun emptySendReportsFailure() = runBlockingTest {
        val client = failingClient()
        client.openAndAwaitActive("c1")
        val result = CompletableDeferred<Boolean>()
        client.send("   ") { result.complete(it) }
        assertFalse(withTimeout(2_000) { result.await() }, "blank body with no attachments → failure")
    }

    @Test fun failedSendReportsFailureAndAppendsNothing() = runBlockingTest {
        val client = failingClient()
        client.openAndAwaitActive("c1")
        val result = CompletableDeferred<Boolean>()
        client.send("hi there") { result.complete(it) }
        assertFalse(withTimeout(2_000) { result.await() }, "a failed POST must report failure so the draft is kept")
        // Nothing was optimistically appended, and an error was recorded.
        assertTrue(client.state.value.messages.none { it.body == "hi there" })
        assertTrue(client.state.value.error != null)
    }

    @Test fun failedSupportRequestReportsNull() = runBlockingTest {
        val client = failingClient()
        val result = CompletableDeferred<String?>()
        client.openSupportRequest { result.complete(it) }
        assertNull(withTimeout(2_000) { result.await() }, "creation failure must report null so the composer resets")
    }
}
