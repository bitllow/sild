package io.sild.core

import kotlinx.coroutines.CoroutineScope
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonElement
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.jsonPrimitive

/** A realtime envelope (§5.3): { type, conversation_id, data }. */
data class RealtimeEnvelope(val type: String, val conversationId: String?, val data: JsonElement?)

// RealtimeTransport is the socket the client drives (§5): the Centrifugo protocol
// the inbox and web widget use, with server-side subscriptions only — publications
// arrive on the connection, the client never subscribes to channels itself.
interface RealtimeTransport {
    fun connect()

    /** Force the server to re-derive this connection's channel set — needed right
     *  after creating a conversation, whose channel the live socket predates. */
    fun reconnect()

    fun destroy()
}

/** Builds a transport bound to one client's callbacks. */
fun interface RealtimeTransportFactory {
    fun create(
        cfg: SildConfig,
        scope: CoroutineScope,
        onConnection: (ConnectionState) -> Unit,
        onEnvelope: (RealtimeEnvelope) -> Unit,
    ): RealtimeTransport
}

/** The platform's transport, or null where the host must supply one (iOS). */
expect fun defaultRealtimeTransport(): RealtimeTransportFactory?

/** ws(s):// endpoint derived from an http(s) base. */
fun realtimeEndpoint(base: String): String {
    val ws = when {
        base.startsWith("https://") -> "wss://" + base.removePrefix("https://")
        base.startsWith("http://") -> "ws://" + base.removePrefix("http://")
        else -> base
    }
    return "$ws/v1/ws"
}

private val envelopeJson = Json { ignoreUnknownKeys = true }

/** Parse a publication payload, or null if it is not an envelope. */
fun parseRealtimeEnvelope(payload: String): RealtimeEnvelope? = runCatching {
    val obj = envelopeJson.parseToJsonElement(payload).jsonObject
    RealtimeEnvelope(
        type = obj["type"]?.jsonPrimitive?.content ?: "",
        conversationId = obj["conversation_id"]?.jsonPrimitive?.content,
        data = obj["data"],
    )
}.getOrNull()
