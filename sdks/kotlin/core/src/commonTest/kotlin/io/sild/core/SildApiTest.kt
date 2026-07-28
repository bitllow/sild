package io.sild.core

import io.ktor.client.HttpClient
import io.ktor.client.engine.mock.MockEngine
import io.ktor.client.engine.mock.MockRequestHandleScope
import io.ktor.client.engine.mock.respond
import io.ktor.client.engine.mock.respondError
import io.ktor.client.request.HttpRequestData
import io.ktor.client.request.HttpResponseData
import io.ktor.http.HttpStatusCode
import io.ktor.http.content.OutgoingContent
import io.ktor.utils.io.core.toByteArray
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFailsWith
import kotlin.test.assertNull
import kotlin.test.assertTrue

// Offline REST tests: 401 refresh-and-retry, error extraction, upload grant → PUT.
// A live backend won't hand out an expired token to order, hence a scripted engine.
class SildApiTest {
    private val minted = mutableListOf<String>()
    private val requests = mutableListOf<HttpRequestData>()

    private val base = "http://api.test"

    /** Build an api() whose engine replays [responses] in order, recording each request. */
    private fun api(vararg responses: MockRequestHandleScope.() -> HttpResponseData): SildApi {
        val engine = MockEngine { request ->
            requests += request
            val i = requests.size - 1
            if (i >= responses.size) error("unexpected request #${i + 1}: ${request.url}")
            responses[i](this)
        }
        val cfg = SildConfig(
            baseUrl = base,
            // Records each mint so a test can count how often the client asked.
            tokenProvider = TokenProvider { "tok${minted.size + 1}".also { minted += it } },
        )
        return SildApi(cfg, HttpClient(engine) { sildDefaults() })
    }

    private fun MockRequestHandleScope.ok(body: String) = respond(body, HttpStatusCode.OK)

    private fun path(i: Int) = requests[i].url.encodedPath +
        requests[i].url.encodedQuery.let { if (it.isEmpty()) "" else "?$it" }

    private suspend fun body(i: Int): String {
        val content = requests[i].body as OutgoingContent.ByteArrayContent
        return content.bytes().decodeToString()
    }

    private fun header(i: Int, name: String) = requests[i].headers[name]

    @Test fun cachesTheTokenAcrossCalls() = runBlockingTest {
        val api = api({ ok(EMPTY_PAGE) }, { ok(EMPTY_PAGE) })
        api.listConversations()
        api.listConversations()
        assertEquals(1, minted.size, "the token is minted once and reused")
        assertEquals("Bearer tok1", header(0, "Authorization"))
        assertEquals("Bearer tok1", header(1, "Authorization"))
    }

    @Test fun refreshesOnceAndRetriesOn401() = runBlockingTest {
        val convs = api(
            { respond("""{"error":{"message":"token expired"}}""", HttpStatusCode.Unauthorized) },
            { ok("""{"items":[{"id":"c1"}],"next_cursor":null,"has_more":false}""") },
        ).listConversations()
        assertEquals(listOf("c1"), convs.map { it.id }, "the retry's result is returned")
        assertEquals(2, minted.size, "the 401 forces exactly one refresh")
        assertEquals("Bearer tok1", header(0, "Authorization"))
        assertEquals("Bearer tok2", header(1, "Authorization"), "the retry carries the fresh token")
        assertEquals("$SDK_PLATFORM/$SDK_VERSION", header(1, "X-Sild-SDK"))
        assertTrue(SDK_PLATFORM.isNotEmpty(), "every platform names itself in the SDK header")
    }

    @Test fun aSecond401IsNotRetriedAgain() = runBlockingTest {
        val e = assertFailsWith<SildApiException> {
            api(
                { respond("""{"error":{"message":"nope"}}""", HttpStatusCode.Unauthorized) },
                { respond("""{"error":{"message":"still nope"}}""", HttpStatusCode.Unauthorized) },
            ).listConversations()
        }
        assertEquals(401, e.status)
        assertEquals("still nope", e.message, "the server's error.message is surfaced")
        assertEquals(2, requests.size, "one refresh only — no retry loop")
    }

    @Test fun retryReusesTheSameClientMsgId() = runBlockingTest {
        api(
            { respond("", HttpStatusCode.Unauthorized) },
            { ok("""{"id":"m1"}""") },
        ).sendMessage("c1", "hi", "client-msg-42", emptyList())
        val first = body(0)
        val retried = body(1)
        assertEquals(first, retried, "the refreshed retry replays the identical body")
        assertTrue(retried.contains("client-msg-42"), "so the send stays idempotent: $retried")
    }

    @Test fun nonJsonErrorBodyFallsBackToTheStatusMessage() = runBlockingTest {
        val e = assertFailsWith<SildApiException> {
            api({ respond("<html>oops</html>", HttpStatusCode.InternalServerError) }).listConversations()
        }
        assertEquals(500, e.status)
        assertTrue(!e.message.isNullOrEmpty(), "an unparseable body still yields a message")
    }

    @Test fun uploadGrantsThenPutsTheBytes() = runBlockingTest {
        val att = api(
            // sild-dev returns an absolute public-origin URL; the client re-bases it.
            { ok("""{"object_key":"o/1","upload_url":"http://public.example:9999/v1/uploads/local/o/1"}""") },
            { respond("", HttpStatusCode.OK) },
        ).upload("hello".toByteArray(), "note.png", "image/png")

        assertEquals("o/1", att.objectKey)
        assertEquals("inline", att.disposition, "an image is attached inline")
        assertEquals("note.png", att.filename)

        assertEquals("/v1/uploads", path(0))
        assertTrue(body(0).contains("\"size_bytes\":5"))

        assertEquals("PUT", requests[1].method.value)
        assertEquals("$base/v1/uploads/local/o/1", requests[1].url.toString(), "the signed URL was re-based onto our base")
        assertEquals("hello", body(1))
        // The bucket verifies a signature over the request; our API headers must not ride along.
        assertNull(header(1, "Authorization"))
        assertNull(header(1, "X-Sild-SDK"))
        // And nothing else the API path adds either: the PUT carries only content framing.
        assertEquals(
            emptyList(),
            requests[1].headers.names().filterNot { it.equals("Content-Type", true) || it.equals("Content-Length", true) },
            "the signed PUT must not gain headers",
        )
    }

