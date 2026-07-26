package io.sild.core

import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.cancel

// SildSession is the Swift-facing entry point. Swift cannot build a CoroutineScope —
// the coroutines types reach the framework as opaque protocols — so the session owns
// one and bounds every async call the client makes.
class SildSession(
    config: SildConfig,
    onChime: () -> Unit = {},
    transport: RealtimeTransportFactory? = null,
) {
    private val scope = CoroutineScope(SupervisorJob() + Dispatchers.Main)

    val client: SildClient = SildClient(config, scope, onChime, transport)

    /** Tear down the client and cancel everything it has in flight. */
    fun close() {
        client.destroy()
        scope.cancel()
    }
}
