// Package principal holds the authenticated caller. It lives apart from
// middleware so store → policy → middleware → store cannot close as a cycle.
package principal

import "github.com/bitllow/sild/backend/internal/store/models"

// Kind identifies how the caller authenticated.
type Kind string

const (
	KindAPIKey Kind = "apikey"
	KindUser   Kind = "user"
	KindAdmin  Kind = "admin"
	// KindSigned is a signed-URL capability: one object key, one verb, an expiry.
	KindSigned Kind = "signed"
)

// Principal is the authenticated caller. TenantID is always the verified tenant
// — the single scope for every query (§1).
type Principal struct {
	TenantID string
	Kind     Kind

	// user (JWT)
	Subject string

	// admin (session)
	AdminID string
	// Assignments is every role this member holds, each with its scope. A
	// capability is held when any of them carries it; nothing subtracts.
	Assignments []Assignment

	// signed (upload capability)
	ObjectKey string
}

// Assignment is one role the member holds, with the scope that role defines.
type Assignment struct {
	Role  models.PlatformRole
	Scope models.RoleScope
}

// Held builds assignments for roles that carry no scope, for a caller that only
// states which roles someone holds.
func Held(roles ...models.PlatformRole) []Assignment {
	out := make([]Assignment, 0, len(roles))
	for _, r := range roles {
		out = append(out, Assignment{Role: r})
	}
	return out
}

// ForAdmin builds the principal of a signed-in operator from their assignment
// rows. A member with no assignment holds nothing — there is no implied role.
func ForAdmin(admin *models.AdminUser, rows []models.RoleAssignment) *Principal {
	p := &Principal{TenantID: admin.TenantID, Kind: KindAdmin, AdminID: admin.ID}
	for _, r := range rows {
		p.Assignments = append(p.Assignments, Assignment{Role: r.Role, Scope: r.Scope})
	}
	return p
}

// Roles lists the roles this principal holds.
func (p *Principal) Roles() []models.PlatformRole {
	if p == nil {
		return nil
	}
	out := make([]models.PlatformRole, 0, len(p.Assignments))
	for _, a := range p.Assignments {
		out = append(out, a.Role)
	}
	return out
}

// ScopeOf returns the scope of one role, and whether it is held at all.
func (p *Principal) ScopeOf(r models.PlatformRole) (models.RoleScope, bool) {
	if p == nil || p.Kind != KindAdmin {
		return models.RoleScope{}, false
	}
	for _, a := range p.Assignments {
		if a.Role == r {
			return a.Scope, true
		}
	}
	return models.RoleScope{}, false
}

// HasRole reports whether the member holds a role.
func (p *Principal) HasRole(r models.PlatformRole) bool {
	_, ok := p.ScopeOf(r)
	return ok
}

// PeerAccess reports whether peer conversations are reachable — the agent role's
// own dimension. No other role grants it: reading a private chat is an explicit
// grant or nothing.
func (p *Principal) PeerAccess() bool {
	s, ok := p.ScopeOf(models.PlatformAgent)
	return ok && s.Peer
}

// Privileged reports whether an admin holds the owner or admin role.
func (p *Principal) Privileged() bool {
	return p.HasRole(models.PlatformOwner) || p.HasRole(models.PlatformAdmin)
}
