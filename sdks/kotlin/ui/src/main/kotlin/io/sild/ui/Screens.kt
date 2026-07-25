package io.sild.ui

import android.content.Context
import android.content.Intent
import android.net.Uri
import android.provider.OpenableColumns
import java.io.ByteArrayOutputStream
import androidx.activity.compose.rememberLauncherForActivityResult
import androidx.activity.result.contract.ActivityResultContracts
import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.offset
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Icon
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableIntStateOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.saveable.listSaver
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.runtime.snapshotFlow
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import coil.compose.AsyncImage
import io.sild.core.Conversation
import io.sild.core.PendingAttachment
import io.sild.core.SildClient
import io.sild.core.SildState
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext

// HomeScreen is the support landing — the exact counterpart of the web widget's
// Home: a brand-colored welcome header (logo/name, "agents online", heading, sub),
// a "Send us a message → New conversation" card, quick-start topics, and a Recent
// list. It replaces jumping straight into a thread: a support request is created
// lazily on the first message (draft flow), so opening support without a specific
// conversation lands here, matching the web drop-in.
@Composable
fun HomeScreen(state: SildState, onNew: () -> Unit, onOpen: (String) -> Unit, onToggleSound: () -> Unit, onClose: () -> Unit) {
    val colors = LocalSildColors.current
    val radii = LocalSildRadii.current
    val brand = state.brand
    Column(Modifier.fillMaxSize().background(colors.page)) {
        // Brand-colored welcome header.
        Column(Modifier.fillMaxWidth().background(colors.brand).padding(20.dp)) {
            Row(verticalAlignment = Alignment.CenterVertically) {
                val logo = brand.logoSrc
                if (logo != null) {
                    AsyncImage(model = logo, contentDescription = state.brandName, modifier = Modifier.height(28.dp))
                } else if (state.brandName.isNotEmpty()) {
                    Text(state.brandName, color = colors.onBrand, fontWeight = FontWeight.ExtraBold, fontSize = 18.sp, modifier = Modifier.weight(1f))
                } else {
                    Spacer(Modifier.weight(1f))
                }
                if (logo != null) Spacer(Modifier.weight(1f))
                SildHeaderControls(soundOn = state.soundOn, onToggleSound = onToggleSound, onClose = onClose)
            }
            if (brand.showTeam) {
                Spacer(Modifier.height(14.dp))
                TeamRow()
            }
            Spacer(Modifier.height(14.dp))
            Text(brand.heading, color = colors.onBrand, fontWeight = FontWeight.ExtraBold, fontSize = 26.sp)
            if (brand.sub.isNotEmpty()) {
                Text(brand.sub, color = colors.onBrand.copy(alpha = 0.9f), fontSize = 14.sp, modifier = Modifier.padding(top = 6.dp))
            }
        }
        // Body: New-conversation card, topics, Recent.
        Column(Modifier.fillMaxSize().verticalScroll(rememberScrollState()).padding(16.dp), verticalArrangement = Arrangement.spacedBy(12.dp)) {
            Column(Modifier.fillMaxWidth().clip(RoundedCornerShape(radii.card.dp)).background(colors.card).padding(16.dp)) {
                Text("Send us a message", color = colors.text, fontWeight = FontWeight.Bold, fontSize = 15.sp)
                Text("We'll get back to you here. No queue numbers.", color = colors.sub, fontSize = 13.sp, modifier = Modifier.padding(top = 4.dp))
                Row(
                    Modifier.fillMaxWidth().padding(top = 14.dp).clip(RoundedCornerShape(radii.btn.dp)).background(colors.brand).clickable(onClick = onNew).padding(vertical = 13.dp),
                    horizontalArrangement = Arrangement.Center,
                    verticalAlignment = Alignment.CenterVertically,
                ) {
                    Text("New conversation", color = colors.onBrand, fontWeight = FontWeight.Bold, fontSize = 15.sp)
                    Spacer(Modifier.size(8.dp))
                    Icon(SildIcons.Arrow, contentDescription = null, tint = colors.onBrand, modifier = Modifier.size(16.dp))
                }
            }
            brand.topicList.forEach { topic ->
                Row(
                    Modifier.fillMaxWidth().clip(RoundedCornerShape(radii.btn.dp)).background(colors.card).clickable(onClick = onNew).padding(horizontal = 14.dp, vertical = 13.dp),
                    verticalAlignment = Alignment.CenterVertically,
                ) {
                    Text(topic, color = colors.text, fontSize = 14.sp, modifier = Modifier.weight(1f))
                    Icon(SildIcons.Chevron, contentDescription = null, tint = colors.tertiary, modifier = Modifier.size(14.dp))
                }
            }
            if (state.conversations.isNotEmpty()) {
                Text("RECENT", color = colors.tertiary, fontSize = 11.sp, fontWeight = FontWeight.SemiBold, modifier = Modifier.padding(top = 4.dp))
                val fallback = state.agentName ?: "Support"
                Column(Modifier.fillMaxWidth().clip(RoundedCornerShape(radii.card.dp)).background(colors.card)) {
                    state.conversations.forEachIndexed { i, c ->
                        if (i > 0) HorizontalDivider(color = colors.border)
                        ConversationRow(c, fallback) { onOpen(c.id) }
                    }
                }
            }
            if (state.error != null) SildError(state.error!!)
            if (brand.poweredBy) {
                Text("Powered by Sild", color = colors.tertiary, fontSize = 11.sp, modifier = Modifier.fillMaxWidth().padding(top = 8.dp), textAlign = androidx.compose.ui.text.style.TextAlign.Center)
            }
        }
    }
}

