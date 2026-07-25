// Package principal holds the authenticated caller, in a package that depends on
// nothing but models. middleware constructs it; policy and store read it. Keeping
// the type here is what stops store → policy → middleware → store from closing,
// since middleware imports store to resolve credentials.
package principal

import "github.com/bitllow/sild/backend/internal/store/models"

// Kind identifies how the caller authenticated.
type Kind string

const (
	KindAPIKey Kind = "apikey"
	KindUser   Kind = "user"
	KindAdmin  Kind = "admin"
	// KindSigned is a signed-URL capability: one object key, one verb, an expiry.
	// It authorizes the local upload routes, which carry no credential (§11).
	KindSigned Kind = "signed"
)

// Principal is the authenticated caller. TenantID is always the verified tenant
// (key binding / tid claim / admin session / signature) — the single scope for
// every query (§1).
type Principal struct {
	TenantID string
	Kind     Kind

	// user (JWT)
	Subject string

	// admin (session)
	AdminID    string
	Role       models.PlatformRole
	PeerAccess bool

	// signed (upload capability)
	ObjectKey string
}

// Privileged reports whether an admin holds the owner/admin platform role.
func (p *Principal) Privileged() bool {
	return p != nil && p.Kind == KindAdmin &&
		(p.Role == models.PlatformOwner || p.Role == models.PlatformAdmin)
}
