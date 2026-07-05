package io.sild.core

import kotlinx.serialization.SerialName
import kotlinx.serialization.Serializable
import kotlinx.serialization.json.JsonObject

// ── Wire DTOs (exact §4.2 JSON field names) ─────────────────────────────────

@Serializable
data class ApiAttachment(
    @SerialName("object_key") val objectKey: String = "",
    val disposition: String = "attachment", // inline | attachment
    @SerialName("mime_type") val mimeType: String = "",
    @SerialName("size_bytes") val sizeBytes: Long = 0,
    val filename: String = "",
    val url: String? = null,
)

@Serializable
data class ApiMessage(
    val id: String,
    @SerialName("conversation_id") val conversationId: String = "",
    @SerialName("sender_kind") val senderKind: String = "user", // user | agent | bot | system
    val visibility: String = "participants",
    val body: String = "",
    @SerialName("created_at") val createdAt: String = "",
    @SerialName("external_user_id") val externalUserId: String? = null,
    @SerialName("internal_actor_id") val internalActorId: String? = null,
    @SerialName("author_name") val authorName: String? = null,
    val attachments: List<ApiAttachment> = emptyList(),
)

@Serializable
data class ApiMember(
    @SerialName("member_kind") val memberKind: String = "user",
    @SerialName("conv_role") val convRole: String = "",
    @SerialName("external_user_id") val externalUserId: String? = null,
    @SerialName("internal_actor_id") val internalActorId: String? = null,
    val metadata: JsonObject? = null,
)

@Serializable
data class ApiLastMessage(val body: String? = null, @SerialName("created_at") val createdAt: String? = null)

@Serializable
data class ApiAssignment(val status: String? = null)

@Serializable
data class ApiConversation(
    val id: String,
    val status: String? = null,
    val reference: String? = null,
    val members: List<ApiMember> = emptyList(),
    val assignment: ApiAssignment? = null,
    @SerialName("agent_name") val agentName: String? = null,
    @SerialName("last_message") val lastMessage: ApiLastMessage? = null,
)

@Serializable
data class ApiMessagesPage(val messages: List<ApiMessage> = emptyList())

@Serializable
data class ApiUploadGrant(
    @SerialName("object_key") val objectKey: String,
    @SerialName("upload_url") val uploadUrl: String,
)

// ── Domain types the UI renders ─────────────────────────────────────────────

enum class Direction { IN, OUT }

enum class ConnectionState { IDLE, CONNECTING, CONNECTED, DISCONNECTED }

data class Attachment(
    val url: String?,
    val disposition: String,
    val mimeType: String,
    val filename: String,
    val sizeBytes: Long,
) {
    val isInlineImage: Boolean get() = disposition == "inline" && mimeType.startsWith("image/") && url != null
}

data class Message(
    val id: String,
    val direction: Direction,
    val system: Boolean,
    val author: String?,
    val time: String,
    val body: String,
    val attachments: List<Attachment> = emptyList(),
)

data class Conversation(
    val id: String,
    val preview: String,
    val time: String,
    val closed: Boolean,
    val agentName: String?,
    val peer: Boolean,
    /** Row/header title — the other party for a peer chat, else the agent. */
    val title: String?,
    /** Peer thread subtitle, e.g. "Direct chat · trip 9021". */
    val subtitle: String?,
    /** external_user_id → display name, for resolving message authors in the thread. */
    val names: Map<String, String>,
)

/** A file uploaded and ready to attach to the next message. */
data class PendingAttachment(
    val objectKey: String,
    val disposition: String,
    val mimeType: String,
    val filename: String,
)

// SildState is the observable snapshot the UI collects (StateFlow). Mirrors the
// web WidgetState so the Compose screens render the same information.
data class SildState(
    val ready: Boolean = false,
    val error: String? = null,
    val connection: ConnectionState = ConnectionState.IDLE,
    val conversations: List<Conversation> = emptyList(),
    val activeId: String? = null,
    val messages: List<Message> = emptyList(),
    val loadingThread: Boolean = false,
    val soundOn: Boolean = true,
    /** The support agent's display name learned from incoming messages. */
    val agentName: String? = null,
    val brand: BrandConfig = BrandConfig(),
    val brandName: String = "",
)