// TeamRow mirrors the web widget's "2 agents online" flourish (shown when showTeam).
@Composable
private fun TeamRow() {
    val colors = LocalSildColors.current
    Row(verticalAlignment = Alignment.CenterVertically) {
        Box {
            TeamAvatar("E", Color(0xFF7C9CF5), Modifier)
            TeamAvatar("M", Color(0xFFE58A6B), Modifier.offset(x = 16.dp))
        }
        Spacer(Modifier.size(24.dp))
        Text("2 agents online", color = colors.onBrand.copy(alpha = 0.9f), fontSize = 13.sp)
    }
}

@Composable
private fun TeamAvatar(letter: String, bg: Color, modifier: Modifier) {
    Box(modifier.size(24.dp).clip(CircleShape).background(bg), contentAlignment = Alignment.Center) {
        Text(letter, color = Color.White, fontSize = 11.sp, fontWeight = FontWeight.Bold)
    }
}

@Composable
private fun ConversationRow(c: Conversation, fallback: String, onClick: () -> Unit) {
    val colors = LocalSildColors.current
    val title = c.title ?: c.agentName ?: fallback
    Row(
        Modifier.fillMaxWidth().clickable(onClick = onClick).padding(14.dp),
        horizontalArrangement = Arrangement.spacedBy(11.dp),
        verticalAlignment = Alignment.Top,
    ) {
        SildAvatar(title, size = 40, bg = colors.brand)
        Column(Modifier.weight(1f)) {
            Row(verticalAlignment = Alignment.CenterVertically) {
                Text(title, color = colors.text, fontWeight = FontWeight.SemiBold, fontSize = 14.sp, maxLines = 1, overflow = TextOverflow.Ellipsis, modifier = Modifier.weight(1f))
                if (c.time.isNotEmpty()) Text(c.time, color = colors.tertiary, fontSize = 11.sp)
            }
            Text(c.preview, color = colors.sub, fontSize = 13.sp, maxLines = 1, overflow = TextOverflow.Ellipsis, modifier = Modifier.padding(top = 2.dp))
            val sub = c.subtitle ?: (if (c.closed) "Closed" else null)
            if (!sub.isNullOrEmpty()) {
                Text(sub, color = colors.tertiary, fontSize = 11.sp, maxLines = 1, overflow = TextOverflow.Ellipsis, modifier = Modifier.padding(top = 3.dp))
            }
        }
    }
}

