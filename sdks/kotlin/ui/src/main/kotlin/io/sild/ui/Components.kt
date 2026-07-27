package io.sild.ui

import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.clickable
import androidx.compose.foundation.horizontalScroll
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.widthIn
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.text.BasicTextField
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.SolidColor
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.TextStyle
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import coil.compose.AsyncImage
import io.sild.core.Attachment
import io.sild.core.BrandTheme
import io.sild.core.Direction
import io.sild.core.Message
import io.sild.core.PendingAttachment

@Composable
fun SildAvatar(name: String, size: Int = 36, bg: Color? = null) {
    Box(
        Modifier.size(size.dp).clip(CircleShape).background(bg ?: parseColor(BrandTheme.avatarColor(name))),
        contentAlignment = Alignment.Center,
    ) {
        Text(BrandTheme.avatarInitial(name), color = Color.White, fontSize = (size * 0.38).sp, fontWeight = FontWeight.Bold)
    }
}

// SildHeader is the brand-colored thread top bar: optional back button, an avatar,
// a title + subtitle (peer chats show "Direct chat · <ref>"), and an optional
// trailing action (the sound toggle) pinned right — mirroring the web widget's
// shared header control cluster.
@Composable
fun SildHeader(
    title: String,
    subtitle: String?,
    onBack: (() -> Unit)?,
    avatarName: String? = null,
    action: (@Composable () -> Unit)? = null,
) {
    val colors = LocalSildColors.current
    Row(
        Modifier.fillMaxWidth().background(colors.brand).padding(horizontal = 12.dp, vertical = 12.dp),
        verticalAlignment = Alignment.CenterVertically,
        horizontalArrangement = Arrangement.spacedBy(10.dp),
    ) {
        if (onBack != null) {
            IconButton(onClick = onBack) {
                Icon(SildIcons.Back, contentDescription = "Back", tint = colors.onBrand)
            }
        }
        if (avatarName != null) SildAvatar(avatarName, size = 36)
        Column(Modifier.weight(1f)) {
            Text(title, color = colors.onBrand, fontWeight = FontWeight.Bold, fontSize = 16.sp, maxLines = 1, overflow = TextOverflow.Ellipsis)
            if (!subtitle.isNullOrEmpty()) {
                Text(subtitle, color = colors.onBrand.copy(alpha = 0.8f), fontSize = 12.sp, maxLines = 1, overflow = TextOverflow.Ellipsis)
            }
        }
        if (action != null) action()
    }
}

// SildSoundToggle mirrors the web widget's reply-notification control: a speaker
// glyph that swaps to the slashed variant when muted. White to sit on the brand header.
@Composable
fun SildSoundToggle(on: Boolean, onToggle: () -> Unit) {
    val colors = LocalSildColors.current
    IconButton(onClick = onToggle) {
        Icon(
            if (on) SildIcons.Speaker else SildIcons.SpeakerOff,
            contentDescription = if (on) "Turn off reply notifications" else "Turn on reply notifications",
            tint = colors.onBrand,
        )
    }
}

// SildHeaderControls is the web widget's shared top-right cluster (sound + close),
// rendered on every screen so the icons never shift position. Close dismisses the
// messenger (finishes the activity) — the native equivalent of closing the panel.
@Composable
fun SildHeaderControls(soundOn: Boolean, onToggleSound: () -> Unit, onClose: () -> Unit) {
    val colors = LocalSildColors.current
    Row(verticalAlignment = Alignment.CenterVertically) {
        SildSoundToggle(on = soundOn, onToggle = onToggleSound)
        IconButton(onClick = onClose) {
            Icon(SildIcons.Close, contentDescription = "Close", tint = colors.onBrand)
        }
    }
}

