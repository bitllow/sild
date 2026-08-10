package policy

import (
	"slices"

	"github.com/bitllow/sild/backend/internal/principal"
	"github.com/bitllow/sild/backend/internal/store/models"
)

// ResourceScope is the ceiling on what a principal may see in a collection.
// Only Scope can construct one, so a query that skipped authorization does not
// compile; the getters return copies, so a scope can be read but never widened.
type ResourceScope struct {
	kinds              []models.ConversationKind
	participant        *string
	requiresAssignment bool
	// denyAll is a flag rather than an empty kinds list, because an empty list
	// means "unrestricted".
	denyAll bool
}

// DenyAll reports that nothing is in scope; callers must short-circuit.
func (s ResourceScope) DenyAll() bool { return s.denyAll }

// AllowedKinds returns the kinds in scope. Empty means unrestricted — check
// DenyAll first.
func (s ResourceScope) AllowedKinds() []models.ConversationKind {
	return slices.Clone(s.kinds)
}

// Participant returns the external user id the scope is pinned to (user JWTs).
func (s ResourceScope) Participant() (string, bool) {
	if s.participant == nil {
		return "", false
	}
	return *s.participant, true
}

// RequiresAssignment means an assignment must EXIST — anyone's, not the
// caller's. "Assigned to me" is a client filter and lives on the query.
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
		if !p.PeerAccess() {
			s.kinds = []models.ConversationKind{models.KindSupport}
		}
		// Agents need an assignment; owner/admin are tenant-wide.
		if !p.Privileged() {
			s.requiresAssignment = true
		}
		return s
	}
	return ResourceScope{denyAll: true}
}

// Grant is one action a principal holds, with the scope it holds it over, so a
// frontend renders affordances from the decisions the backend enforces.
type Grant struct {
	Action Action      `json:"action"`
	Scope  *GrantScope `json:"scope,omitempty"`
}

// GrantScope is the wire projection of a ResourceScope — an explicit allowlist,
// so a new scope attribute stays private until deliberately exposed.
type GrantScope struct {
	Kinds              []models.ConversationKind `json:"kinds,omitempty"`
	RequiresAssignment bool                      `json:"requires_assignment,omitempty"`
	Participant        string                    `json:"participant,omitempty"`
}

// Grants lists every action the principal holds, scoped where collection-shaped.
func Grants(p *principal.Principal) []Grant {
	out := make([]Grant, 0, len(capabilities))
	for _, a := range sortedActions() {
		if !holds(p, a) {
			continue
		}
		// A translator's scope is part of what they hold: advertising a publish
		// their grant withholds puts a button on screen that only 403s.
		if scope, narrows := TranslationNarrowing(p, a); narrows && a == TranslationsPublish && !scope.Publish {
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