// ThreadScreen — one conversation (support or peer), or a draft support request
// that hasn't been created yet. In draft mode the header reads "Type your message
// to start" and the first send creates the support request, then sends — the same
// lazy-create the web widget does. Attachments are picked, uploaded, and sent with
// the message via the pending tray in SildComposer.
@Composable
fun ThreadScreen(client: SildClient, state: SildState, draft: Boolean, onBack: () -> Unit, onCreated: () -> Unit, onClose: () -> Unit) {
    val colors = LocalSildColors.current
    val context = LocalContext.current
    val scope = rememberCoroutineScope()
    var pending by rememberSaveable(stateSaver = PendingAttachmentsSaver) { mutableStateOf(emptyList<PendingAttachment>()) }
    // Not saved: a restored non-zero counter would disable send forever.
    var uploading by remember { mutableIntStateOf(0) }
    var attachError by remember { mutableStateOf<String?>(null) }

    val openUrl: (String) -> Unit = { url ->
        // Tenant-supplied data: any other scheme could redirect the user out of the app.
        val uri = Uri.parse(url)
        if (uri.scheme?.lowercase() in setOf("http", "https")) {
            runCatching { context.startActivity(Intent(Intent.ACTION_VIEW, uri)) }
        }
    }
    val picker = rememberLauncherForActivityResult(ActivityResultContracts.GetMultipleContents()) { uris ->
        if (uris.isNotEmpty()) attachError = null
        uris.forEach { uri ->
            uploading++
            scope.launch {
                try {
                    val file = withContext(Dispatchers.IO) { readFile(context, uri, client.uploadSizeLimitBytes) }
                    val att = client.upload(file.bytes, file.name, file.mime)
                    pending = pending + att
                } catch (e: Exception) {
                    // Say why — an oversized camera photo is the common case.
                    attachError = when (e) {
                        is IllegalArgumentException -> "That file is too large (max ${client.uploadSizeLimitBytes / (1024 * 1024)} MB)."
                        else -> "Couldn't attach that file. Please try again."
                    }
                } finally {
                    uploading--
                }
            }
        }
    }

    val active = state.conversations.firstOrNull { it.id == state.activeId }
    val peer = active?.peer == true
    val title = if (peer) active?.title ?: "Direct chat" else state.agentName ?: "Support"
    val subtitle = when {
        peer -> active?.subtitle ?: "Direct chat"
        draft -> "Type your message to start"
        state.connection.name == "CONNECTED" -> "Replies in a few minutes"
        else -> "Connecting…"
    }
    val closed = active?.closed == true
    val listState = rememberScrollState()
    // Observing maxValue follows the content; keying on message count races the layout
    // and leaves the newest bubble below the fold.
    LaunchedEffect(listState) {
        snapshotFlow { listState.maxValue }.collect { listState.animateScrollTo(it) }
    }

    Column(Modifier.fillMaxSize().background(colors.page)) {
        SildHeader(
            title = title, subtitle = subtitle, onBack = onBack, avatarName = title,
            action = { SildHeaderControls(soundOn = state.soundOn, onToggleSound = { client.toggleSound() }, onClose = onClose) },
        )
        Column(
            Modifier.weight(1f).fillMaxWidth().verticalScroll(listState).padding(horizontal = 14.dp, vertical = 16.dp),
            verticalArrangement = Arrangement.spacedBy(12.dp),
        ) {
            if (state.loadingThread && state.messages.isEmpty()) {
                Text("Loading…", color = colors.tertiary, fontSize = 13.sp)
            }
            state.messages.forEach { m -> SildMessageBubble(m, onOpenUrl = openUrl) }
            if (!state.loadingThread && state.messages.isEmpty()) {
                Text("Send a message to start the conversation.", color = colors.tertiary, fontSize = 13.sp)
            }
            Spacer(Modifier.height(4.dp))
        }
        if (closed) {
            Text(
                "This conversation is closed.",
                color = colors.sub, fontSize = 13.sp,
                modifier = Modifier.fillMaxWidth().background(colors.card).padding(14.dp),
            )
        }
        // Send/create/attachment failures surface here; the composer keeps the draft.
        val problem = state.error ?: attachError
        if (problem != null) {
            SildError(problem, Modifier.fillMaxWidth().background(colors.card).padding(horizontal = 14.dp))
        }
        SildComposer(
            pending = pending,
            uploading = uploading,
            enabled = !closed,
            onAttach = { picker.launch("*/*") },
            onRemove = { i -> pending = pending.filterIndexed { j, _ -> j != i } },
            onSend = { body, onResult ->
                val atts = pending
                if (body.isEmpty() && atts.isEmpty()) return@SildComposer onResult(false)
                // Clear the draft (text + attachments) only once the send is confirmed —
                // on any failure both are preserved so the user can retry.
                // On success remove only the attachments we actually sent — the picker
                // stays live during the request, so any added meanwhile are kept.
                val done = { ok: Boolean ->
                    if (ok) pending = pending.filterNot { it in atts }
                    onResult(ok)
                }
                if (draft && state.activeId == null) {
                    // First attempt: create the support request, then send into it (web
                    // parity). Leaving the draft view (onCreated) waits for a successful
                    // send; a creation OR send failure reports done(false) so the composer
                    // resets and the draft stays put with the error shown below the thread.
                    client.openSupportRequest { id ->
                        if (id == null) return@openSupportRequest done(false)
                        client.send(body, atts) { ok ->
                            if (ok) onCreated()
                            done(ok)
                        }
                    }
                } else {
                    // An existing thread, or a draft whose conversation was already created
                    // by a prior attempt whose send failed — (re)send, never re-create.
                    client.send(body, atts) { ok ->
                        if (draft && ok) onCreated()
                        done(ok)
                    }
                }
            },
        )
    }
}

