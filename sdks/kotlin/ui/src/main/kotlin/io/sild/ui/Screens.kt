package io.sild.ui

import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import io.sild.core.Conversation
import io.sild.core.SildClient
import io.sild.core.SildState

// ConversationListScreen — the App's "Messages" list. Each row shows the other
// party / agent, a preview, and the reference; peer rows read their derived title.
@Composable
fun ConversationListScreen(state: SildState, onOpen: (String) -> Unit, onBack: (() -> Unit)?) {
    val colors = LocalSildColors.current
    Column(Modifier.fillMaxSize().background(colors.page)) {
        SildHeader(title = "Messages", subtitle = null, onBack = onBack)
        LazyColumn(Modifier.fillMaxSize().background(colors.card)) {
            items(state.conversations, key = { it.id }) { c ->
                ConversationRow(c) { onOpen(c.id) }
                HorizontalDivider(color = colors.border)
            }
        }
    }
}

@Composable
private fun ConversationRow(c: Conversation, onClick: () -> Unit) {
    val colors = LocalSildColors.current
    val title = c.title ?: c.agentName ?: "Conversation"
    Row(
        Modifier.fillMaxWidth().clickable(onClick = onClick).padding(14.dp),
        horizontalArrangement = Arrangement.spacedBy(11.dp),
        verticalAlignment = Alignment.Top,
    ) {
        SildAvatar(title, size = 40)
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

// ThreadScreen — one conversation (support or peer). The header title/subtitle
// come from the derived Conversation (peer shows "Direct chat · <ref>").
@Composable
fun ThreadScreen(client: SildClient, state: SildState, onBack: () -> Unit) {
    val colors = LocalSildColors.current
    val context = androidx.compose.ui.platform.LocalContext.current
    val openUrl: (String) -> Unit = { url ->
        runCatching {
            context.startActivity(android.content.Intent(android.content.Intent.ACTION_VIEW, android.net.Uri.parse(url)))
        }
    }
    val active = state.conversations.firstOrNull { it.id == state.activeId }
    val title = active?.title ?: state.agentName ?: "Support"
    val subtitle = active?.subtitle ?: if (state.connection.name == "CONNECTED") "Replies in a few minutes" else "Connecting…"
    val listState = rememberScrollState()
    LaunchedEffect(state.messages.size) { listState.scrollTo(listState.maxValue) }

    Column(Modifier.fillMaxSize().background(colors.page)) {
        SildHeader(title = title, subtitle = subtitle, onBack = onBack, avatarName = title)
        Column(
            Modifier.weight(1f).fillMaxWidth().verticalScroll(listState).padding(horizontal = 14.dp, vertical = 16.dp),
            verticalArrangement = Arrangement.spacedBy(12.dp),
        ) {
            if (state.loadingThread && state.messages.isEmpty()) {
                Text("Loading…", color = colors.tertiary, fontSize = 13.sp)
            }
            state.messages.forEach { m -> SildMessageBubble(m, onOpenUrl = openUrl) }
            Spacer(Modifier.height(4.dp))
        }
        val closed = active?.closed == true
        if (closed) {
            Text(
                "This conversation is closed.",
                color = colors.sub, fontSize = 13.sp,
                modifier = Modifier.fillMaxWidth().background(colors.card).padding(14.dp),
            )
        } else {
            SildComposer(onSend = { client.send(it) })
        }
    }
}
