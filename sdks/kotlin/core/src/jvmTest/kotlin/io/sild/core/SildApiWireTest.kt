package io.sild.core

import kotlinx.coroutines.runBlocking
import okhttp3.mockwebserver.MockResponse
import okhttp3.mockwebserver.MockWebServer
import kotlin.test.AfterTest
import kotlin.test.BeforeTest
import kotlin.test.Test
import kotlin.test.assertEquals

// The mock engine in SildApiTest asserts the request the client *builds*; this asserts
// what the real OkHttp engine actually sends, over a socket. That gap matters for the
// signed upload PUT, whose URL is signed over the request shape — an engine-level
// default header would break it and the mock engine would never show it.
class SildApiWireTest {
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

    private fun api() = SildApi(
        SildConfig(
            baseUrl = base(),
            tokenProvider = TokenProvider { "tok${minted.size + 1}".also { minted += it } },
        ),
    )

    @Test fun signedPutCarriesOnlyContentFraming() = runBlocking {
        server.enqueue(MockResponse().setBody("""{"object_key":"o/1","upload_url":"${base()}/v1/uploads/local/o/1"}"""))
        server.enqueue(MockResponse().setResponseCode(200))
        api().upload("hello".toByteArray(), "note.png", "image/png")

        server.takeRequest() // the grant
        val put = server.takeRequest()
        assertEquals("PUT", put.method)
        assertEquals("/v1/uploads/local/o/1", put.path)
        assertEquals("hello", put.body.readUtf8())
        // Transport headers are OkHttp's own (it has always added these, gzip included);
        // everything else describes the request to the bucket and must be absent.
        val transport = setOf("host", "connection", "user-agent", "accept-encoding")
        val carried = put.headers.names()
            .filterNot { it.lowercase() in transport }
            .sortedBy { it.lowercase() }
        assertEquals(listOf("Content-Length", "Content-Type"), carried)
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
        // Pinned: Android hosts consume this variant, and the tag is already in server logs.
        assertEquals("android/$SDK_VERSION", retry.getHeader("X-Sild-SDK"))
    }
}
