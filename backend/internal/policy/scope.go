package policy

import (
	"slices"

	"github.com/bitllow/sild/backend/internal/principal"
	"github.com/bitllow/sild/backend/internal/store/models"
)

// ResourceScope is the ceiling on what a principal may see in a collection. Its
// fields are unexported and only Scope constructs it, so a repository query that
// skipped authorization does not compile. Inspection is exported — a repo has to
// read it to build a WHERE clause — but the getters hand back copies, so a scope
// can be read and never widened.
type ResourceScope struct {
	kinds              []models.ConversationKind
	participant        *string
	requiresAssignment bool
	// denyAll is set when the principal holds no capability at all. It is a
	// distinct flag rather than an empty kind list, because an empty list means
	// "unrestricted" — the two must never be confused in a query builder.
	denyAll bool
}

// DenyAll reports that nothing is in scope. A query builder must short-circuit
// to an empty result rather than treating an unrestricted-looking scope as open.
func (s ResourceScope) DenyAll() bool { return s.denyAll }

// AllowedKinds returns the conversation kinds in scope. Empty means unrestricted
// — check DenyAll first.
func (s ResourceScope) AllowedKinds() []models.ConversationKind {
	return slices.Clone(s.kinds)
}

// Participant returns the external user id the scope is pinned to, if any. Set
// for user JWTs, which may only ever see their own conversations.
func (s ResourceScope) Participant() (string, bool) {
	if s.participant == nil {
		return "", false
	}
	return *s.participant, true
}

// RequiresAssignment reports whether support conversations must carry an
// assignment to be in scope. It means "an assignment exists", anyone's or
// nobody's — never "assigned to the caller", which is a client filter and lives
// on the query instead.
func (s ResourceScope) RequiresAssignment() bool { return s.requiresAssignment }

// AllowsKind reports whether a kind survives the scope.
func (s ResourceScope) AllowsKind(k models.ConversationKind) bool {
	if s.denyAll {
		return false
	}
	if len(s.kinds) == 0 {
		return true
	}
	return slices.Contains(s.kinds, k)
}

// Scope produces the ceiling for a collection query. A client filter may narrow
// what this returns, never widen it.
func Scope(p *principal.Principal, a Action) ResourceScope {
	if !holds(p, a) {
		return ResourceScope{denyAll: true}
	}
	switch p.Kind {
	case principal.KindAPIKey:
		return ResourceScope{}

	case principal.KindUser:
		sub := p.Subject
		return ResourceScope{participant: &sub}

	case principal.KindAdmin:
		s := ResourceScope{}
		if !p.PeerAccess {
			s.kinds = []models.ConversationKind{models.KindSupport}
		}
		// Agents see support conversations that carry an assignment; owner/admin
		// are tenant-wide across everything that isn't peer.
		if !p.Privileged() {
			s.requiresAssignment = true
		}
		return s
	}
	return ResourceScope{denyAll: true}
}

// Grant is one action a principal holds, with the scope it holds it over. It is
// what GET /v1/principal returns, so frontends render affordances from the same
// decisions the backend enforces instead of re-deriving them from role flags.
type Grant struct {
	Action Action      `json:"action"`
	Scope  *GrantScope `json:"scope,omitempty"`
}

// GrantScope is the wire projection of a ResourceScope: an explicit allowlist,
// so a new internal scope attribute is private until it is deliberately exposed.
// ResourceScope itself cannot be marshalled — its fields are unexported.
type GrantScope struct {
	Kinds              []models.ConversationKind `json:"kinds,omitempty"`
	RequiresAssignment bool                      `json:"requires_assignment,omitempty"`
	Participant        string                    `json:"participant,omitempty"`
}

// Grants lists every action the principal holds, each with its scope where the
// action is collection-shaped.
func Grants(p *principal.Principal) []Grant {
	out := make([]Grant, 0, len(capabilities))
	for _, a := range sortedActions() {
		if !holds(p, a) {
			continue
		}
		g := Grant{Action: a}
		if a == ConversationsList || a == ContactsList {
			g.Scope = scopeDTO(Scope(p, a))
		}
		out = append(out, g)
	}
	return out
}

func scopeDTO(s ResourceScope) *GrantScope {
	dto := &GrantScope{
		Kinds:              s.AllowedKinds(),
		RequiresAssignment: s.RequiresAssignment(),
	}
	if pt, ok := s.Participant(); ok {
		dto.Participant = pt
	}
	return dto
}
