package io.sild.ui

import android.app.NotificationChannel
import android.app.NotificationManager
import android.app.PendingIntent
import android.content.Context
import android.os.Build
import androidx.core.app.NotificationCompat
import io.sild.core.SildPush
import io.sild.core.SildPushMessage

/**
 * Optional renderer for Sild's push nudges (§5.5).
 *
 * FCM hands a foregrounded app the message instead of displaying it, so a host
 * that wants the system's look posts it here. Rendering it yourself is equally
 * supported — nothing else in the SDK goes through this.
 *
 * On Android 13+ the host owns the POST_NOTIFICATIONS permission; without it
 * posting silently does nothing.
 */
object SildNotifications {

    /**
     * Creates the channel the server payload names. Idempotent, and safe to call
     * on every delivery — Android 8+ drops a notification whose channel is absent.
     */
    fun ensureChannel(context: Context) {
        ensureChannel(context.getSystemService(NotificationManager::class.java) ?: return)
    }

    /**
     * Posts a nudge on Sild's channel, tagged by conversation so an active thread
     * updates one notification rather than stacking — the same collapse the server
     * asks for.
     *
     * [title] and [body] are the ones the message arrived with
     * (`RemoteMessage.notification`); the server composes them per the tenant's
     * privacy settings, so neither is rewritten here. [contentIntent] is what a tap
     * opens — build it against `SildMessenger.openConversation(context, id)`, reading
     * the id off [message].
     */
    fun show(
        context: Context,
        message: SildPushMessage,
        title: String,
        body: String? = null,
        smallIcon: Int,
        contentIntent: PendingIntent,
    ) {
        val manager = context.getSystemService(NotificationManager::class.java) ?: return
        ensureChannel(manager)
        val builder = NotificationCompat.Builder(context, SildPush.NOTIFICATION_CHANNEL)
            .setSmallIcon(smallIcon)
            .setContentTitle(title)
            .setContentIntent(contentIntent)
            .setCategory(NotificationCompat.CATEGORY_MESSAGE)
            .setPriority(NotificationCompat.PRIORITY_HIGH)
            .setAutoCancel(true)
        if (!body.isNullOrEmpty()) builder.setContentText(body)
        if (message.unreadCount > 1) builder.setNumber(message.unreadCount)
        manager.notify(message.conversationId, NOTIFICATION_ID, builder.build())
    }

    private fun ensureChannel(manager: NotificationManager) {
        if (Build.VERSION.SDK_INT < Build.VERSION_CODES.O) return
        manager.createNotificationChannel(
            NotificationChannel(SildPush.NOTIFICATION_CHANNEL, "Messages", NotificationManager.IMPORTANCE_HIGH),
        )
    }

    /** The tag is the conversation, so one id suffices for every nudge. */
    private const val NOTIFICATION_ID = 1
}
