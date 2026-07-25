package io.sild.core

import io.github.centrifugal.centrifuge.Client
import io.github.centrifugal.centrifuge.ConnectedEvent
import io.github.centrifugal.centrifuge.ConnectingEvent
import io.github.centrifugal.centrifuge.ConnectionTokenEvent
import io.github.centrifugal.centrifuge.ConnectionTokenGetter
import io.github.centrifugal.centrifuge.DisconnectedEvent
import io.github.centrifugal.centrifuge.EventListener
import io.github.centrifugal.centrifuge.Options
import io.github.centrifugal.centrifuge.ServerPublicationEvent
import io.github.centrifugal.centrifuge.TokenCallback
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.launch
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonElement
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.jsonPrimitive

/** A realtime envelope (§5.3): { type, conversation_id, data }. */
data class RealtimeEnvelope(val type: String, val conversationId: String?, val data: JsonElement?)

// SildRealtime wraps the official Centrifugo client over WebSocket (/v1/ws) —
// the same broker + protocol the inbox and web widget use, so server-derived
// channel subscriptions and the reconnect-after-create flow behave identically.
// Server-side subscriptions mean publications arrive on the top-level listener's
// onPublication; the client never subscribes to channels itself.
internal class SildRealtime(
    private val cfg: SildConfig,
    private val scope: CoroutineScope,
    private val onConnection: (ConnectionState) -> Unit,
    private val onEnvelope: (RealtimeEnvelope) -> Unit,
) {
    private val json = Json { ignoreUnknownKeys = true }
    private var client: Client? = null

    /** ws(s):// endpoint derived from the http(s) base. */
    private fun wsEndpoint(): String {
        val b = cfg.base
        val ws = when {
            b.startsWith("https://") -> "wss://" + b.removePrefix("https://")
            b.startsWith("http://") -> "ws://" + b.removePrefix("http://")
            else -> b
        }
        return "$ws/v1/ws"
    }

    fun connect() {
        if (client != null) return
        val opts = Options().apply {
            // Centrifuge asks for a token on connect + on refresh; delegate to the
            // host token provider (forced-fresh each time, like the web getToken(true)).
            setTokenGetter(object : ConnectionTokenGetter() {
                override fun getConnectionToken(event: ConnectionTokenEvent, cb: TokenCallback) {
                    scope.launch {
                        runCatching { cfg.tokenProvider.token() }
                            .onSuccess { cb.Done(null, it) }
                            .onFailure { cb.Done(it, null) }
                    }
                }
            })
        }
        val listener = object : EventListener() {
            override fun onConnecting(c: Client, e: ConnectingEvent) = onConnection(ConnectionState.CONNECTING)
            override fun onConnected(c: Client, e: ConnectedEvent) = onConnection(ConnectionState.CONNECTED)
            override fun onDisconnected(c: Client, e: DisconnectedEvent) = onConnection(ConnectionState.DISCONNECTED)
            override fun onPublication(c: Client, e: ServerPublicationEvent) {
                val env = runCatching { parseEnvelope(e.data) }.getOrNull() ?: return
                onEnvelope(env)
            }
        }
        client = Client(wsEndpoint(), opts, listener).also { it.connect() }
    }

    private fun parseEnvelope(bytes: ByteArray): RealtimeEnvelope {
        val obj: JsonObject = json.parseToJsonElement(bytes.decodeToString()).jsonObject
        return RealtimeEnvelope(
            type = obj["type"]?.jsonPrimitive?.content ?: "",
            conversationId = obj["conversation_id"]?.jsonPrimitive?.content,
            data = obj["data"],
        )
    }

    // reconnect forces the server to re-derive this connection's channel set —
    // needed right after creating a conversation (the socket predates it, so its
    // subscription set doesn't yet include conv:<id>). Mirrors the web reconnect().
    fun reconnect() {
        val c = client ?: return
        runCatching {
            c.disconnect()
            c.connect()
        }
    }

    fun destroy() {
        runCatching { client?.disconnect() }
        client = null
    }
}
