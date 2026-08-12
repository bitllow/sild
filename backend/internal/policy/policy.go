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
	// IsMember reports whether the user principal is an active or archived member.
	IsMember bool
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
		// A key minted with a translation scope is a build token and nothing else: a
		// credential advertised as held to one project must not also read the
		// tenant's conversations. An unscoped key is the tenant's own backend
		// credential and keeps every capability it had.
		if p.IsBuildToken() {
			return g.apiKey && IsTranslationAction(a)
		}
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
