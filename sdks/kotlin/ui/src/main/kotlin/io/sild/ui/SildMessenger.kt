package io.sild.ui

import android.content.Context
import android.content.Intent
import android.media.AudioManager
import android.media.ToneGenerator
import android.os.Bundle
import androidx.activity.ComponentActivity
import androidx.activity.compose.setContent
import androidx.compose.runtime.CompositionLocalProvider
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.lifecycle.ViewModel
import androidx.lifecycle.ViewModelProvider
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.lifecycle.viewModelScope
import io.sild.core.SildClient
import io.sild.core.SildConfig
import io.sild.core.SildHost
import io.sild.core.SildStringStore

// SildRuntime holds the host-supplied config process-wide. The config carries a
// TokenProvider (not parcelable), so the launcher stores it here and the messenger
// Activity reads it — the same "init once, open many" model as the web Sild.init.
object SildRuntime {
    // One holder, shared with the push calls in :core — a second one here would
    // let a host configure the messenger and still fail to register a device.
    var config: SildConfig?
        get() = SildHost.config
        set(value) {
            SildHost.config = value
        }
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

    /**
     * Init with a Context, so the strings a tenant published survive the app being
     * killed: Android's small-value store is behind a Context, which :core has no
     * way to reach. Without it the messenger still works and renders the text
     * bundled in the app until it has polled once.
     */
    fun init(context: Context, config: SildConfig): SildMessenger =
        init(config.copy(stringStore = config.stringStore ?: SildPreferences(context)))

    // Push (§5.5). The host owns its FCM registration (ADR 0001) and hands the token
    // here; the work is SildHost's, in :core, so Android and iOS run one implementation.

    /** Forward the FCM token you were issued, and again whenever it rotates. */
    fun setPushToken(token: String, onResult: (Boolean) -> Unit = {}) =
        SildHost.setPushToken(token, onResult)

    /** Release this device on sign-out, so the next user of it hears nothing. */
    fun clearPushToken(token: String, onResult: (Boolean) -> Unit = {}) =
        SildHost.clearPushToken(token, onResult)

    /**
     * Whether to display a nudge that arrived while the app was in the foreground.
     * False for the conversation the messenger is showing, and for anything that is
     * not ours.
     */
    fun shouldShow(data: Map<String, String>): Boolean = SildHost.shouldShow(data)
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

// SildPreferences keeps downloaded strings in the app's own SharedPreferences —
// small values, written off the main thread by the fetch that produced them.
private class SildPreferences(context: Context) : SildStringStore {
    private val prefs = context.applicationContext
        .getSharedPreferences("io.sild.strings", Context.MODE_PRIVATE)

    override fun read(key: String): String? = prefs.getString(key, null)

    override fun write(key: String, value: String) {
        prefs.edit().putString(key, value).apply()
    }
}

// SildSession owns the SildClient in a ViewModel, not the Activity, so a
// configuration change doesn't drop the connection or bounce the user back to Home.
internal class SildSession(cfg: SildConfig) : ViewModel() {
    private val tone = runCatching { ToneGenerator(AudioManager.STREAM_NOTIFICATION, 60) }.getOrNull()

    val client = SildClient(cfg, viewModelScope, onChime = { tone?.startTone(ToneGenerator.TONE_PROP_BEEP, 150) })

    private var started = false

    /** First creation only: a rotation must not re-run start() and re-open the target. */
    fun startOnce(conversationId: String?) {
        if (started) return
        started = true
        client.start(conversationId)
    }

    override fun onCleared() {
        client.destroy()
        tone?.release()
    }

    class Factory(private val cfg: SildConfig) : ViewModelProvider.Factory {
        @Suppress("UNCHECKED_CAST")
        override fun <T : ViewModel> create(modelClass: Class<T>): T = SildSession(cfg) as T
    }
}

// SildMessengerActivity hosts the Compose messenger, themed from the live brand.
class SildMessengerActivity : ComponentActivity() {
    private var session: SildSession? = null

    // Resumed, not merely alive: a messenger sitting behind another of the host's
    // screens is not what the user is reading, and must not suppress its nudges.
    override fun onResume() {
        super.onResume()
        SildHost.onScreen = session?.client
        // Coming back to the front re-checks a held manifest that has aged out. What
        // it downloads is staged for the next start, never applied here.
        session?.client?.onForeground()
    }

    override fun onPause() {
        super.onPause()
        SildHost.onScreen = null
    }

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        val config = SildRuntime.config
        if (config == null) { finish(); return }

        // TARGET_LIST/TARGET_SUPPORT both land on Home (welcome + New conversation +
        // Recent), like the web launcher — the support request is created lazily on
        // the first message. A conv: target opens that thread directly (single-thread
        // mode: back finishes rather than returning to a Home that wasn't in the stack).
        val target = intent.getStringExtra(SildMessenger.EXTRA_TARGET) ?: SildMessenger.TARGET_LIST
        val rootIsHome = target == SildMessenger.TARGET_LIST || target == SildMessenger.TARGET_SUPPORT

        val session = ViewModelProvider(this, SildSession.Factory(config))[SildSession::class.java]
        session.startOnce(if (rootIsHome) null else target.removePrefix(SildMessenger.CONV_PREFIX))
        val client = session.client
        this.session = session

        setContent {
            val state by client.state.collectAsStateWithLifecycle()
            var draft by rememberSaveable { mutableStateOf(false) }
            // Re-derived when the language or the strings behind it move, so a
            // published update redraws the screens instead of sitting in memory.
            val strings: SildStrings = remember(state.locale, state.stringsRevision) {
                { key, vars -> client.i18n.t(key, vars) }
            }
            val plurals: SildPlurals = remember(state.locale, state.stringsRevision) {
                { base, count, vars -> client.i18n.tPlural(base, count, vars) }
            }
            CompositionLocalProvider(LocalSildStrings provides strings, LocalSildPlurals provides plurals) {
            SildTheme(state.brand) {
                val close = { finish() }
                when {
                    !rootIsHome -> ThreadScreen(client, state, draft = false, onBack = close, onCreated = {}, onClose = close)
                    // ONE ThreadScreen call site spans both draft and created states so its
                    // composer keeps its remembered text/attachments across the transition.
                    // openSupportRequest sets activeId before the first send finishes, and
                    // onCreated only flips draft=false on success; a separate call site per
                    // state would dispose the composer and lose whatever the user typed while
                    // that first send was in flight (or the whole draft if the send failed).
                    draft || state.activeId != null -> ThreadScreen(
                        client, state,
                        draft = draft,
                        // Leave to the list in one press: backToList clears activeId AND
                        // cancels any in-flight support-request creation, so a conversation
                        // created (or still creating) for this draft can't keep us here or
                        // reopen the thread after we've left.
                        onBack = { draft = false; client.backToList() },
                        onCreated = { draft = false },
                        onClose = close,
                    )
                    else -> HomeScreen(state, onNew = { draft = true }, onOpen = { client.openConversation(it) }, onToggleSound = { client.toggleSound() }, onClose = close)
                }
            }
            }
        }
    }
}
