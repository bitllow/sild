// Package push handles offline delivery (§5.5): fan-out only to members with no
// live connection (Centrifuge presence), via FCM (Android/web) + APNs (iOS).
package push

import "context"

// Payload is the nudge sent to a device (§5.5).
type Payload struct {
	ConversationID string `json:"conversation_id"`
	MessageID      string `json:"message_id"`
	Preview        string `json:"preview,omitempty"`
	UnreadCount    int    `json:"unread_count"`
}

// Target is a device token to notify.
type Target struct {
	Platform string
	Token    string
}

// Notifier delivers a payload to device tokens. FCM/APNs implement it.
type Notifier interface {
	Notify(ctx context.Context, targets []Target, p Payload) error
}

// NoopNotifier drops notifications (default until FCM/APNs are configured).
type NoopNotifier struct{}

func (NoopNotifier) Notify(context.Context, []Target, Payload) error { return nil }

// PresenceChecker reports whether a user has a live connection. The real impl
// queries Centrifuge presence (Redis); sild-worker holds it. An api-side checker
// that always reports offline would double-notify, so fan-out lives in the
// worker (§5.5, §3a).
type PresenceChecker interface {
	Online(ctx context.Context, tenantID, userID string) (bool, error)
}

// AlwaysOffline is a PresenceChecker for environments without a broker (tests/
// single-node dev): every member is considered offline.
type AlwaysOffline struct{}

func (AlwaysOffline) Online(context.Context, string, string) (bool, error) { return false, nil }
