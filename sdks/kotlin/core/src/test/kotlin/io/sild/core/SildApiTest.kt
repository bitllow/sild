package io.sild.core

import kotlinx.coroutines.runBlocking
import okhttp3.mockwebserver.MockResponse
import okhttp3.mockwebserver.MockWebServer
import kotlin.test.AfterTest
import kotlin.test.BeforeTest
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFailsWith
import kotlin.test.assertTrue

// Offline REST tests: 401 refresh-and-retry, error extraction, upload grant → PUT.
// A live backend won't hand out an expired token to order, hence MockWebServer.
class SildApiTest {
    private lateinit var server: MockWebServer
    private lateinit var minted: MutableList<String>

    @BeforeTest fun setUp() {
        server = MockWebServer().apply { start() }
        minted = mutableListOf()
    }

    @AfterTest fun tearDown() {
        server.shutdown()
    }

    private fun base() = server.url("/").toString().trimEnd('/')

    // Records each mint so a test can count how often the client asked.
    private fun api(): SildApi {
        val cfg = SildConfig(
            baseUrl = base(),
            tokenProvider = TokenProvider { "tok${minted.size + 1}".also { minted += it } },
        )
        return SildApi(cfg)
    }

    @Test fun cachesTheTokenAcrossCalls() = runBlocking {
        server.enqueue(MockResponse().setBody(EMPTY_PAGE))
        server.enqueue(MockResponse().setBody(EMPTY_PAGE))
        val api = api()
        api.listConversations()
        api.listConversations()
        assertEquals(1, minted.size, "the token is minted once and reused")
        assertEquals("Bearer tok1", server.takeRequest().getHeader("Authorization"))
        assertEquals("Bearer tok1", server.takeRequest().getHeader("Authorization"))
    }

    @Test fun refreshesOnceAndRetriesOn401() = runBlocking {
        server.enqueue(MockResponse().setResponseCode(401).setBody("""{"error":{"message":"token expired"}}"""))
        server.enqueue(MockResponse().setBody("""{"items":[{"id":"c1"}],"next_cursor":null,"has_more":false}"""))
        val convs = api().listConversations()
        assertEquals(listOf("c1"), convs.map { it.id }, "the retry's result is returned")
        assertEquals(2, minted.size, "the 401 forces exactly one refresh")
        assertEquals("Bearer tok1", server.takeRequest().getHeader("Authorization"))
        val retry = server.takeRequest()
        assertEquals("Bearer tok2", retry.getHeader("Authorization"), "the retry carries the fresh token")
        assertEquals("android/$SDK_VERSION", retry.getHeader("X-Sild-SDK"))
    }

    @Test fun aSecond401IsNotRetriedAgain() = runBlocking {
        server.enqueue(MockResponse().setResponseCode(401).setBody("""{"error":{"message":"nope"}}"""))
        server.enqueue(MockResponse().setResponseCode(401).setBody("""{"error":{"message":"still nope"}}"""))
        val e = assertFailsWith<SildApiException> { api().listConversations() }
        assertEquals(401, e.status)
        assertEquals("still nope", e.message, "the server's error.message is surfaced")
        assertEquals(2, server.requestCount, "one refresh only — no retry loop")
    }

    @Test fun retryReusesTheSameClientMsgId() = runBlocking {
        server.enqueue(MockResponse().setResponseCode(401))
        server.enqueue(MockResponse().setBody("""{"id":"m1"}"""))
        api().sendMessage("c1", "hi", "client-msg-42", emptyList())
        val first = server.takeRequest().body.readUtf8()
        val retried = server.takeRequest().body.readUtf8()
        assertEquals(first, retried, "the refreshed retry replays the identical body")
        assertTrue(retried.contains("client-msg-42"), "so the send stays idempotent: $retried")
    }

    @Test fun nonJsonErrorBodyFallsBackToTheStatusMessage() = runBlocking {
        server.enqueue(MockResponse().setResponseCode(500).setBody("<html>oops</html>"))
        val e = assertFailsWith<SildApiException> { api().listConversations() }
        assertEquals(500, e.status)
        assertTrue(!e.message.isNullOrEmpty(), "an unparseable body still yields a message")
    }

    @Test fun uploadGrantsThenPutsTheBytes() = runBlocking {
        // sild-dev returns an absolute public-origin URL; the client re-bases it.
        server.enqueue(
            MockResponse().setBody(
                """{"object_key":"o/1","upload_url":"http://public.example:9999/v1/uploads/local/o/1"}""",
            ),
        )
        server.enqueue(MockResponse().setResponseCode(200))
        val att = api().upload("hello".toByteArray(), "note.png", "image/png")

        assertEquals("o/1", att.objectKey)
        assertEquals("inline", att.disposition, "an image is attached inline")
        assertEquals("note.png", att.filename)

        val grant = server.takeRequest()
        assertEquals("/v1/uploads", grant.path)
        assertTrue(grant.body.readUtf8().contains("\"size_bytes\":5"))

        val put = server.takeRequest()
        assertEquals("PUT", put.method)
        assertEquals("/v1/uploads/local/o/1", put.path, "the signed URL was re-based onto our base")
        assertEquals("hello", put.body.readUtf8())
        // The bucket verifies a signature over the request; our API headers must not ride along.
        assertEquals(null, put.getHeader("Authorization"))
        assertEquals(null, put.getHeader("X-Sild-SDK"))
    }

    @Test fun uploadOfANonImageIsAnAttachment() = runBlocking {
        server.enqueue(MockResponse().setBody("""{"object_key":"o/2","upload_url":"${base()}/v1/uploads/local/o/2"}"""))
        server.enqueue(MockResponse().setResponseCode(200))
        val att = api().upload(byteArrayOf(1, 2), "receipt.pdf", "")
        assertEquals("attachment", att.disposition)
        assertEquals("application/octet-stream", att.mimeType, "an empty mime falls back to octet-stream")
    }

    @Test fun failedPutSurfacesTheStatus() = runBlocking {
        server.enqueue(MockResponse().setBody("""{"object_key":"o/3","upload_url":"${base()}/v1/uploads/local/o/3"}"""))
        server.enqueue(MockResponse().setResponseCode(403))
        val e = assertFailsWith<SildApiException> { api().upload(byteArrayOf(1), "x.bin", "application/octet-stream") }
        assertEquals(403, e.status)
    }

    @Test fun brandLogoUrlIsRebasedOntoOurBase() = runBlocking {
        server.enqueue(
            MockResponse().setBody(
                """{"name":"Acme","config":{"logoUrl":"http://public.example:9999/v1/uploads/local/logo.png"}}""",
            ),
        )
        val res = api().fetchBrand()
        assertEquals("Acme", res.name)
        assertEquals("${base()}/v1/uploads/local/logo.png", res.config.logoUrl)
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
