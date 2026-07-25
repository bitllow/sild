package io.sild.core

import kotlinx.coroutines.delay
import okhttp3.OkHttpClient
import okhttp3.Request
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.jsonPrimitive

// Integration tests run against a live sild-dev (the same zero-infra backend the
// e2e suite boots). Point them at it with SILD_BASE_URL=http://localhost:8080;
// unset → the tests are skipped (so a plain `./gradlew test` never fails offline).
internal object Dev {
    val baseUrl: String? = System.getenv("SILD_BASE_URL")?.takeIf { it.isNotBlank() }
    private val http = OkHttpClient()
    private val json = Json { ignoreUnknownKeys = true }

    /** A TokenProvider hitting the dev token-mint endpoint (stands in for the host). */
    fun tokenProvider(userId: String) = TokenProvider {
        val req = Request.Builder().url("$baseUrl/v1/dev/widget-token?user_id=$userId").build()
        http.newCall(req).execute().use { res ->
            val body = res.body!!.string()
            json.parseToJsonElement(body).jsonObject.getValue("token").jsonPrimitive.content
        }
    }

    /** Ensure a driver↔rider peer conversation exists and return its id. */
    fun ensurePeerConversation(riderId: String, reference: String): String {
        val req = Request.Builder()
            .url("$baseUrl/v1/dev/peer-conversation?user_id=$riderId&reference=$reference")
            .build()
        http.newCall(req).execute().use { res ->
            val body = res.body!!.string()
            return json.parseToJsonElement(body).jsonObject.getValue("conversation_id").jsonPrimitive.content
        }
    }

    /** The deterministic driver id ensurePeerConversation assigns (sild-dev). */
    fun driverId(reference: String) = "u_driver_$reference"
}

/** Poll [predicate] on a value produced by [get] until true or [timeoutMs] elapses. */
internal suspend fun <T> awaitUntil(timeoutMs: Long = 10_000, get: () -> T, predicate: (T) -> Boolean): T {
    val deadline = System.nanoTime() + timeoutMs * 1_000_000
    while (System.nanoTime() < deadline) {
        val v = get()
        if (predicate(v)) return v
        delay(100)
    }
    return get()
}
