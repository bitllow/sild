package io.sild.core

import io.ktor.client.request.get
import io.ktor.client.statement.bodyAsText
import kotlin.time.TimeSource
import kotlin.uuid.ExperimentalUuidApi
import kotlin.uuid.Uuid
import kotlin.concurrent.atomics.AtomicReference
import kotlin.concurrent.atomics.ExperimentalAtomicApi
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.delay
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.jsonPrimitive

/** Read a process environment variable — JVM has System.getenv, Kotlin/Native does not. */
internal expect fun envVar(name: String): String?

/** runBlocking, which lives in the concurrent (JVM + native) source set, not common.
 *  The tests await real callbacks on real threads, so a virtual-time runTest would race. */
internal expect fun runBlockingTest(block: suspend CoroutineScope.() -> Unit)

/**
 * What a mock engine saw, collected safely. The client issues requests concurrently
 * — a row fetch and its message page go out together — so two handler coroutines
 * append at once and a plain list loses one, failing a test whose request was in
 * fact made. Reads as an ordinary List everywhere else.
 */
@OptIn(ExperimentalAtomicApi::class)
internal class Recorded<T> : AbstractList<T>() {
    // Compare-and-set on a whole immutable list: an append that raced another
    // retries, where `items += x` would silently drop one of them.
    private val items = AtomicReference<List<T>>(emptyList())

    operator fun plusAssign(item: T) {
        while (true) {
            val seen = items.load()
            if (items.compareAndSet(seen, seen + item)) return
        }
    }

    /** Forget what was recorded — a test asserting on the next call only. */
    fun clear() = items.store(emptyList())

    override val size: Int get() = items.load().size
    override fun get(index: Int): T = items.load()[index]
}

/** A unique id per call, so concurrent runs against one sild-dev don't collide. */
@OptIn(ExperimentalUuidApi::class)
internal fun uid(prefix: String) = "${prefix}_${Uuid.random().toHexString().take(12)}"

// Integration tests run against a live sild-dev (the same zero-infra backend the
// e2e suite boots). Point them at it with SILD_BASE_URL=http://localhost:8080;
// unset → the tests return early (so a plain offline test run never fails).
internal object Dev {
    val baseUrl: String? = envVar("SILD_BASE_URL")?.takeIf { it.isNotBlank() }
    private val http = SildHttp.client
    private val json = Json { ignoreUnknownKeys = true }

    /** A TokenProvider hitting the dev token-mint endpoint (stands in for the host). */
    fun tokenProvider(userId: String) = TokenProvider {
        val body = http.get("$baseUrl/v1/dev/widget-token?user_id=$userId").bodyAsText()
        json.parseToJsonElement(body).jsonObject.getValue("token").jsonPrimitive.content
    }

    /** Ensure a driver↔rider peer conversation exists and return its id. */
    suspend fun ensurePeerConversation(riderId: String, reference: String): String {
        val body = http.get("$baseUrl/v1/dev/peer-conversation?user_id=$riderId&reference=$reference").bodyAsText()
        return json.parseToJsonElement(body).jsonObject.getValue("conversation_id").jsonPrimitive.content
    }

    /** The deterministic driver id ensurePeerConversation assigns (sild-dev). */
    fun driverId(reference: String) = "u_driver_$reference"
}

/** Poll [predicate] on a value produced by [get] until true or [timeoutMs] elapses. */
internal suspend fun <T> awaitUntil(timeoutMs: Long = 10_000, get: () -> T, predicate: (T) -> Boolean): T {
    val start = TimeSource.Monotonic.markNow()
    while (start.elapsedNow().inWholeMilliseconds < timeoutMs) {
        val v = get()
        if (predicate(v)) return v
        delay(100)
    }
    return get()
}
