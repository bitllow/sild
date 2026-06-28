// Package middleware holds gin middleware that resolves the request Principal
// and enforces RBAC. Tenant is resolved here — once, server-side — and never
// read from a path, header, or body (§1).
package middleware

import (
	"github.com/bitllow/sild/backend/internal/store/models"
	"github.com/gin-gonic/gin"
)

// PrincipalKind identifies how the caller authenticated.
type PrincipalKind string

const (
	KindAPIKey PrincipalKind = "apikey"
	KindUser   PrincipalKind = "user"
	KindAdmin  PrincipalKind = "admin"
)

// Principal is the authenticated caller. TenantID is always the verified tenant
// (key binding / tid claim / admin session) — the single scope for every query.
type Principal struct {
	TenantID string
	Kind     PrincipalKind

	// user (JWT)
	Subject string

	// admin (session)
	AdminID string
	Role    models.PlatformRole
}

const principalKey = "sild.principal"

func setPrincipal(c *gin.Context, p *Principal) { c.Set(principalKey, p) }

// Get returns the request principal, or nil if unauthenticated.
func Get(c *gin.Context) *Principal {
	if v, ok := c.Get(principalKey); ok {
		if p, ok := v.(*Principal); ok {
			return p
		}
	}
	return nil
}

// TenantID returns the resolved tenant for the request ("" if none).
func TenantID(c *gin.Context) string {
	if p := Get(c); p != nil {
		return p.TenantID
	}
	return ""
}
