package io.sild.core

import io.ktor.client.HttpClient
import io.ktor.client.engine.mock.MockEngine
import io.ktor.client.engine.mock.respond
import io.ktor.http.HttpStatusCode
import kotlinx.coroutines.CompletableDeferred
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.cancel
import kotlinx.coroutines.delay
import kotlin.test.AfterTest
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertNull
import kotlin.test.assertTrue

class SildClientHistoryTest {
    private val scope = CoroutineScope(Dispatchers.Default + SupervisorJob())
    private val cursors = mutableListOf<String?>()

    @AfterTest fun tearDown() {
        scope.cancel()
    }

    /** A client whose thread endpoint answers per CURSOR, not per path — the same URL
     *  serves the newest page and each older one. */
    private fun client(pages: Map<String?, String>): SildClient {
        val engine = MockEngine { request ->
            val path = request.url.encodedPath
            if (!path.endsWith("/messages")) {
                val body = when (path) {
                    "/v1/conversations" -> """{"items":[],"next_cursor":null,"has_more":false}"""
                    "/v1/brands/active" -> """{"name":"Acme","config":{}}"""
                    "/v1/conversations/c1" -> """{"id":"c1","status":"open","members":[]}"""
                    else -> return@MockEngine respond("", HttpStatusCode.NotFound)
                }
                return@MockEngine respond(body, HttpStatusCode.OK)
            }
            val cursor = request.url.parameters["cursor"]
            cursors += cursor
            respond(pages.getValue(cursor), HttpStatusCode.OK)
        }
        val cfg = SildConfig(
            baseUrl = "http://api.test",
            tokenProvider = TokenProvider { "tok" },
            userId = "u_rider",
        )
        return SildClient(cfg, scope, {}, null, SildApi(cfg, HttpClient(engine) { sildDefaults() }))
    }

    private fun msg(id: String, at: String) =
        """{"id":"$id","sender_kind":"user","external_user_id":"u_rider","body":"$id","created_at":"$at"}"""

    @Test fun loadOlderPrependsOlderMessagesAndAdvancesTheCursor() = runBlockingTest {
        val client = client(
            mapOf(
                null to """{"items":[${msg("m5", "2026-01-01T00:00:05Z")},${msg("m6", "2026-01-01T00:00:06Z")}],"next_cursor":"cur_m5","has_more":true}""",
                "cur_m5" to """{"items":[${msg("m3", "2026-01-01T00:00:03Z")},${msg("m4", "2026-01-01T00:00:04Z")}],"next_cursor":"cur_m3","has_more":true}""",
                "cur_m3" to """{"items":[${msg("m1", "2026-01-01T00:00:01Z")},${msg("m2", "2026-01-01T00:00:02Z")}],"next_cursor":null,"has_more":false}""",
            ),
        )

        client.openConversation("c1")
        var s = awaitUntil(timeoutMs = 3_000, get = { client.state.value }) { it.messages.size == 2 }
        assertEquals("cur_m5", s.olderCursor, "the first page's cursor is recorded, or nothing can load older")

        client.loadOlder()
        s = awaitUntil(timeoutMs = 3_000, get = { client.state.value }) { it.messages.size == 4 }
        assertEquals(
            listOf("m3", "m4", "m5", "m6"), s.messages.map { it.id },
            "older messages are PREPENDED in order — appending would show history after the present",
        )
        assertEquals("cur_m3", s.olderCursor, "the cursor advanced, so the next call reaches further back")

        client.loadOlder()
        s = awaitUntil(timeoutMs = 3_000, get = { client.state.value }) { it.messages.size == 6 }
        assertEquals(listOf("m1", "m2", "m3", "m4", "m5", "m6"), s.messages.map { it.id })
        assertNull(s.olderCursor, "a whole thread offers nothing older")
        assertEquals(listOf(null, "cur_m5", "cur_m3"), cursors, "each request carried the previous page's cursor")
    }