    @Test fun uploadOfANonImageIsAnAttachment() = runBlockingTest {
        val att = api(
            { ok("""{"object_key":"o/2","upload_url":"$base/v1/uploads/local/o/2"}""") },
            { respond("", HttpStatusCode.OK) },
        ).upload(byteArrayOf(1, 2), "receipt.pdf", "")
        assertEquals("attachment", att.disposition)
        assertEquals("application/octet-stream", att.mimeType, "an empty mime falls back to octet-stream")
    }

    @Test fun failedPutSurfacesTheStatus() = runBlockingTest {
        val e = assertFailsWith<SildApiException> {
            api(
                { ok("""{"object_key":"o/3","upload_url":"$base/v1/uploads/local/o/3"}""") },
                { respondError(HttpStatusCode.Forbidden) },
            ).upload(byteArrayOf(1), "x.bin", "application/octet-stream")
        }
        assertEquals(403, e.status)
    }

    @Test fun brandLogoUrlIsRebasedOntoOurBase() = runBlockingTest {
        val res = api(
            { ok("""{"name":"Acme","config":{"logoUrl":"http://public.example:9999/v1/uploads/local/logo.png"}}""") },
        ).fetchBrand()
        assertEquals("Acme", res.name)
        assertEquals("$base/v1/uploads/local/logo.png", res.config.logoUrl)
    }

    @Test fun fetchesOneConversationRow() = runBlockingTest {
        val conv = api(
            { ok("""{"id":"c1","status":"closed","reference":"trip_1","members":[]}""") },
        ).getConversation("c1")
        assertEquals("/v1/conversations/c1", path(0))
        assertEquals("closed", conv.status)
        assertEquals("trip_1", conv.reference)
    }

    // ?since= is a sync read, not a page: the caller resumes from the last id it got
    // and repeats while has_more, so a gap longer than one limit is not truncated.
    @Test fun catchUpDrainsEveryMissedMessageAcrossPages() = runBlockingTest {
        val api = api(
            { ok("""{"items":[{"id":"m2"},{"id":"m3"}],"next_cursor":null,"has_more":true}""") },
            { ok("""{"items":[{"id":"m4"}],"next_cursor":null,"has_more":false}""") },
        )

        val first = api.catchUpMessages("c1", "m1")
        assertEquals(listOf("m2", "m3"), first.items.map { it.id })
        assertTrue(first.hasMore, "has_more survives decoding — without it the drain stops early")

        val second = api.catchUpMessages("c1", first.items.last().id)
        assertEquals(listOf("m4"), second.items.map { it.id })
        assertTrue(!second.hasMore)

        assertEquals("/v1/conversations/c1/messages?since=m1&limit=100", path(0))
        assertEquals(
            "/v1/conversations/c1/messages?since=m3&limit=100", path(1),
            "the second call resumes from the last id received, not from the original",
        )
    }

    // Paging BACKWARD: the thread endpoint always returned next_cursor, and no client
    // consumed it — so any conversation past the first page was permanently truncated
    // to its newest 100 messages on every surface.
    @Test fun listMessagesPagesBackwardThroughTheCursor() = runBlockingTest {
        val api = api(
            { ok("""{"items":[{"id":"m3"},{"id":"m4"}],"next_cursor":"cur_m3","has_more":true}""") },
            { ok("""{"items":[{"id":"m1"},{"id":"m2"}],"next_cursor":null,"has_more":false}""") },
        )

        val newest = api.listMessages("c1")
        assertEquals(listOf("m3", "m4"), newest.items.map { it.id })
        assertEquals("cur_m3", newest.nextCursor)
        assertTrue(newest.hasMore, "has_more survives decoding — without it there is nothing to load")

        val older = api.listMessages("c1", newest.nextCursor)
        assertEquals(listOf("m1", "m2"), older.items.map { it.id })
        assertTrue(!older.hasMore)

        assertEquals("/v1/conversations/c1/messages?limit=100", path(0))
        assertEquals(
            "/v1/conversations/c1/messages?limit=100&cursor=cur_m3", path(1),
            "the second call carries the cursor the first page returned",
        )
    }
}

// Local-dev storage is rewritten onto our base (an emulator reaches the host at
// 10.0.2.2); a signed cloud URL must pass through untouched.
class RebaseLocalUrlTest {
    @Test fun rewritesLocalDevStorageOntoTheBase() {
        assertEquals(
            "http://10.0.2.2:8080/v1/uploads/local/abc/def.png",
            rebaseLocalUrl("http://10.0.2.2:8080", "http://localhost:8080/v1/uploads/local/abc/def.png"),
        )
    }

    @Test fun passesCloudUrlsThrough() {
        val signed = "https://bucket.s3.amazonaws.com/o/1?X-Amz-Signature=deadbeef"
        assertEquals(signed, rebaseLocalUrl("http://10.0.2.2:8080", signed))
    }

    @Test fun nullStaysNull() {
        assertEquals(null, rebaseLocalUrl("http://10.0.2.2:8080", null))
    }
}

/** The list envelope every collection endpoint returns. */
private const val EMPTY_PAGE = """{"items":[],"next_cursor":null,"has_more":false}"""
