package io.sild.core

/**
 * Push notifications (§5.5).
 *
 * The SDK does NOT integrate Firebase. The host app owns its push registration —
 * on Android only one FirebaseMessagingService wins per app, so a second one
 * inside a library silently never fires — and forwards two things here: the
 * token it was issued, and any message that turns out to be ours.
 *
 * See docs/adr/0001-host-app-owns-fcm-sdk-takes-the-token.md.
 */

/** A nudge Sild sent, decoded from the host's push message data. */
data class SildPushMessage(
    val conversationId: String,
    /** "support" or "peer" — enough to route a tap without a round-trip. */
    val conversationKind: String,
    val messageId: String,
    val unreadCount: Int,
)

object SildPush {
    /** Marks our payloads so a host app can tell them from its own. */
    const val KEY = "sild"

    /** The Android notification channel our payloads name. The host creates it. */
    const val NOTIFICATION_CHANNEL = "sild_messages"

    /**
     * Reports whether a push message came from Sild. The host calls this in its
     * own message handler before doing anything else with the payload.
     */
    fun isSildPush(data: Map<String, String>): Boolean = data[KEY] == "1"

    /** Decodes a nudge, or null when the payload is not ours or is malformed. */
    fun parse(data: Map<String, String>): SildPushMessage? {
        if (!isSildPush(data)) return null
        val conversationId = data["conversation_id"].orEmpty()
        if (conversationId.isEmpty()) return null
        return SildPushMessage(
            conversationId = conversationId,
            conversationKind = data["conversation_kind"].orEmpty(),
            messageId = data["message_id"].orEmpty(),
            unreadCount = data["unread_count"]?.toIntOrNull() ?: 0,
        )
    }
}