    @Test fun loadOlderDoesNotDuplicateMessagesAlreadyHeld() = runBlockingTest {
        val client = client(
            mapOf(
                null to """{"items":[${msg("m2", "2026-01-01T00:00:02Z")}],"next_cursor":"cur_m2","has_more":true}""",
                "cur_m2" to """{"items":[${msg("m1", "2026-01-01T00:00:01Z")},${msg("m2", "2026-01-01T00:00:02Z")}],"next_cursor":null,"has_more":false}""",
            ),
        )

        client.openConversation("c1")
        awaitUntil(timeoutMs = 3_000, get = { client.state.value }) { it.messages.size == 1 }

        client.loadOlder()
        val s = awaitUntil(timeoutMs = 3_000, get = { client.state.value }) { it.olderCursor == null }
        assertEquals(listOf("m1", "m2"), s.messages.map { it.id }, "the overlapping message appears once")
    }

    // A cursorless call would re-fetch the newest page and look like a reload.
    @Test fun loadOlderIsANoOpOnAWholeThread() = runBlockingTest {
        val client = client(
            mapOf(null to """{"items":[${msg("m1", "2026-01-01T00:00:01Z")}],"next_cursor":null,"has_more":false}"""),
        )

        client.openConversation("c1")
        awaitUntil(timeoutMs = 3_000, get = { client.state.value }) { it.messages.size == 1 }
        val before = cursors.size

        client.loadOlder()
        val s = awaitUntil(timeoutMs = 3_000, get = { client.state.value }) { !it.loadingOlder }
        assertEquals(1, s.messages.size)
        assertEquals(before, cursors.size, "no request was issued")
        assertTrue(s.olderCursor == null)
    }

    @Test fun anOlderPageArrivingAfterAThreadSwitchIsDropped() = runBlockingTest {
        val release = CompletableDeferred<Unit>()
        val engine = MockEngine { request ->
            val path = request.url.encodedPath
            val body = when (path) {
                "/v1/conversations" -> """{"items":[],"next_cursor":null,"has_more":false}"""
                "/v1/brands/active" -> """{"name":"Acme","config":{}}"""
                "/v1/conversations/c1" -> """{"id":"c1","status":"open","members":[]}"""
                "/v1/conversations/c2" -> """{"id":"c2","status":"open","members":[]}"""
                "/v1/conversations/c1/messages" ->
                    if (request.url.parameters["cursor"] == null) {
                        """{"items":[${msg("a2", "2026-01-01T00:00:02Z")}],"next_cursor":"cur_a2","has_more":true}"""
                    } else {
                        release.await()
                        cursors += "cur_a2"
                        """{"items":[${msg("a1", "2026-01-01T00:00:01Z")}],"next_cursor":null,"has_more":false}"""
                    }
                "/v1/conversations/c2/messages" ->
                    if (request.url.parameters["cursor"] == null) {
                        """{"items":[${msg("b2", "2026-01-02T00:00:02Z")}],"next_cursor":"cur_b2","has_more":true}"""
                    } else {
                        """{"items":[${msg("b1", "2026-01-02T00:00:01Z")}],"next_cursor":null,"has_more":false}"""
                    }
                else -> return@MockEngine respond("", HttpStatusCode.NotFound)
            }
            respond(body, HttpStatusCode.OK)
        }
        val cfg = SildConfig(baseUrl = "http://api.test", tokenProvider = TokenProvider { "tok" }, userId = "u_rider")
        val client = SildClient(cfg, scope, {}, null, SildApi(cfg, HttpClient(engine) { sildDefaults() }))

        client.openConversation("c1")
        awaitUntil(timeoutMs = 3_000, get = { client.state.value }) { it.olderCursor == "cur_a2" }
        client.loadOlder()
        awaitUntil(timeoutMs = 3_000, get = { client.state.value }) { it.loadingOlder }

        client.openConversation("c2")
        awaitUntil(timeoutMs = 3_000, get = { client.state.value }) { it.olderCursor == "cur_b2" }
        release.complete(Unit)
        awaitUntil(timeoutMs = 3_000, get = { cursors }) { "cur_a2" in it }
        delay(200)

        var s = client.state.value
        assertEquals(listOf("b2"), s.messages.map { it.id }, "c1's history stayed out of c2")
        assertEquals("cur_b2", s.olderCursor, "c2 kept its own cursor")

        client.loadOlder()
        s = awaitUntil(timeoutMs = 3_000, get = { client.state.value }) { it.messages.size == 2 }
        assertEquals(listOf("b1", "b2"), s.messages.map { it.id }, "c2 can still page back")
    }
}
