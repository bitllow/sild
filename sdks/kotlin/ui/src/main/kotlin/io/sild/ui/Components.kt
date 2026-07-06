package io.sild.ui

import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.ArrowBack
import androidx.compose.material.icons.filled.AttachFile
import androidx.compose.material.icons.automirrored.filled.Send
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import coil.compose.AsyncImage
import io.sild.core.Attachment
import io.sild.core.Direction
import io.sild.core.Message

private val AVATAR_PALETTE = listOf(
    0xFF3D63FF, 0xFFFF7A45, 0xFF18A957, 0xFF7C5CFF, 0xFF0EA5A5, 0xFFE0599B, 0xFFD9881A, 0xFF2440B8,
)

private fun initials(name: String): String {
    val parts = name.trim().split(Regex("\\s+")).filter { it.isNotEmpty() }
    if (parts.isEmpty()) return "?"
    if (parts.size == 1) return parts[0].take(2).uppercase()
    return "${parts.first().first()}${parts.last().first()}".uppercase()
}

private fun colorFor(name: String): Color {
    var h = 0
    for (ch in name) h = (h * 31 + ch.code) and 0x7FFFFFFF
    return Color(AVATAR_PALETTE[h % AVATAR_PALETTE.size])
}

@Composable
fun SildAvatar(name: String, size: Int = 36, bg: Color? = null) {
    Box(
        Modifier.size(size.dp).clip(CircleShape).background(bg ?: colorFor(name)),
        contentAlignment = Alignment.Center,
    ) {
        Text(initials(name), color = Color.White, fontSize = (size * 0.38).sp, fontWeight = FontWeight.Bold)
    }
}

// SildHeader is the brand-colored thread/list top bar: optional back button, an
// avatar, and a title + subtitle (peer chats show "Direct chat · <ref>").
@Composable
fun SildHeader(title: String, subtitle: String?, onBack: (() -> Unit)?, avatarName: String? = null) {
    val colors = LocalSildColors.current
    Row(
        Modifier.fillMaxWidth().background(colors.brand).padding(horizontal = 12.dp, vertical = 12.dp),
        verticalAlignment = Alignment.CenterVertically,
        horizontalArrangement = Arrangement.spacedBy(10.dp),
    ) {
        if (onBack != null) {
            IconButton(onClick = onBack) {
                Icon(Icons.AutoMirrored.Filled.ArrowBack, contentDescription = "Back", tint = colors.onBrand)
            }
        }
        if (avatarName != null) SildAvatar(avatarName, size = 36)
        Column(Modifier.weight(1f)) {
            Text(title, color = colors.onBrand, fontWeight = FontWeight.Bold, fontSize = 16.sp, maxLines = 1, overflow = TextOverflow.Ellipsis)
            if (!subtitle.isNullOrEmpty()) {
                Text(subtitle, color = colors.onBrand.copy(alpha = 0.8f), fontSize = 12.sp, maxLines = 1, overflow = TextOverflow.Ellipsis)
            }
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
        if (!out && message.author != null) {
            Text(message.author!!, color = colors.sub, fontSize = 12.sp, fontWeight = FontWeight.SemiBold, modifier = Modifier.padding(start = 4.dp, bottom = 2.dp))
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
        if (message.time.isNotEmpty()) {
            Text(message.time, color = colors.tertiary, fontSize = 11.sp, modifier = Modifier.padding(top = 2.dp, start = 4.dp, end = 4.dp))
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
        Icon(Icons.Filled.AttachFile, contentDescription = null, tint = colors.tertiary, modifier = Modifier.size(16.dp))
        Text(att.filename.ifEmpty { "attachment" }, color = colors.text, fontSize = 13.sp, maxLines = 1, overflow = TextOverflow.Ellipsis)
    }
}

// SildComposer is the message input row: an attach button, the text field, and a
// send button (enabled when there is text). Attachments picked externally flow
// through onSend; this first cut wires text send + an attach hook.
@Composable
fun SildComposer(onSend: (String) -> Unit, onAttach: (() -> Unit)? = null, enabled: Boolean = true) {
    val colors = LocalSildColors.current
    var text by remember { mutableStateOf("") }
    Row(
        Modifier.fillMaxWidth().background(colors.card).padding(10.dp),
        verticalAlignment = Alignment.CenterVertically,
        horizontalArrangement = Arrangement.spacedBy(6.dp),
    ) {
        if (onAttach != null) {
            IconButton(onClick = onAttach, enabled = enabled) {
                Icon(Icons.Filled.AttachFile, contentDescription = "Attach a file", tint = colors.tertiary)
            }
        }
        OutlinedTextField(
            value = text,
            onValueChange = { text = it },
            modifier = Modifier.weight(1f),
            placeholder = { Text("Write a message…") },
            enabled = enabled,
            maxLines = 4,
        )
        IconButton(
            onClick = { if (text.isNotBlank()) { onSend(text.trim()); text = "" } },
            enabled = enabled && text.isNotBlank(),
        ) {
            Icon(Icons.AutoMirrored.Filled.Send, contentDescription = "Send", tint = colors.brand)
        }
    }
}
