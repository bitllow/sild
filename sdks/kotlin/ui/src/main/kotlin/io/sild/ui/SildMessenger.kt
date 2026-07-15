package io.sild.ui

import android.content.Context
import android.content.Intent
import android.media.AudioManager
import android.media.ToneGenerator
import android.os.Bundle
import androidx.activity.ComponentActivity
import androidx.activity.compose.setContent
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.lifecycle.lifecycleScope
import io.sild.core.SildClient
import io.sild.core.SildConfig

// SildRuntime holds the host-supplied config process-wide. The config carries a
// TokenProvider (not parcelable), so the launcher stores it here and the messenger
// Activity reads it — the same "init once, open many" model as the web Sild.init.
object SildRuntime {
    @Volatile
    var config: SildConfig? = null
}

// Sild is the SDK entry point: Sild.init(config) once (e.g. in Application), then
// use the returned SildMessenger to open support or a specific conversation —
// mirroring the web widget's Sild.init handle and the App design's
// client.openSupportRequest() / client.openConversation(tripId).
object Sild {
    fun init(config: SildConfig): SildMessenger {
        SildRuntime.config = config
        return SildMessenger
    }
}

object SildMessenger {
    internal const val EXTRA_TARGET = "sild.target"
    internal const val TARGET_LIST = "list"
    internal const val TARGET_SUPPORT = "support"
    internal const val CONV_PREFIX = "conv:"

    /** Open the messenger on the conversation list ("Messages"). */
    fun openList(context: Context) = launch(context, TARGET_LIST)

    /** Start (or resume) a support request and open it. */
    fun openSupportRequest(context: Context) = launch(context, TARGET_SUPPORT)

    /** Open a specific conversation directly — e.g. the trip's driver chat. */
    fun openConversation(context: Context, conversationId: String) = launch(context, CONV_PREFIX + conversationId)

    private fun launch(context: Context, target: String) {
        val intent = Intent(context, SildMessengerActivity::class.java)
            .putExtra(EXTRA_TARGET, target)
        if (context !is ComponentActivity) intent.addFlags(Intent.FLAG_ACTIVITY_NEW_TASK)
        context.startActivity(intent)
    }
}

// SildMessengerActivity hosts the Compose messenger for one launch. It owns a
// SildClient bound to the activity's lifecycle scope, themed from the live brand.
class SildMessengerActivity : ComponentActivity() {
    private lateinit var client: SildClient
    private var tone: ToneGenerator? = null

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        val config = SildRuntime.config
        if (config == null) { finish(); return }

        tone = runCatching { ToneGenerator(AudioManager.STREAM_NOTIFICATION, 60) }.getOrNull()
        client = SildClient(config, lifecycleScope, onChime = { tone?.startTone(ToneGenerator.TONE_PROP_BEEP, 150) })

        // TARGET_LIST/TARGET_SUPPORT both land on Home (welcome + New conversation +
        // Recent), like the web launcher — the support request is created lazily on
        // the first message. A conv: target opens that thread directly (single-thread
        // mode: back finishes rather than returning to a Home that wasn't in the stack).
        val target = intent.getStringExtra(SildMessenger.EXTRA_TARGET) ?: SildMessenger.TARGET_LIST
        val rootIsHome = target == SildMessenger.TARGET_LIST || target == SildMessenger.TARGET_SUPPORT
        if (rootIsHome) client.start() else client.start(target.removePrefix(SildMessenger.CONV_PREFIX))

        setContent {
            val state by client.state.collectAsStateWithLifecycle()
            var draft by rememberSaveable { mutableStateOf(false) }
            SildTheme(state.brand) {
                val close = { finish() }
                when {
                    !rootIsHome -> ThreadScreen(client, state, draft = false, onBack = close, onCreated = {}, onClose = close)
                    // Draft takes precedence over activeId: openSupportRequest sets activeId
                    // before the first message lands, and switching to the created-thread
                    // branch here would dispose the draft composer (losing its text) if that
                    // first send then failed. onCreated — fired only on a successful first
                    // send — is what leaves the draft view.
                    draft -> ThreadScreen(client, state, draft = true, onBack = { draft = false }, onCreated = { draft = false }, onClose = close)
                    state.activeId != null -> ThreadScreen(client, state, draft = false, onBack = { client.backToList() }, onCreated = {}, onClose = close)
                    else -> HomeScreen(state, onNew = { draft = true }, onOpen = { client.openConversation(it) }, onToggleSound = { client.toggleSound() }, onClose = close)
                }
            }
        }
    }

    override fun onDestroy() {
        // client is lateinit — it stays uninitialized when onCreate finish()ed early
        // (no config, e.g. launched before Sild.init or recreated after process death).
        if (::client.isInitialized) client.destroy()
        tone?.release()
        super.onDestroy()
    }
}
