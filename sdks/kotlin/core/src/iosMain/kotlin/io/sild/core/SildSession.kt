package io.sild.core

import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.Job
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.cancel
import kotlinx.coroutines.launch

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

    /** Observe [SildClient.state]. The current value arrives immediately and every
     *  change after it, always on the main dispatcher so a UI can bind directly.
     *  A Flow reaches Swift only as an opaque protocol, so this is the seam. */
    fun watchState(onChange: (SildState) -> Unit): SildCancellable =
        SildCancellable(scope.launch { client.state.collect { onChange(it) } })

    /** Tear down the client and cancel everything it has in flight. */
    fun close() {
        client.destroy()
        scope.cancel()
    }
}

/** Stops an observation started by [SildSession.watchState]. */
class SildCancellable internal constructor(private val job: Job) {
    fun cancel() {
        job.cancel()
    }
}
