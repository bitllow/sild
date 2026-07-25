// Package middleware holds gin middleware that resolves the request Principal
// and enforces RBAC. Tenant is resolved here — once, server-side — and never
// read from a path, header, or body (§1).
//
// The Principal type itself lives in internal/principal, so policy and store can
// read it without importing this package — which imports store to resolve
// credentials, and would otherwise close an import cycle.
package middleware

import (
	"github.com/bitllow/sild/backend/internal/principal"
	"github.com/gin-gonic/gin"
)

const principalKey = "sild.principal"

func setPrincipal(c *gin.Context, p *principal.Principal) { c.Set(principalKey, p) }

// Get returns the request principal, or nil if unauthenticated.
func Get(c *gin.Context) *principal.Principal {
	if v, ok := c.Get(principalKey); ok {
		if p, ok := v.(*principal.Principal); ok {
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
