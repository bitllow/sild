// Package realtime is the egress-only realtime layer (§5). This file defines the
// Publisher interface the domain depends on; the Centrifuge implementation and
// the sild-ws node live alongside it (built in the realtime buildout).
package realtime

import "context"

// Event types (§5.3).
const (
	EventMessageCreated     = "message.created"
	EventMessageRead        = "message.read"
	EventMemberAdded        = "member.added"
	EventMemberRemoved      = "member.removed"
	EventAssignmentUpdated  = "assignment.updated"
	EventConversationClosed = "conversation.closed"
	EventTyping             = "typing"
)

// Channel naming (§5.1). Clients never choose channels; subscriptions are
// derived from membership server-side (§5.2).
func UserChannel(userID string) string         { return "user:" + userID }
func ConvChannel(convID string) string         { return "conv:" + convID }
func ConvInternalChannel(convID string) string { return "conv:" + convID + ":internal" }

// Envelope is the wire format pushed to clients (§5.3).
type Envelope struct {
	Type           string `json:"type"`
	ConversationID string `json:"conversation_id,omitempty"`
	Data           any    `json:"data"`
	Ts             int64  `json:"ts"`
}

// Target selects which channels an envelope fans out to.
type Target struct {
	Conversation string   // conv:<id> (or conv:<id>:internal when Internal)
	Internal     bool     // route to the agents-only internal channel (§5.6)
	Users        []string // also push to user:<id> channels
}

// Publisher pushes envelopes to the broker. REST handlers call it AFTER the
// Postgres commit; delivery is best-effort (reconnect catch-up is the
// correctness mechanism, §5.4).
type Publisher interface {
	Publish(ctx context.Context, t Target, env Envelope) error
}

// NoopPublisher drops events — used where realtime is irrelevant (some workers).
type NoopPublisher struct{}

func (NoopPublisher) Publish(context.Context, Target, Envelope) error { return nil }
