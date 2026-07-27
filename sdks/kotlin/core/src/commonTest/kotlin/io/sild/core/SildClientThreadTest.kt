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
import kotlin.test.assertNull
import kotlin.test.assertTrue

// Opening a conversation the recent page does not contain. The list is one bounded page
// (RECENT_CONVERSATIONS), so a directly opened thread — a trip's driver chat, a deep
// link — has no row unless the client fetches it.
class SildClientThreadTest {
    private val scope = CoroutineScope(Dispatchers.Default + SupervisorJob())
    private val paths = mutableListOf<String>()

    @AfterTest fun tearDown() {
        scope.cancel()
    }

    /** A client whose engine serves an empty recent list plus the scripted routes. */
    private fun client(vararg routes: Pair<String, String>): SildClient {
        val table = routes.toMap()
        val engine = MockEngine { request ->
            val path = request.url.encodedPath
            paths += path
            val body = table[path] ?: when {
                path == "/v1/conversations" -> """{"items":[],"next_cursor":null,"has_more":false}"""
                path == "/v1/brands/active" -> """{"name":"Acme","config":{}}"""
                path.endsWith("/messages") -> """{"items":[],"next_cursor":null,"has_more":false}"""
                else -> return@MockEngine respond("", HttpStatusCode.NotFound)
            }
            respond(body, HttpStatusCode.OK)
        }
        val cfg = SildConfig(
            baseUrl = "http://api.test",
            tokenProvider = TokenProvider { "tok" },
            userId = "u_rider",
        )
        return SildClient(cfg, scope, {}, null, SildApi(cfg, HttpClient(engine) { sildDefaults() }))
    }

    private val peerRow = """
        {"id":"c_deep","status":"open","reference":"trip_9021",
         "members":[
           {"member_kind":"user","external_user_id":"u_rider","metadata":{"name":"Riia"}},
           {"member_kind":"user","external_user_id":"u_driver","metadata":{"name":"Toomas Vaher"}}
         ]}
    """.trimIndent()

    @Test fun fetchesTheRowForAConversationOutsideTheRecentPage() = runBlockingTest {
        val client = client("/v1/conversations/c_deep" to peerRow)
        client.openConversation("c_deep")
        val s = awaitUntil(timeoutMs = 3_000, get = { client.state.value }) {
            it.conversations.any { c -> c.id == "c_deep" }
        }
        val conv = s.conversations.first { it.id == "c_deep" }
        assertTrue(conv.peer, "no assignment and no agent → peer")
        assertEquals("Toomas Vaher", conv.title, "the header names the other party, not \"Support\"")
        assertTrue(conv.subtitle!!.startsWith("Direct chat · trip_9021"))
        assertEquals("Toomas Vaher", conv.names["u_driver"], "member names resolve for author mapping")
        assertTrue(paths.contains("/v1/conversations/c_deep"), "the single row was fetched")
    }

    @Test fun closedStateReachesTheThreadSoTheComposerLocks() = runBlockingTest {
        val closed = """{"id":"c_closed","status":"closed","members":[]}"""
        val client = client("/v1/conversations/c_closed" to closed)
        client.openConversation("c_closed")
        val s = awaitUntil(timeoutMs = 3_000, get = { client.state.value }) {
            it.conversations.any { c -> c.id == "c_closed" }
        }
        assertTrue(s.conversations.first { it.id == "c_closed" }.closed)
    }

    // The row is already there after loadConversations, so the extra GET must not fire.
    @Test fun doesNotRefetchAConversationAlreadyInTheList() = runBlockingTest {
        val page = """{"items":[{"id":"c_listed","status":"open","members":[]}],"next_cursor":null,"has_more":false}"""
        val client = client("/v1/conversations" to page)
        client.loadConversations()
        client.openConversation("c_listed")
        awaitUntil(timeoutMs = 3_000, get = { client.state.value }) { it.activeId == "c_listed" }
        assertTrue(
            paths.none { it == "/v1/conversations/c_listed" },
            "a listed conversation needs no second request: $paths",
        )
    }

    // A missing or forbidden row must not block the thread: messages still render.
    @Test fun aFailedRowFetchStillLoadsTheThread() = runBlockingTest {
        val client = client() // /v1/conversations/c_gone → 404
        client.openConversation("c_gone")
        // activeId first: loadingThread is already false before the load starts, so
        // waiting on it alone would return the untouched initial state.
        val s = awaitUntil(timeoutMs = 3_000, get = { client.state.value }) {
            it.activeId == "c_gone" && !it.loadingThread
        }
        assertEquals("c_gone", s.activeId)
        assertNull(s.conversations.firstOrNull { it.id == "c_gone" })
    }
}
