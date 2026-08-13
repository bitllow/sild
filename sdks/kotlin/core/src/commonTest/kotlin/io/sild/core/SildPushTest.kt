package io.sild.core

import io.ktor.client.HttpClient
import io.ktor.client.engine.mock.MockEngine
import io.ktor.client.engine.mock.respond
import io.ktor.http.content.OutgoingContent
import io.ktor.http.HttpStatusCode
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.cancel
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.first
import kotlin.test.AfterTest
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFalse
import kotlin.test.assertNull
import kotlin.test.assertTrue

class SildPushTest {
    private val scope = CoroutineScope(Dispatchers.Default + SupervisorJob())
    // Method plus body: a release that reaches the endpoint without its token is
    // indistinguishable from a working one unless the body is checked.
    private val calls = Recorded<Pair<String, String>>()

    @AfterTest fun tearDown() {
        scope.cancel()
    }

    private fun client(): SildClient {
        val engine = MockEngine { request ->
            val path = request.url.encodedPath
            if (path == "/v1/push-tokens") {
                val body = request.body as? OutgoingContent.ByteArrayContent
                calls += request.method.value to (body?.bytes()?.decodeToString() ?: "")
                return@MockEngine respond("", HttpStatusCode.NoContent)
            }
            val body = when (path) {
                "/v1/conversations" -> """{"items":[],"next_cursor":null,"has_more":false}"""
                "/v1/brands/active" -> """{"name":"Acme","config":{}}"""
                "/v1/conversations/c1" -> """{"id":"c1","status":"open","members":[]}"""
                "/v1/conversations/c1/messages" -> """{"items":[],"next_cursor":null,"has_more":false}"""
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

    private fun nudge(conversationId: String = "c1", kind: String = "peer") = mapOf(
        "sild" to "1",
        "conversation_id" to conversationId,
        "conversation_kind" to kind,
        "message_id" to "m1",
        "unread_count" to "3",
    )

    // The host's own push messages arrive at the same handler as ours, so telling
    // them apart is the first thing the integration has to do.
    @Test
    fun tellsOurMessagesFromTheHostsOwn() {
        assertTrue(SildPush.isSildPush(nudge()))
        assertFalse(SildPush.isSildPush(mapOf("campaign" to "spring-sale")))
        assertFalse(SildPush.isSildPush(emptyMap()))
    }

    @Test
    fun decodesTheNudge() {
        val push = SildPush.parse(nudge(conversationId = "c_42", kind = "support"))
        assertEquals("c_42", push?.conversationId)
        assertEquals("support", push?.conversationKind)
        assertEquals("m1", push?.messageId)
        assertEquals(3, push?.unreadCount)
    }

    @Test
    fun refusesToDecodeSomethingThatIsNotOurs() {
        assertNull(SildPush.parse(mapOf("campaign" to "spring-sale")))
        // Marked as ours but unusable — routing a tap on it would go nowhere.
        assertNull(SildPush.parse(mapOf("sild" to "1")))
    }

    // The host reaches its own handler only while foregrounded, so the remaining
    // question is whether the user is already looking at that conversation.
    @Test
    fun suppressesOnlyTheConversationOnScreen() = runBlockingTest {
        val c = client()
        assertTrue(c.shouldShow(nudge(conversationId = "c1")))

        c.openConversation("c1")
        c.state.first { it.activeId == "c1" }
        // Suppressed for the open thread, still shown for any other.
        assertFalse(c.shouldShow(nudge(conversationId = "c1")))
        assertTrue(c.shouldShow(nudge(conversationId = "c2")))
    }

    @Test
    fun neverShowsAPayloadThatIsNotOurs() {
        assertFalse(client().shouldShow(mapOf("campaign" to "spring-sale")))
    }

    @Test
    fun registersAndReleasesTheHostsToken() = runBlockingTest {
        val c = client()
        c.setPushToken("device-1")
        delay(100)
        assertEquals(1, calls.size)
        assertEquals("POST", calls[0].first)
        assertTrue(calls[0].second.contains("device-1"), "registration carried no token: ${calls[0].second}")

        calls.clear()
        c.clearPushToken("device-1")
        delay(100)
        assertEquals(1, calls.size)
        assertEquals("DELETE", calls[0].first)
        assertTrue(calls[0].second.contains("device-1"), "release carried no token: ${calls[0].second}")
    }

    // A token the host has not been issued yet must not become an empty
    // registration that shadows the real one.
    @Test
    fun ignoresAnEmptyToken() = runBlockingTest {
        val c = client()
        c.setPushToken("")
        c.clearPushToken("")
        delay(100)
        assertTrue(calls.isEmpty())
    }
}
