package policy

import (
	"github.com/bitllow/sild/backend/internal/principal"
	"github.com/bitllow/sild/backend/internal/store/models"
)

// Denied is a policy refusal with a stable code and a human message.
type Denied struct {
	Code    string
	Message string
	authn   bool // true → 401 rather than 403
}

func (d *Denied) Error() string         { return d.Message }
func (d *Denied) Unauthenticated() bool { return d.authn }

func denied(code, msg string) error { return &Denied{Code: code, Message: msg} }
func unauthenticated() error {
	return &Denied{Code: "unauthorized", Message: "authentication required", authn: true}
}

// ResourceAttrs are the resource facts a decision needs. policy does no I/O.
type ResourceAttrs struct {
	Kind models.ConversationKind
	// SupportReachable means an assignment exists (anyone's, not the caller's) or
	// the conversation is archived formerly-support.
	SupportReachable bool
	// IsMember reports whether the principal is an active or archived member: a
	// user by external id, a requester by admin id.
	IsMember bool
	// Queue is the conversation's queue; only non-peer conversations consult it.
	Queue string
}

// Authorize decides one action. Actions with no resource dimension pass a zero
// ResourceAttrs.
func Authorize(p *principal.Principal, a Action, r ResourceAttrs) error {
	if !holds(p, a) {
		if p == nil {
			return unauthenticated()
		}
		return denied("forbidden", "insufficient capability")
	}
	if !conversationScoped(a) {
		return nil
	}
	return authorizeConversation(p, r)
}

// Holds reports whether the principal carries an action at all, for a handler
// that probes a capability rather than authorizing its route.
func Holds(p *principal.Principal, a Action) bool { return holds(p, a) }

// holds reports whether the principal's kind and role carry the action at all.
func holds(p *principal.Principal, a Action) bool {
	g, ok := capabilities[a]
	if !ok {
		return false // undeclared action: deny, never default-allow
	}
	if p == nil {
		return false
	}
	switch p.Kind {
	case principal.KindAPIKey:
		return g.apiKey
	case principal.KindUser:
		return g.user
	case principal.KindSigned:
		return g.signed
	case principal.KindAdmin:
		// Union across the member's roles: a second role only ever widens.
		for _, a := range p.Assignments {
			if roleHolds(g, a.Role) {
				return true
			}
		}
		return false
	}
	return false
}

func conversationScoped(a Action) bool {
	switch a {
	case ConversationsRead, ConversationsClose, MessagesRead, MessagesSend,
		ReceiptsWrite, TypingWrite, AssignmentsCreate, AssignmentsClaim, AssignmentsClose:
		return true
	}
	return false
}

// authorizeConversation applies the per-conversation rules (§4.2, §7).
func authorizeConversation(p *principal.Principal, r ResourceAttrs) error {
	switch p.Kind {
	case principal.KindAPIKey:
		return nil

	case principal.KindAdmin:
		// A requester reaches their own requests, whatever other role they hold.
		ownRequest := p.HasRole(models.PlatformRequester) && r.Kind != models.KindPeer && r.IsMember &&
			p.RequesterReaches(r.Queue)
		if ownRequest {
			return nil
		}
		if p.RequesterOnly() {
			if r.Kind == models.KindPeer || !r.IsMember {
				return denied("not_a_member", "not a member of this conversation")
			}
			return denied("queue_out_of_scope", "this conversation is in a queue you do not use")
		}
		// Peer takes precedence over every other role: it is the agent
		// assignment's own grant, and no other role implies it.
		if r.Kind == models.KindPeer {
			if p.PeerAccess() {
				return nil
			}
			return denied("peer_access_required", "peer access is not enabled for this operator")
		}
		if p.Privileged() {
			return nil
		}
		if !p.ReachesQueue(r.Queue) {
			return denied("queue_out_of_scope", "this conversation is in a queue you do not work")
		}
		if r.SupportReachable {
			return nil
		}
		return denied("support_only", "agents may only access support conversations")

	case principal.KindUser:
		if r.IsMember {
			return nil
		}
		return denied("not_a_member", "not a member of this conversation")
	}
	return denied("forbidden", "insufficient capability")
}
