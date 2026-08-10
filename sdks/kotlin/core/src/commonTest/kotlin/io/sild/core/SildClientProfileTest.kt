package io.sild.core

import io.ktor.client.HttpClient
import io.ktor.client.engine.mock.MockEngine
import io.ktor.client.engine.mock.respond
import io.ktor.http.HttpStatusCode
import io.ktor.http.content.TextContent
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.cancel
import kotlin.test.AfterTest
import kotlin.test.Test
import kotlin.test.assertContains
import kotlin.test.assertTrue

// PUT /v1/contacts/me replaces the profile whole, so what start() sends decides
// whether the host's own backend-written profile survives the app launch. An
// unconfigured profile asserts nothing; a configured-empty one asserts empty.
class SildClientProfileTest {
    private val scope = CoroutineScope(Dispatchers.Default + SupervisorJob())
    private val writes = mutableListOf<String>()

    @AfterTest fun tearDown() {
        scope.cancel()
    }

    private fun client(metadata: Map<String, String>?): SildClient {
        val engine = MockEngine { request ->
            val path = request.url.encodedPath
            // Only a body carrying `metadata` is a profile write; start() also records
            // the reader's language on the same route, which asserts nothing about it.
            val sent = (request.body as? TextContent)?.text.orEmpty()
            if (path == "/v1/contacts/me" && sent.contains("\"metadata\"")) writes += request.method.value
            val body = when (path) {
                "/v1/conversations" -> """{"items":[],"next_cursor":null,"has_more":false}"""
                "/v1/brands/active" -> """{"name":"Acme","config":{}}"""
                else -> return@MockEngine respond("", HttpStatusCode.NoContent)
            }
            respond(body, HttpStatusCode.OK)
        }
        val cfg = SildConfig(
            baseUrl = "http://api.test",
            tokenProvider = TokenProvider { "tok" },
            userId = "u_rider",
            metadata = metadata,
        )
        return SildClient(cfg, scope, {}, null, SildApi(cfg, HttpClient(engine) { sildDefaults() }))
    }

    private suspend fun startAndSettle(client: SildClient) {
        client.start()
        awaitUntil(timeoutMs = 3_000, get = { client.state.value }) { it.ready }
    }

    @Test fun noConfiguredMetadataWritesNoProfile() = runBlockingTest {
        startAndSettle(client(null))
        assertTrue(writes.isEmpty(), "start() must not blank a profile it was given nothing to assert: $writes")
    }

    // Configured-empty is an assertion, not an absence: it must still erase.
    @Test fun configuredEmptyMetadataStillWritesTheProfile() = runBlockingTest {
        startAndSettle(client(emptyMap()))
        assertContains(writes, "PUT")
    }

    @Test fun configuredMetadataStillWritesTheProfile() = runBlockingTest {
        startAndSettle(client(mapOf("name" to "Riia")))
        assertContains(writes, "PUT")
    }
}
