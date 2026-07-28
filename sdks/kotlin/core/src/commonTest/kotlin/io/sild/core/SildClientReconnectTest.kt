package io.sild.core

import io.ktor.client.HttpClient
import io.ktor.client.engine.mock.MockEngine
import io.ktor.client.engine.mock.respond
import io.ktor.http.HttpStatusCode
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.cancel
import kotlin.test.AfterTest
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertTrue

// The ?since= catch-up is covered at the API layer, but nothing exercised the TRIGGER:
// the CONNECTED transition that fires it. A regression that stopped catch-up happening
// at all would leave messages sent during an outage invisible until the thread is
// reopened — the exact bug it was added to fix — with every other test still green.
//
// The transport is injectable, so this drives the connection state directly instead of
// needing an instrumented test with a real socket.
class SildClientReconnectTest {
    private val scope = CoroutineScope(Dispatchers.Default + SupervisorJob())
    private val requests = mutableListOf<String>()

    @AfterTest fun tearDown() {
        scope.cancel()
    }

    /** A transport that reports nothing on its own, so the test owns every transition. */
    private class FakeTransport : RealtimeTransport {
        var onConnection: ((ConnectionState) -> Unit)? = null
        override fun connect() {}
        override fun reconnect() {}
        override fun destroy() {}
    }

    private fun client(routes: Map<String, String>, fake: FakeTransport): SildClient {
        val engine = MockEngine { request ->
            val url = request.url
            requests += url.encodedPath + (url.parameters["since"]?.let { "?since=$it" } ?: "")
            val body = routes[url.encodedPath + (url.parameters["since"]?.let { "?since=$it" } ?: "")]
                ?: routes[url.encodedPath]
                ?: when (url.encodedPath) {
                    "/v1/conversations" -> """{"items":[],"next_cursor":null,"has_more":false}"""
                    "/v1/brands/active" -> """{"name":"Acme","config":{}}"""
                    "/v1/conversations/c1" -> """{"id":"c1","status":"open","members":[]}"""
                    else -> return@MockEngine respond("", HttpStatusCode.NotFound)
                }
            respond(body, HttpStatusCode.OK)
        }
        val cfg = SildConfig(
            baseUrl = "http://api.test",
            tokenProvider = TokenProvider { "tok" },
            userId = "u_rider",
        )
        val factory = RealtimeTransportFactory { _, _, onConnection, _ ->
            fake.onConnection = onConnection
            fake
        }
        return SildClient(cfg, scope, {}, factory, SildApi(cfg, HttpClient(engine) { sildDefaults() }))
    }

    private fun msg(id: String, at: String) =
        """{"id":"$id","sender_kind":"user","external_user_id":"u_rider","body":"$id","created_at":"$at"}"""

    @Test fun reconnectResumesFromTheLastMessageHeldAndAppendsTheGap() = runBlockingTest {
        val fake = FakeTransport()
        val client = client(
            mapOf(
                "/v1/conversations/c1/messages" to
                    """{"items":[${msg("m1", "2026-01-01T00:00:01Z")}],"next_cursor":null,"has_more":false}""",
                "/v1/conversations/c1/messages?since=m1" to
                    """{"items":[${msg("m2", "2026-01-01T00:00:02Z")},${msg("m3", "2026-01-01T00:00:03Z")}],"next_cursor":null,"has_more":false}""",
            ),
            fake,
        )

        client.openConversation("c1")
        awaitUntil(timeoutMs = 3_000, get = { client.state.value }) { it.messages.size == 1 }

        // An outage, then the socket comes back — the transition catch-up hangs off.
        fake.onConnection?.invoke(ConnectionState.CONNECTED)
        fake.onConnection?.invoke(ConnectionState.DISCONNECTED)
        requests.clear()
        fake.onConnection?.invoke(ConnectionState.CONNECTED)

        val s = awaitUntil(timeoutMs = 3_000, get = { client.state.value }) { it.messages.size == 3 }
        assertEquals(
            listOf("m1", "m2", "m3"), s.messages.map { it.id },
            "the gap is appended in order after what was already held",
        )
        assertTrue(
            requests.any { it == "/v1/conversations/c1/messages?since=m1" },
            "catch-up resumed from the last id held, not from the start: $requests",
        )
    }

    // has_more means "call again with the last id you got". Stopping after one page
    // silently drops the remainder of a longer outage.
    @Test fun reconnectDrainsEveryPageOfTheGap() = runBlockingTest {
        val fake = FakeTransport()
        val client = client(
            mapOf(
                "/v1/conversations/c1/messages" to
                    """{"items":[${msg("m1", "2026-01-01T00:00:01Z")}],"next_cursor":null,"has_more":false}""",
                "/v1/conversations/c1/messages?since=m1" to
                    """{"items":[${msg("m2", "2026-01-01T00:00:02Z")}],"next_cursor":null,"has_more":true}""",
                "/v1/conversations/c1/messages?since=m2" to
                    """{"items":[${msg("m3", "2026-01-01T00:00:03Z")}],"next_cursor":null,"has_more":false}""",
            ),
            fake,
        )

        client.openConversation("c1")
        awaitUntil(timeoutMs = 3_000, get = { client.state.value }) { it.messages.size == 1 }

        fake.onConnection?.invoke(ConnectionState.CONNECTED)
        fake.onConnection?.invoke(ConnectionState.DISCONNECTED)
        fake.onConnection?.invoke(ConnectionState.CONNECTED)

        val s = awaitUntil(timeoutMs = 3_000, get = { client.state.value }) { it.messages.size == 3 }
        assertEquals(listOf("m1", "m2", "m3"), s.messages.map { it.id })
        assertTrue(
            requests.any { it == "/v1/conversations/c1/messages?since=m2" },
            "the second page was requested from the last id of the first: $requests",
        )
    }
}
