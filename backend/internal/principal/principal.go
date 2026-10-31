// Package principal holds the authenticated caller. It lives apart from
// middleware so store → policy → middleware → store cannot close as a cycle.
package principal

import (
	"slices"
	"time"

	"github.com/bitllow/sild/backend/internal/store/models"
)

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

	// admin (session, or a service account's key)
	AdminID string
	// Service marks a service account: it authenticates by key and posts as a bot.
	Service bool
	// Assignments is every role this member holds, each with its scope. A
	// capability is held when any of them carries it; nothing subtracts.
	Assignments []Assignment

	// signed (upload capability)
	ObjectKey string

	// Expires is when the credential ends, for anything minted from it; zero when
	// nothing minted from it can outlive it.
	Expires time.Time
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
	p := &Principal{TenantID: admin.TenantID, Kind: KindAdmin, AdminID: admin.ID, Service: admin.Kind == models.AdminService}
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

// Queues returns the queues an operator reaches, and all=true when every queue
// is reachable. An agent assignment without a queues dimension reaches support.
func (p *Principal) Queues() (queues []string, all bool) {
	if p.Privileged() {
		return nil, true
	}
	s, ok := p.ScopeOf(models.PlatformAgent)
	if !ok {
		return nil, false
	}
	if len(s.Queues) == 0 {
		return []string{models.QueueSupport}, false
	}
	if slices.Contains(s.Queues, models.ScopeAll) {
		return nil, true
	}
	return slices.Clone(s.Queues), false
}

// ReachesQueue reports whether an operator may reach a queue's conversations.
func (p *Principal) ReachesQueue(queue string) bool {
	qs, all := p.Queues()
	return all || slices.Contains(qs, queue)
}

// Operator reports whether the member works conversations (owner, admin or agent).
func (p *Principal) Operator() bool {
	return p.Privileged() || p.HasRole(models.PlatformAgent)
}

// RequesterOnly reports a member whose only conversation access is their own
// requests: every conversation decision then pins to membership.
func (p *Principal) RequesterOnly() bool {
	return !p.Operator() && p.HasRole(models.PlatformRequester)
}

// RequesterQueues returns the queues a requester may open conversations in,
// and all=true when every queue is open to them.
func (p *Principal) RequesterQueues() (queues []string, all bool) {
	// Owners and admins open their own requests anywhere a requester could.
	if p.Privileged() {
		return nil, true
	}
	s, ok := p.ScopeOf(models.PlatformRequester)
	if !ok {
		return nil, false
	}
	if slices.Contains(s.Queues, models.ScopeAll) {
		return nil, true
	}
	return slices.Clone(s.Queues), false
}

// RequesterReaches reports whether a requester may use a queue.
func (p *Principal) RequesterReaches(queue string) bool {
	qs, all := p.RequesterQueues()
	// A privileged member opens support conversations for an end user, never for themselves.
	if p.Privileged() && queue == models.QueueSupport {
		return false
	}
	return all || slices.Contains(qs, queue)
}

// Privileged reports whether an admin holds the owner or admin role.
func (p *Principal) Privileged() bool {
	return p.HasRole(models.PlatformOwner) || p.HasRole(models.PlatformAdmin)
}