// SildMessageBubble renders one message: system lines centered; the visitor's own
// (OUT) messages right-aligned in brand color; everyone else left-aligned with an
// author label. Inline images render via Coil; other files as tappable chips.
@Composable
fun SildMessageBubble(message: Message, onOpenUrl: (String) -> Unit) {
    val colors = LocalSildColors.current
    val radii = LocalSildRadii.current
    if (message.system) {
        Row(Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.Center) {
            Text(message.body, color = colors.tertiary, fontSize = 12.sp, modifier = Modifier.padding(vertical = 4.dp))
        }
        return
    }
    val out = message.direction == Direction.OUT
    Column(
        Modifier.fillMaxWidth(),
        horizontalAlignment = if (out) Alignment.End else Alignment.Start,
    ) {
        // Meta row above the bubble (web parity): author + time. Own messages are
        // labelled "You"; incoming show the sender's name. Time sits here, not below.
        val label = if (out) "You" else message.author
        if (label != null || message.time.isNotEmpty()) {
            Row(
                Modifier.padding(start = 4.dp, end = 4.dp, bottom = 4.dp),
                horizontalArrangement = Arrangement.spacedBy(7.dp),
                verticalAlignment = Alignment.CenterVertically,
            ) {
                if (label != null) Text(label, color = colors.sub, fontSize = 12.sp, fontWeight = FontWeight.SemiBold)
                if (message.time.isNotEmpty()) Text(message.time, color = colors.tertiary, fontSize = 11.sp)
            }
        }
        message.attachments.filter { it.isInlineImage }.forEach { att ->
            AsyncImage(
                model = att.url,
                contentDescription = att.filename,
                modifier = Modifier.padding(bottom = 4.dp).size(220.dp).clip(RoundedCornerShape(radii.card.dp)),
            )
        }
        if (message.body.isNotEmpty()) {
            Box(
                Modifier
                    .clip(RoundedCornerShape(radii.bubble.dp))
                    .background(if (out) colors.brand else colors.sunken)
                    .padding(horizontal = 13.dp, vertical = 9.dp),
            ) {
                Text(message.body, color = if (out) colors.onBrand else colors.text, fontSize = 14.sp)
            }
        }
        message.attachments.filter { !it.isInlineImage }.forEach { att ->
            FileChip(att, onOpenUrl)
        }
    }
}

@Composable
private fun FileChip(att: Attachment, onOpenUrl: (String) -> Unit) {
    val colors = LocalSildColors.current
    val radii = LocalSildRadii.current
    Row(
        Modifier
            .padding(top = 4.dp)
            .clip(RoundedCornerShape(radii.btn.dp))
            .background(colors.card)
            // Tap to open/download the file the backend exposes (matches the web
            // widget's attachment link); no-op if the server didn't return a URL.
            .let { if (att.url != null) it.clickable { onOpenUrl(att.url!!) } else it }
            .padding(horizontal = 11.dp, vertical = 8.dp),
        verticalAlignment = Alignment.CenterVertically,
        horizontalArrangement = Arrangement.spacedBy(7.dp),
    ) {
        Icon(SildIcons.Clip, contentDescription = null, tint = colors.tertiary, modifier = Modifier.size(16.dp))
        Text(att.filename.ifEmpty { "attachment" }, color = colors.text, fontSize = 13.sp, maxLines = 1, overflow = TextOverflow.Ellipsis)
    }
}

// Inline error line shown wherever the client surfaces a failure (Home, thread).
// One place to change error styling / add a retry affordance later.
@Composable
fun SildError(text: String, modifier: Modifier = Modifier) {
    Text(text, color = LocalSildColors.current.tertiary, fontSize = 12.sp, modifier = modifier)
}