// The bytes are already on the server, so four strings each survive a rotation.
private val PendingAttachmentsSaver = listSaver<List<PendingAttachment>, String>(
    save = { atts -> atts.flatMap { listOf(it.objectKey, it.disposition, it.mimeType, it.filename) } },
    restore = { flat -> flat.chunked(4).filter { it.size == 4 }.map { PendingAttachment(it[0], it[1], it[2], it[3]) } },
)

private class PickedFile(val bytes: ByteArray, val name: String, val mime: String)

/** Read a picked content Uri into bytes + display name + mime (off the main thread).
 *  Throws if the file exceeds [maxBytes] (caller drops it), so an oversized pick is
 *  rejected before it can be buffered. [maxBytes] is a client-side memory bound (see
 *  SildConfig.uploadSizeLimitBytes), not the backend's authoritative per-tenant limit. */
private fun readFile(context: Context, uri: Uri, maxBytes: Long): PickedFile {
    val cr = context.contentResolver
    var name = "file"
    var size: Long = -1
    cr.query(uri, arrayOf(OpenableColumns.DISPLAY_NAME, OpenableColumns.SIZE), null, null, null)?.use { c ->
        if (c.moveToFirst()) {
            val ni = c.getColumnIndex(OpenableColumns.DISPLAY_NAME)
            if (ni >= 0) c.getString(ni)?.let { name = it }
            val si = c.getColumnIndex(OpenableColumns.SIZE)
            if (si >= 0 && !c.isNull(si)) size = c.getLong(si)
        }
    }
    // Reject on reported size first (cheap early-out), but don't trust it: SIZE is
    // optional and can be wrong. Bound the actual read too, so a provider that reports
    // no/low size still can't OOM us — we stop the moment the stream passes the cap.
    require(size < 0 || size <= maxBytes) { "file too large" }
    val bytes = cr.openInputStream(uri)?.use { input ->
        val out = ByteArrayOutputStream()
        val chunk = ByteArray(64 * 1024)
        var total = 0L
        while (true) {
            val n = input.read(chunk)
            if (n < 0) break
            total += n
            require(total <= maxBytes) { "file too large" }
            out.write(chunk, 0, n)
        }
        out.toByteArray()
    } ?: ByteArray(0)
    val mime = cr.getType(uri) ?: "application/octet-stream"
    return PickedFile(bytes, name, mime)
}
