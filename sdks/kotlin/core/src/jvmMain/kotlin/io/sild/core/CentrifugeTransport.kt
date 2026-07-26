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

actual fun defaultRealtimeTransport(): RealtimeTransportFactory? =
    RealtimeTransportFactory { cfg, scope, onConnection, onEnvelope ->
        CentrifugeTransport(cfg, scope, onConnection, onEnvelope)
    }

// CentrifugeTransport wraps the official Centrifugo client over WebSocket (/v1/ws)
// — the same broker + protocol the inbox and web widget use, so server-derived
// channel subscriptions and the reconnect-after-create flow behave identically.
// Server-side subscriptions mean publications arrive on the top-level listener's
// onPublication; the client never subscribes to channels itself.
internal class CentrifugeTransport(
    private val cfg: SildConfig,
    private val scope: CoroutineScope,
    private val onConnection: (ConnectionState) -> Unit,
    private val onEnvelope: (RealtimeEnvelope) -> Unit,
) : RealtimeTransport {
    private var client: Client? = null

    override fun connect() {
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
                val env = parseRealtimeEnvelope(e.data.decodeToString()) ?: return
                onEnvelope(env)
            }
        }
        client = Client(realtimeEndpoint(cfg.base), opts, listener).also { it.connect() }
    }

    override fun reconnect() {
        val c = client ?: return
        runCatching {
            c.disconnect()
            c.connect()
        }
    }

    override fun destroy() {
        runCatching { client?.disconnect() }
        client = null
    }
}