// SildComposer is the message input row: a pending-attachment tray, an attach
// button, the text field, and a send button — mirroring the web widget. Picked
// files show as removable chips (plus an "Uploading…" chip while in flight) and
// are sent together with the text; send is disabled until an upload settles.
@Composable
fun SildComposer(
    pending: List<PendingAttachment>,
    uploading: Int,
    // Reports the message to send and a completion callback; the composer clears
    // the input only once that reports success, so a failed send keeps the draft.
    onSend: (String, (Boolean) -> Unit) -> Unit,
    onAttach: () -> Unit,
    onRemove: (Int) -> Unit,
    enabled: Boolean = true,
) {
    val colors = LocalSildColors.current
    val radii = LocalSildRadii.current
    // `sending` deliberately isn't saved: restoring it true would wedge the button.
    var text by rememberSaveable { mutableStateOf("") }
    var sending by remember { mutableStateOf(false) }
    val canSend = enabled && !sending && uploading == 0 && (text.isNotBlank() || pending.isNotEmpty())
    // .composer: card surface with a top hairline border.
    Column(Modifier.fillMaxWidth().background(colors.card)) {
        HorizontalDivider(color = colors.border)
        Column(Modifier.padding(start = 12.dp, end = 12.dp, top = 10.dp, bottom = 12.dp)) {
            // .pending — removable chips + an "Uploading…" chip while in flight.
            if (pending.isNotEmpty() || uploading > 0) {
                Row(
                    Modifier.fillMaxWidth().horizontalScroll(rememberScrollState()).padding(bottom = 8.dp),
                    horizontalArrangement = Arrangement.spacedBy(6.dp),
                    verticalAlignment = Alignment.CenterVertically,
                ) {
                    pending.forEachIndexed { i, att ->
                        Row(
                            Modifier.clip(RoundedCornerShape(radii.btn.dp)).background(colors.sunken)
                                .border(1.dp, colors.border, RoundedCornerShape(radii.btn.dp))
                                .padding(start = 10.dp, end = 4.dp, top = 5.dp, bottom = 5.dp),
                            verticalAlignment = Alignment.CenterVertically,
                            horizontalArrangement = Arrangement.spacedBy(4.dp),
                        ) {
                            Text(att.filename.ifEmpty { "attachment" }, color = colors.sub, fontSize = 12.sp, maxLines = 1, overflow = TextOverflow.Ellipsis, modifier = Modifier.widthIn(max = 180.dp))
                            Box(Modifier.size(18.dp).clip(CircleShape).clickable { onRemove(i) }, contentAlignment = Alignment.Center) {
                                Icon(SildIcons.Close, contentDescription = "Remove attachment", tint = colors.tertiary, modifier = Modifier.size(13.dp))
                            }
                        }
                    }
                    if (uploading > 0) {
                        Text("Uploading…", color = colors.tertiary, fontSize = 12.sp)
                    }
                }
            }
            // .inputwrap — one rounded, bordered pill holding attach + field + send.
            Row(
                Modifier.fillMaxWidth().clip(RoundedCornerShape(radii.card.dp))
                    .border(1.dp, colors.border, RoundedCornerShape(radii.card.dp))
                    .padding(start = 8.dp, end = 6.dp, top = 6.dp, bottom = 6.dp),
                verticalAlignment = Alignment.Bottom,
                horizontalArrangement = Arrangement.spacedBy(8.dp),
            ) {
                Box(
                    Modifier.size(34.dp).clip(RoundedCornerShape(radii.btn.dp)).clickable(enabled = enabled, onClick = onAttach),
                    contentAlignment = Alignment.Center,
                ) {
                    Icon(SildIcons.Clip, contentDescription = "Attach a file", tint = colors.tertiary, modifier = Modifier.size(20.dp))
                }
                Box(Modifier.weight(1f).padding(vertical = 6.dp), contentAlignment = Alignment.CenterStart) {
                    if (text.isEmpty()) Text("Message…", color = colors.tertiary, fontSize = 14.sp)
                    BasicTextField(
                        value = text,
                        onValueChange = { text = it },
                        enabled = enabled,
                        textStyle = TextStyle(color = colors.text, fontSize = 14.sp, lineHeight = 21.sp),
                        cursorBrush = SolidColor(colors.brand),
                        maxLines = 4,
                        modifier = Modifier.fillMaxWidth().semantics { contentDescription = "Message input" },
                    )
                }
                Box(
                    Modifier.size(34.dp).clip(RoundedCornerShape(radii.btn.dp))
                        .background(if (canSend) colors.brand else colors.brand.copy(alpha = 0.4f))
                        .clickable(enabled = canSend) {
                            val sent = text.trim()
                            sending = true
                            onSend(sent) { ok ->
                                sending = false
                                // Clear only if the field still holds exactly what we sent —
                                // the input stays editable during the request, so anything the
                                // user typed meanwhile is theirs to keep (and a failure keeps
                                // the draft either way).
                                if (ok && text.trim() == sent) text = ""
                            }
                        },
                    contentAlignment = Alignment.Center,
                ) {
                    Icon(SildIcons.Send, contentDescription = "Send", tint = colors.onBrand, modifier = Modifier.size(18.dp))
                }
            }
        }
    }
}
