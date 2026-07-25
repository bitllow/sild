package policy

import (
	"errors"

	"github.com/bitllow/sild/backend/internal/principal"
	"github.com/bitllow/sild/backend/internal/store/models"
)

// Denial reasons. Callers map these to HTTP: ErrUnauthenticated → 401, the rest
// → 403. Reason carries the client-facing message so the wording lives with the
// rule that produced it.
var (
	ErrUnauthenticated = errors.New("authentication required")
	ErrForbidden       = errors.New("forbidden")
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

// ResourceAttrs are the facts about a specific resource a decision depends on.
// The caller loads them once; policy does no I/O.
type ResourceAttrs struct {
	Kind models.ConversationKind
	// SupportReachable reports that an agent may reach this support conversation:
	// it carries an assignment — ANYONE'S, or nobody's, never "assigned to the
	// caller" — or it is an archived formerly-support conversation.
	SupportReachable bool
	// IsMember reports whether the user principal is an active or archived member.
	IsMember bool
}

// Authorize decides a single action against a known resource. Actions with no
// resource dimension pass a zero ResourceAttrs.
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

// holds reports whether the principal's kind and role carry the action at all,
// before any resource attributes are considered.
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
		if g.adminPriv {
			return p.Privileged()
		}
		return g.admin
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

// authorizeConversation applies the per-conversation rules (§4.2, §7):
//   - API key:     tenant-wide
//   - owner/admin: tenant-wide for non-peer; peer still needs their own peer_access
//   - agent:       support conversations carrying an assignment, plus archived
//     formerly-support; peer only with peer_access
//   - user:        must be an (active or archived) member
func authorizeConversation(p *principal.Principal, r ResourceAttrs) error {
	switch p.Kind {
	case principal.KindAPIKey:
		return nil

	case principal.KindAdmin:
		// Peer takes precedence over role: peer access is the operator's own
		// opt-in whatever their platform role, archived peer included.
		if r.Kind == models.KindPeer {
			if p.PeerAccess {
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
