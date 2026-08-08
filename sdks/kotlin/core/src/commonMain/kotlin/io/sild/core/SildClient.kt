package io.sild.core

import kotlin.uuid.ExperimentalUuidApi
import kotlin.uuid.Uuid
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Job
import kotlinx.coroutines.async
import kotlinx.coroutines.coroutineScope
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.getAndUpdate
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch
import kotlinx.serialization.json.Json

// SildClient is the framework-agnostic orchestrator (the Kotlin peer of the web
// SildClient): §4.2 REST + §5 realtime, exposing an observable SildState the UI
// collects. It owns no platform types, so it runs and tests on a plain JVM.
//
// `scope` is supplied by the host (e.g. viewModelScope) and bounds all async work.
// `onChime` fires on an inbound reply while sound is on — the :ui layer plays the
// notification tone (audio can't live in pure Kotlin).
// `transport` defaults to the platform's socket; where there is none the client is
// REST-only.
@OptIn(ExperimentalUuidApi::class)
class SildClient internal constructor(
    private val cfg: SildConfig,
    private val scope: CoroutineScope,
    private val onChime: () -> Unit,
    transport: RealtimeTransportFactory?,
    private val api: SildApi,
) {
    constructor(
        cfg: SildConfig,
        scope: CoroutineScope,
        onChime: () -> Unit = {},
        transport: RealtimeTransportFactory? = defaultRealtimeTransport(),
    ) : this(cfg, scope, onChime, transport, SildApi(cfg))

    private val json = Json { ignoreUnknownKeys = true }
    private val selfId = cfg.userId

    // The in-flight support-request creation, so leaving the draft (backToList) can
    // cancel it — otherwise its activeId update would reopen the thread after the user
    // navigated away.
    private var pendingOpen: Job? = null

    private val _state = MutableStateFlow(SildState())
    val state: StateFlow<SildState> = _state.asStateFlow()

    /** Configured client-side attachment size ceiling (bytes); see [SildConfig]. */
    val uploadSizeLimitBytes: Long get() = cfg.uploadSizeLimitBytes

    private val realtime = transport?.create(
        cfg,
        scope,
        { conn ->
            val reconnected = conn == ConnectionState.CONNECTED &&
                _state.getAndUpdate { it.copy(connection = conn) }.connection != ConnectionState.CONNECTED
            if (reconnected) catchUpOnReconnect()
        },
        { onEvent(it) },
    )

    // ── lifecycle ──────────────────────────────────────────────────────────

    /** Load branding, connect realtime, then open [conversationId] or list all. */
    fun start(conversationId: String? = null) {
        scope.launch {
            // With no transport nothing will ever report CONNECTED, so don't leave the
            // UI showing a connection that is permanently pending.
            val conn = if (realtime == null) ConnectionState.IDLE else ConnectionState.CONNECTING
            _state.update { it.copy(connection = conn, error = null) }
            // Fire-and-forget: never gate the support channel on the profile write.
            scope.launch { writeProfile() }
            runCatching {
                loadBrand()
                realtime?.connect()
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

    // ── push (§5.5) ────────────────────────────────────────────────────────

    /**
     * Registers this device for notifications with the token the HOST's push
     * integration was issued. Call again whenever that token rotates.
     */
    fun setPushToken(token: String, platform: String = SDK_PLATFORM, onResult: (Boolean) -> Unit = {}) {
        if (token.isEmpty()) {
            onResult(false)
            return
        }
        scope.launch { onResult(runCatching { api.registerPushToken(platform, token) }.isSuccess) }
    }

    /**
     * Releases this device on sign-out.
     *
     * [onResult] matters more here than on registration: a token outlives the
     * session, so a release that quietly failed leaves this device receiving the
     * previous user's messages. A host signing someone out should retry until
     * this succeeds.
     */
    fun clearPushToken(token: String, onResult: (Boolean) -> Unit = {}) {
        if (token.isEmpty()) {
            onResult(false)
            return
        }
        scope.launch { onResult(runCatching { api.deletePushToken(token) }.isSuccess) }
    }

    /**
     * Reports whether the host should display a notification for this payload.
     *
     * The host only reaches its own handler while the app is in the foreground —
     * backgrounded, the system displays the notification and no app code runs —
     * so the only question left is whether the user is already looking at that
     * conversation.
     */
    fun shouldShow(data: Map<String, String>): Boolean =
        SildPush.shouldShow(data, _state.value.activeId)

    /**
     * Upserts the configured profile, so Sild knows who this is before they ever
     * write a message. A missing profile costs a display name.
     */
    private suspend fun writeProfile() {
        runCatching { api.upsertOwnProfile() }
            .onFailure { println("sild: profile write failed: ${it.message}") }
    }

    private suspend fun loadBrand() {
        runCatching { api.fetchBrand() }.onSuccess { res ->
            _state.update { it.copy(brand = res.config, brandName = res.name) }
        }
    }

    /** GET /me/conversations → rows, with peer derivation matching the web client. */
    suspend fun loadConversations() {
        val list = runCatching { api.listConversations() }.getOrElse {
            // Propagate cancellation so a caller cancelled mid-load (e.g. openSupportRequest
            // after backToList) actually stops, rather than falling through to loadThread
            // and re-setting activeId — which would reopen the thread the user just left.
            if (it is CancellationException) throw it
            _state.update { s -> s.copy(error = it.message) }; return
        }
        val convs = list.map { toConversation(it) }
        val named = convs.firstOrNull { it.agentName != null }?.agentName
        _state.update { it.copy(conversations = convs, agentName = named ?: it.agentName) }
    }

    /** Prepend the next page of older messages to the open thread. */
    fun loadOlder() {
        scope.launch {
            val s = _state.value
            val id = s.activeId ?: return@launch
            val cursor = s.olderCursor ?: return@launch
            if (s.loadingOlder) return@launch
            _state.update { it.copy(loadingOlder = true) }
            runCatching { api.listMessages(id, cursor) }
                .onSuccess { page ->
                    val names = namesOf(id)
                    _state.update { cur ->
                        // The thread may have been switched or re-paged while this was in
                        // flight; applying it now would prepend another thread's history.
                        if (cur.activeId != id || cur.olderCursor != cursor) return@update cur
                        val older = newMessages(page.items.sortedBy { m -> m.createdAt }, cur.messages, names)
                        cur.copy(
                            messages = older + cur.messages,
                            olderCursor = if (page.hasMore) page.nextCursor else null,
                            loadingOlder = false,
                        )
                    }
                }
                .onFailure {
                    if (it is CancellationException) throw it
                    _state.update { cur ->
                        if (cur.activeId != id || cur.olderCursor != cursor) cur
                        else cur.copy(loadingOlder = false) // retry by scrolling again
                    }
                }
        }
    }

    /** Map [items] to messages, dropping the ones [held] already has. */
    private fun newMessages(
        items: List<ApiMessage>,
        held: List<Message>,
        names: Map<String, String>,
    ): List<Message> {
        val have = held.mapTo(HashSet()) { m -> m.id }
        return items.filterNot { m -> m.id in have }.map { m -> mapMessage(m, names) }
    }

    /** Open a conversation and load its (last 100) messages. */
    fun openConversation(id: String) {
        scope.launch { loadThread(id) }
    }

    /** Set [id] active and load its thread. Suspends until the page is applied so a
     *  caller can safely send afterwards without the load clobbering the new message. */
    private suspend fun loadThread(id: String) {
        // olderCursor belongs to the thread being left — carrying it over would page the
        // new one from the wrong place.
        _state.update {
            it.copy(
                activeId = id,
                loadingThread = true,
                messages = emptyList(),
                olderCursor = null,
                loadingOlder = false,
            )
        }
        // The row lookup and the page are independent; serialising them would put a
        // second round trip in front of every deep-linked open.
        val result = coroutineScope {
            val row = async { ensureConversation(id) }
            val messages = async { runCatching { api.listMessages(id) } }
            row.await()
            messages.await()
        }
        result
            .onSuccess { page ->
                // A newer open() may have superseded this load while it was in flight —
                // applying it now would render this thread under another's header and
                // resolve its authors against the wrong members. Drop the stale result.
                if (!isActive(id)) return
                val names = namesOf(id)
                val msgs = page.items.sortedBy { it.createdAt }.map { mapMessage(it, names) }
                _state.update {
                    it.copy(
                        messages = msgs,
                        loadingThread = false,
                        olderCursor = if (page.hasMore) page.nextCursor else null,
                        agentName = agentNameOf(msgs) ?: it.agentName,
                    )
                }
            }
            .onFailure { e ->
                if (e is CancellationException) throw e // cancelled (left the draft) — not an error
                if (!isActive(id)) return
                _state.update { it.copy(loadingThread = false, error = e.message) }
            }
    }

    /** Make sure [id]'s row is in state before its thread renders.
     *
     *  The recent list is one bounded page, so a conversation opened directly — a trip's
     *  driver chat, a deep link — need not be in it. Without its row the header falls
     *  back to "Support", no member name resolves, and a closed conversation still shows
     *  a live composer. A miss costs one extra GET; a hit costs nothing.
     */
    private suspend fun ensureConversation(id: String) {
        if (_state.value.conversations.any { it.id == id }) return
        val row = runCatching { api.getConversation(id) }.getOrElse {
            if (it is CancellationException) throw it
            return // the thread still loads; only its metadata is missing
        }
        val conv = toConversation(row)
        _state.update { s ->
            if (s.conversations.any { it.id == id }) return@update s
            val convs = s.conversations + conv
            s.copy(conversations = convs, agentName = convs.firstOrNull { c -> c.agentName != null }?.agentName ?: s.agentName)
        }
    }

    /** Create a support request, open it, then reconnect so its channel is covered.
     *  [onResult] receives the new conversation id, or null if creation failed — so a
     *  caller (the draft composer) can reset its in-flight state on either outcome. */
    fun openSupportRequest(onResult: (String?) -> Unit = {}) {
        pendingOpen?.cancel()
        pendingOpen = scope.launch {
            runCatching {
                val id = api.openSupportRequest()
                loadConversations()
                // Await the initial thread load: a send() fired from onResult must not
                // race the in-flight listMessages, which would overwrite the new message.
                loadThread(id)
                realtime?.reconnect()
                id
            }.onSuccess { onResult(it) }
                .onFailure { e ->
                    // Cancelled = the user left the draft; don't surface an error or report.
                    if (e is CancellationException) throw e
                    _state.update { it.copy(error = e.message) }
                    onResult(null)
                }
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
            runCatching { api.sendMessage(id, body, Uuid.random().toString(), attachments) }
                .onSuccess { msg ->
                    // Only append to the thread the send targeted: the user may have
                    // navigated to another conversation while the POST was in flight.
                    if (isActive(id)) {
                        val mapped = mapMessage(msg, namesOf(id))
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
        // Cancel an in-flight support-request creation so its activeId update can't
        // reopen the thread after the user has left. Drop a thread-level error too, so a
        // send failure doesn't follow the user back to the list and resurface on Home.
        pendingOpen?.cancel()
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
        realtime?.destroy()
    }

    // ── realtime ─────────────────────────────────────────────────────────────

    /** The reconnect sequence (§5.4): re-fetch the conversation list, then drain
     *  ?since= for the thread whose messages this client holds.
     *
     *  Reconnecting only re-establishes subscriptions; anything published while the
     *  connection was down was never sent to anyone and is not replayed. Without this
     *  the gap is permanent until the thread is reopened.
     *
     *  Drains rather than taking one page: has_more means the gap is longer than the
     *  limit, and stopping there would leave a hole in the middle of the thread.
     *
     *  Only the active thread is drained: it is the only one whose messages are in
     *  state, and the list refetch already carries every other conversation's
     *  current preview and unread count.
     */
    private fun catchUpOnReconnect() {
        scope.launch {
            // The list FIRST, awaited: a conversation added while offline is invisible
            // until its row is refetched, and namesOf() resolves authors off those
            // rows — draining concurrently would label the gap's messages against a
            // stale member set.
            runCatching { loadConversations() }

            val id = _state.value.activeId ?: return@launch
            // No messages yet means the thread load will fetch them anyway, and there
            // is no id to resume from.
            var since = _state.value.messages.lastOrNull()?.id ?: return@launch
            runCatching {
                while (true) {
                    val page = api.catchUpMessages(id, since)
                    if (page.items.isEmpty()) return@runCatching
                    // The user may have switched threads while this was in flight.
                    if (!isActive(id)) return@runCatching
                    val names = namesOf(id)
                    _state.update { s ->
                        val fresh = newMessages(page.items, s.messages, names)
                        if (fresh.isEmpty()) s else s.copy(messages = s.messages + fresh)
                    }
                    since = page.items.last().id
                    if (!page.hasMore) return@runCatching
                }
            }.onFailure { e ->
                if (e is CancellationException) throw e
                // A failed catch-up must not replace the thread with an error banner;
                // the next reconnect tries again from the same id.
            }
        }
    }

    /** Whether [id] is the open conversation — guards async results that land after
     *  the user switched away. */
    private fun isActive(id: String): Boolean = _state.value.activeId == id

    /** external_user_id → display name for [id]'s members. Read at use time so an
     *  interleaved open() can't resolve authors against another conversation. */
    private fun namesOf(id: String): Map<String, String> =
        _state.value.conversations.firstOrNull { it.id == id }?.names ?: emptyMap()

    private fun onEvent(env: RealtimeEnvelope) {
        // No conversation_id → can't attribute it to a thread.
        val convId = env.conversationId ?: return
        when (env.type) {
            "message.created" -> {
                val api = env.data?.let { runCatching { json.decodeFromJsonElement(ApiMessage.serializer(), it) }.getOrNull() } ?: return
                // Mapped only for the open thread — other authors would resolve against
                // the wrong members.
                val msg = if (isActive(convId)) mapMessage(api, namesOf(convId)) else null
                var appended = false
                // One emission for both effects: each one recomposes the collectors.
                _state.update { s ->
                    // The list row stays fresh even for a background conversation.
                    val convs = s.conversations.map {
                        if (it.id == convId) it.copy(preview = api.body.ifEmpty { "No messages yet" }, time = clock(api.createdAt)) else it
                    }
                    // Not the open thread, or our own echo.
                    if (msg == null || s.messages.any { it.id == msg.id }) return@update s.copy(conversations = convs)
                    appended = true
                    val agent = if (msg.direction == Direction.IN && !msg.system && msg.author != null) msg.author else s.agentName
                    s.copy(conversations = convs, messages = s.messages + msg, agentName = agent)
                }
                if (appended && msg != null && msg.direction == Direction.IN && !msg.system && _state.value.soundOn) onChime()
            }
            // Ungated: it only mutates the list row, which Home shows too.
            "conversation.closed" -> _state.update { s ->
                s.copy(conversations = s.conversations.map { if (it.id == convId) it.copy(closed = true) else it })
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

    private fun mapMessage(m: ApiMessage, names: Map<String, String>): Message {
        val system = m.senderKind == "system"
        val isAgent = m.senderKind == "agent" || m.senderKind == "bot" || m.internalActorId != null
        val mine = m.senderKind == "user" && (selfId == null || m.externalUserId == selfId)
        val author: String? = when {
            system || mine -> null
            isAgent -> m.authorName ?: "Support"
            else -> (m.externalUserId?.let { names[it] }) ?: m.authorName ?: m.externalUserId ?: "User"
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
