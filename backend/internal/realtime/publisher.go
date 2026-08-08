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
	EventContactUpdated     = "contact.updated"
)

// Channel naming (§5.1). Clients never choose channels; subscriptions are
// derived from membership server-side (§5.2).
func UserChannel(userID string) string { return "user:" + userID }
func ConvChannel(convID string) string { return "conv:" + convID }

// AgentsChannel carries every event in every support conversation an assignment
// makes readable, plus queue changes, to every connected agent (inbox) — the
// whole queue without a subscription per conversation (§5.1). Only agent
// connections are ever subscribed here, never end-user clients.
func AgentsChannel(tenantID string) string { return "agents:" + tenantID }

// PeerChannel carries every peer-conversation event (new-conversation nudges and
// message.created / member.added / conversation.closed for peer conversations) to
// operators who observe the peer surface. ONLY peer_access operators are ever
// subscribed here (gated in agentSubscriptions + reconcilePeerSubscriptions), so
// peer content reaching a non-peer operator is impossible as a subscription fact.
// A single tenant channel — rather than one subscription per peer conversation —
// means a peer conversation created after an operator connects is observed with
// no per-connection bookkeeping (§5.1).
func PeerChannel(tenantID string) string { return "peer:" + tenantID }

// Envelope is the wire format pushed to clients (§5.3).
type Envelope struct {
	Type           string `json:"type"`
	ConversationID string `json:"conversation_id,omitempty"`
	Data           any    `json:"data"`
	Ts             int64  `json:"ts"`
}

// Target selects which channels an envelope fans out to.
type Target struct {
	Conversation string   // conv:<id>
	Internal     bool     // agents-only: suppresses every client channel (§5.6)
	Users        []string // also push to user:<id> channels
	Tenant       string   // also push to agents:<tenant> (queue-wide agent fan-out)
	Peer         string   // also push to peer:<tenant> (peer-access operator fan-out)
}

// Publisher pushes envelopes to the broker. REST handlers call it AFTER the
// Postgres commit; delivery is best-effort (reconnect catch-up is the
// correctness mechanism, §5.4).
type Publisher interface {
	Publish(ctx context.Context, t Target, env Envelope) error
}

// Subscriber adjusts a connected user's server-side subscriptions live, so an
// access change (e.g. peer_access granted/revoked) takes effect without waiting
// for the connection to reconnect — subscriptions are otherwise only derived at
// connect time (agentSubscriptions). Implemented by the Centrifuge publisher;
// absent in tests/workers, so callers type-assert and no-op when unavailable.
type Subscriber interface {
	Subscribe(userID, channel string) error
	Unsubscribe(userID, channel string) error
}

// NoopPublisher drops events — used where realtime is irrelevant (some workers).
type NoopPublisher struct{}

func (NoopPublisher) Publish(context.Context, Target, Envelope) error { return nil }
