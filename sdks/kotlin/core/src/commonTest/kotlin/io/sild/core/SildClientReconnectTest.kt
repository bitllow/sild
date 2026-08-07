package io.sild.core

import io.ktor.client.HttpClient
import io.ktor.client.engine.mock.MockEngine
import io.ktor.client.engine.mock.respond
import io.ktor.http.HttpStatusCode
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.cancel
import kotlinx.coroutines.delay
import kotlin.test.AfterTest
import kotlin.test.Test
import kotlin.test.assertEquals

// The transport is injectable, so the CONNECTED transition catch-up hangs off can be
// driven directly rather than through a real socket.
class SildClientReconnectTest {
    private val scope = CoroutineScope(Dispatchers.Default + SupervisorJob())
    private val requests = mutableListOf<String>()
    private val routes = mutableMapOf<String, String>()
    private val fake = FakeTransport()

    @AfterTest fun tearDown() {
        scope.cancel()
    }

    /** A transport that reports nothing on its own, so the test owns every transition. */
    private class FakeTransport : RealtimeTransport {
        var onConnection: (ConnectionState) -> Unit = {}
        override fun connect() {}
        override fun reconnect() {}
        override fun destroy() {}
    }

    /** A client whose thread endpoint answers per ?since= value, not per path. */
    private fun client(): SildClient {
        val engine = MockEngine { request ->
            val url = request.url
            val since = url.parameters["since"]
            val key = url.encodedPath + (since?.let { "?since=$it" } ?: "")
            requests += key
            // An unscripted ?since= is a resume from the wrong id — 404 rather than the
            // pathless route, which would answer it with the thread's first page.
            if (since != null && key !in routes) return@MockEngine respond("", HttpStatusCode.NotFound)
            val body = routes[key]
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

    private fun msg(n: Int) =
        """{"id":"m$n","sender_kind":"user","external_user_id":"u_rider","body":"m$n","created_at":"2026-01-01T00:00:0${n}Z"}"""

    private fun page(vararg items: String, more: Boolean = false) =
        """{"items":[${items.joinToString(",")}],"next_cursor":null,"has_more":$more}"""

    // has_more means "call again with the last id you got".
    @Test fun reconnectResumesFromTheLastMessageHeldAndDrainsEveryPage() = runBlockingTest {
        routes["/v1/conversations/c1/messages"] = page(msg(1))
        val client = client()

        client.openConversation("c1")
        awaitUntil(timeoutMs = 3_000, get = { client.state.value }) { it.messages.size == 1 }

        fake.onConnection(ConnectionState.DISCONNECTED)
        // The gap only becomes servable once the socket is down, so nothing but the
        // reconnect can be what fetched it.
        routes["/v1/conversations/c1/messages?since=m1"] = page(msg(2), more = true)
        routes["/v1/conversations/c1/messages?since=m2"] = page(msg(3))
        assertEquals(
            listOf("m1"), client.state.value.messages.map { it.id },
            "the outage gap is still missing before the reconnect",
        )

        fake.onConnection(ConnectionState.CONNECTED)

        val s = awaitUntil(timeoutMs = 3_000, get = { client.state.value }) { it.messages.size == 3 }
        assertEquals(
            listOf("m1", "m2", "m3"), s.messages.map { it.id },
            "the gap is appended in order after what was already held",
        )
        assertEquals(
            listOf("/v1/conversations/c1/messages?since=m1", "/v1/conversations/c1/messages?since=m2"),
            requests.filter { "since=" in it },
            "catch-up resumed from the last id held, then from the last id of that page",
        )
    }

    // The trigger is the transition, not the state: a transport that re-announces a
    // connection it already reported must not make the client refetch.
    @Test fun aConnectedRepeatedOnALiveConnectionDoesNotCatchUpAgain() = runBlockingTest {
        routes["/v1/conversations/c1/messages"] = page(msg(1))
        routes["/v1/conversations/c1/messages?since=m1"] = page(msg(2))
        val client = client()

        client.openConversation("c1")
        awaitUntil(timeoutMs = 3_000, get = { client.state.value }) { it.messages.size == 1 }

        fake.onConnection(ConnectionState.CONNECTED)
        awaitUntil(timeoutMs = 3_000, get = { client.state.value }) { it.messages.size == 2 }

        fake.onConnection(ConnectionState.CONNECTED)
        delay(200) // absence needs a window; the mock engine answers immediately
        assertEquals(
            listOf("/v1/conversations/c1/messages?since=m1"),
            requests.filter { "since=" in it },
            "only the transition into CONNECTED catches up",
        )
    }
}
