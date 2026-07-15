package io.sild.core

import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch
import kotlinx.serialization.json.Json
import java.util.UUID

// SildClient is the framework-agnostic orchestrator (the Kotlin peer of the web
// SildClient): §4.2 REST + §5 realtime, exposing an observable SildState the UI
// collects. It owns no Android types, so it runs and tests on a plain JVM.
//
// `scope` is supplied by the host (e.g. viewModelScope) and bounds all async work.
// `onChime` fires on an inbound reply while sound is on — the :ui layer plays the
// notification tone (audio can't live in pure Kotlin).
class SildClient(
    private val cfg: SildConfig,
    private val scope: CoroutineScope,
    private val onChime: () -> Unit = {},
) {
    private val api = SildApi(cfg)
    private val json = Json { ignoreUnknownKeys = true }
    private val selfId = cfg.userId
    private var activeNames: Map<String, String> = emptyMap()

    private val _state = MutableStateFlow(SildState())
    val state: StateFlow<SildState> = _state.asStateFlow()

    private val realtime = SildRealtime(
        cfg = cfg,
        scope = scope,
        onConnection = { conn -> _state.update { it.copy(connection = conn) } },
        onEnvelope = { onEvent(it) },
    )

    // ── lifecycle ──────────────────────────────────────────────────────────

    /** Load branding, connect realtime, then open [conversationId] or list all. */
    fun start(conversationId: String? = null) {
        scope.launch {
            _state.update { it.copy(connection = ConnectionState.CONNECTING, error = null) }
            runCatching {
                loadBrand()
                realtime.connect()
                // Always load the list first: it drives the App's conversation list
                // AND supplies the peer title/subtitle/author names for a directly
                // opened conversation (openConversation reads names from it).
                loadConversations()
                if (conversationId != null) openConversation(conversationId)
            }.onFailure { e ->
                _state.update { it.copy(error = e.message ?: "Failed to connect") }
            }
            _state.update { it.copy(ready = true) }
        }
    }

    private suspend fun loadBrand() {
        runCatching { api.fetchBrand() }.onSuccess { res ->
            _state.update { it.copy(brand = res.config, brandName = res.name) }
        }
    }

    /** GET /me/conversations → rows, with peer derivation matching the web client. */
    suspend fun loadConversations() {
        val list = runCatching { api.listConversations() }.getOrElse {
            _state.update { s -> s.copy(error = it.message) }; return
        }
        val convs = list.map { toConversation(it) }
        val named = convs.firstOrNull { it.agentName != null }?.agentName
        _state.update { it.copy(conversations = convs, agentName = named ?: it.agentName) }
    }

    /** Open a conversation and load its (last 100) messages. */
    fun openConversation(id: String) {
        scope.launch { loadThread(id) }
    }

    /** Set [id] active and load its thread. Suspends until the page is applied so a
     *  caller can safely send afterwards without the load clobbering the new message. */
    private suspend fun loadThread(id: String) {
        activeNames = _state.value.conversations.firstOrNull { it.id == id }?.names ?: emptyMap()
        _state.update { it.copy(activeId = id, loadingThread = true, messages = emptyList()) }
        runCatching { api.listMessages(id) }
            .onSuccess { page ->
                // A newer open() may have superseded this load while it was in flight —
                // applying it now would render this thread under another's header and
                // resolve its authors against the wrong members. Drop the stale result.
                if (!isActive(id)) return
                val msgs = page.messages.sortedBy { it.createdAt }.map { mapMessage(it) }
                _state.update { it.copy(messages = msgs, loadingThread = false, agentName = agentNameOf(msgs) ?: it.agentName) }
            }
            .onFailure { e ->
                if (!isActive(id)) return
                _state.update { it.copy(loadingThread = false, error = e.message) }
            }
    }

    /** Create a support request, open it, then reconnect so its channel is covered.
     *  [onResult] receives the new conversation id, or null if creation failed — so a
     *  caller (the draft composer) can reset its in-flight state on either outcome. */
    fun openSupportRequest(onResult: (String?) -> Unit = {}) {
        scope.launch {
            runCatching {
                val id = api.openSupportRequest()
                loadConversations()
                // Await the initial thread load: a send() fired from onResult must not
                // race the in-flight listMessages, which would overwrite the new message.
                loadThread(id)
                realtime.reconnect()
                id
            }.onSuccess { onResult(it) }
                .onFailure { e -> _state.update { it.copy(error = e.message) }; onResult(null) }
        }
    }

    /** Send a message (optionally with attachments) to the active conversation.
     *  [onResult] reports delivery so the caller can clear its draft only once the
     *  message is safely persisted (false on empty input, no active thread, or a
     *  transient failure) — never dropping the user's text on a failed send. */
    fun send(text: String, attachments: List<PendingAttachment> = emptyList(), onResult: (Boolean) -> Unit = {}) {
        val id = _state.value.activeId ?: return onResult(false)
        val body = text.trim()
        if (body.isEmpty() && attachments.isEmpty()) return onResult(false)
        scope.launch {
            runCatching { api.sendMessage(id, body, UUID.randomUUID().toString(), attachments) }
                .onSuccess { msg ->
                    // Only append to the thread the send targeted: the user may have
                    // navigated to another conversation while the POST was in flight.
                    if (isActive(id)) {
                        val mapped = mapMessage(msg)
                        _state.update { s ->
                            val msgs = if (s.messages.any { it.id == mapped.id }) s.messages
                                       else s.messages + mapped
                            s.copy(messages = msgs, error = null)
                        }
                    }
                    onResult(true)
                }
                // Surface the error only if this is still the open thread — the same
                // gate as the success path, so a failed send in A can't flash its error
                // onto B after the user switched. onResult still fires for the caller.
                .onFailure { e ->
                    if (isActive(id)) _state.update { it.copy(error = e.message) }
                    onResult(false)
                }
        }
    }

    /** Upload a file for attaching to the next message (§11 direct-to-bucket). */
    suspend fun upload(bytes: ByteArray, filename: String, mimeType: String): PendingAttachment =
        api.upload(bytes, filename, mimeType)

    fun backToList() {
        // Drop a thread-level error too, so a send failure doesn't follow the user
        // back to the list and resurface on Home.
        _state.update { it.copy(activeId = null, messages = emptyList(), error = null) }
        scope.launch { loadConversations() }
    }

    fun toggleSound() {
        _state.update { it.copy(soundOn = !it.soundOn) }
    }

    fun setSoundOn(on: Boolean) {
        _state.update { it.copy(soundOn = on) }
    }

    fun destroy() {
        realtime.destroy()
    }

    // ── realtime ─────────────────────────────────────────────────────────────

    /** Whether [id] is still the open conversation. Guards async results (thread
     *  loads, sends, realtime events) that may land after the user switched away. */
    private fun isActive(id: String?): Boolean = _state.value.activeId == id

    private fun onEvent(env: RealtimeEnvelope) {
        when (env.type) {
            "message.created" -> {
                if (!isActive(env.conversationId)) return
                val api = env.data?.let { runCatching { json.decodeFromJsonElement(ApiMessage.serializer(), it) }.getOrNull() } ?: return
                val msg = mapMessage(api)
                var appended = false
                _state.update { s ->
                    if (s.messages.any { it.id == msg.id }) return@update s // dedupe own echo
                    appended = true
                    val agent = if (msg.direction == Direction.IN && !msg.system && msg.author != null) msg.author else s.agentName
                    s.copy(messages = s.messages + msg, agentName = agent)
                }
                if (appended && msg.direction == Direction.IN && !msg.system && _state.value.soundOn) onChime()
            }
            "conversation.closed" -> {
                if (!isActive(env.conversationId)) return
                _state.update { s ->
                    s.copy(conversations = s.conversations.map { if (it.id == env.conversationId) it.copy(closed = true) else it })
                }
            }
        }
    }

    // ── mapping (matches the web mapMessage / peer derivation) ────────────────

    private fun toConversation(c: ApiConversation): Conversation {
        val agentName = c.agentName
        val peer = c.assignment == null && agentName == null && c.members.none { it.memberKind == "agent" }
        val names = HashMap<String, String>()
        for (m in c.members) {
            val ext = m.externalUserId ?: continue
            names[ext] = m.metadata.name() ?: ext
        }
        val other = c.members.firstOrNull { it.memberKind != "agent" && it.externalUserId != null && it.externalUserId != selfId }
        val otherName = other?.let { it.metadata.name() ?: it.externalUserId }
        val reference = c.reference ?: ""
        val closed = c.status == "closed" || c.assignment?.status == "closed"
        return Conversation(
            id = c.id,
            preview = c.lastMessage?.body ?: "No messages yet",
            time = clock(c.lastMessage?.createdAt),
            closed = closed,
            agentName = agentName,
            peer = peer,
            title = if (peer) otherName ?: "Direct chat" else agentName,
            subtitle = if (peer) "Direct chat" + (if (reference.isNotEmpty()) " · $reference" else "") else null,
            names = names,
        )
    }

    private fun mapMessage(m: ApiMessage): Message {
        val system = m.senderKind == "system"
        val isAgent = m.senderKind == "agent" || m.senderKind == "bot" || m.internalActorId != null
        val mine = m.senderKind == "user" && (selfId == null || m.externalUserId == selfId)
        val author: String? = when {
            system || mine -> null
            isAgent -> m.authorName ?: "Support"
            else -> (m.externalUserId?.let { activeNames[it] }) ?: m.authorName ?: m.externalUserId ?: "User"
        }
        return Message(
            id = m.id,
            direction = if (mine) Direction.OUT else Direction.IN,
            system = system,
            author = author,
            time = clock(m.createdAt),
            body = m.body,
            attachments = m.attachments.map {
                Attachment(rebaseLocalUrl(cfg.base, it.url), it.disposition, it.mimeType, it.filename, it.sizeBytes)
            },
        )
    }

    /** The last incoming non-system author — the header agent name. */
    private fun agentNameOf(msgs: List<Message>): String? =
        msgs.lastOrNull { it.direction == Direction.IN && !it.system && it.author != null }?.author
}
