package io.sild.core

import kotlin.concurrent.Volatile
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.SupervisorJob

/**
 * The process-wide host wiring: the config a host supplies once, and the push
 * calls that happen when no messenger is on screen (§5.5).
 *
 * Both platform entry points — `Sild` on Android, `Sild` in Swift — are thin
 * delegations to this, so the rules live here and are tested once.
 */
object SildHost {
    /** What the host configured, and the only holder of it. */
    @Volatile
    var config: SildConfig? = null
        set(value) {
            field = value
            rest = null // a new config is a new tenant or user: mint against it
        }

    /**
     * The messenger's client while it is on screen — the only thing that knows
     * which conversation is open. The platform layer sets and clears it.
     */
    @Volatile
    var onScreen: SildClient? = null

    /** Forwards the push token the host was issued. */
    fun setPushToken(token: String, onResult: (Boolean) -> Unit = {}) {
        val client = rest() ?: return onResult(false)
        client.setPushToken(token, onResult = onResult)
    }

    /** Releases this device, so the next user of it hears nothing. */
    fun clearPushToken(token: String, onResult: (Boolean) -> Unit = {}) {
        val client = rest() ?: return onResult(false)
        client.clearPushToken(token, onResult = onResult)
    }

    /**
     * Whether to display a nudge that arrived in the foreground. Nothing on
     * screen is the same rule with nothing open.
     */
    fun shouldShow(data: Map<String, String>): Boolean =
        onScreen?.shouldShow(data) ?: SildPush.shouldShow(data, null)

    // REST-only (no transport): registering a device must not open a socket. Kept
    // between calls so the bearer it caches survives a sign-out retry.
    private fun rest(): SildClient? {
        rest?.let { return it }
        val cfg = config ?: return null
        return SildClient(cfg, scope, transport = null).also { rest = it }
    }

    @Volatile
    private var rest: SildClient? = null
    private val scope = CoroutineScope(SupervisorJob() + Dispatchers.Default)
}
