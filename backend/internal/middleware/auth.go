package middleware

import (
	"context"
	"strings"
	"time"

	"github.com/bitllow/sild/backend/internal/auth"
	"github.com/bitllow/sild/backend/internal/httpx"
	"github.com/bitllow/sild/backend/internal/principal"
	"github.com/bitllow/sild/backend/internal/store"
	"github.com/bitllow/sild/backend/internal/store/models"
	"github.com/gin-gonic/gin"
)

// AdminCookieName is the admin session cookie.
const AdminCookieName = "sild_admin"

// Auth bundles the middleware constructors with their dependencies. dig provides
// it; route groups call its methods.
type Auth struct {
	store store.Store
	keys  *auth.KeyManager
}

// NewAuth constructs the middleware bundle.
func NewAuth(st store.Store, km *auth.KeyManager) *Auth {
	return &Auth{store: st, keys: km}
}

// ── Resolvers (return a principal or an error; no HTTP side effects) ─────────

type bearerKind int

const (
	bearerJWT bearerKind = iota
	bearerAPIKey
	bearerEmbedded
)

// kindOf classifies a bearer by its prefix; anything unprefixed is a user JWT.
func kindOf(raw string) bearerKind {
	switch {
	case strings.HasPrefix(raw, auth.APIKeyPrefix):
		return bearerAPIKey
	case strings.HasPrefix(raw, auth.EmbedSessionPrefix):
		return bearerEmbedded
	}
	return bearerJWT
}

// invalid is the 401 wording for a bearer of this kind that did not resolve.
func (k bearerKind) invalid() string {
	switch k {
	case bearerAPIKey:
		return "invalid API key"
	case bearerEmbedded:
		return "embedded session expired"
	}
	return "invalid token"
}

// resolveBearer resolves a bearer as whichever credential its prefix names.
func (a *Auth) resolveBearer(ctx context.Context, raw string) (*principal.Principal, bool) {
	switch kindOf(raw) {
	case bearerAPIKey:
		return a.resolveAPIKey(ctx, raw)
	case bearerEmbedded:
		return a.resolveEmbedded(ctx, raw)
	}
	return a.resolveJWT(ctx, raw)
}

func (a *Auth) resolveAPIKey(ctx context.Context, raw string) (*principal.Principal, bool) {
	prefix, secret, err := auth.ParseAPIKey(raw)
	if err != nil {
		return nil, false
	}
	key, err := a.store.APIKeys().FindByPrefix(ctx, prefix)
	if err != nil || !key.Active() || !auth.VerifySecret(secret, key.Hash) {
		return nil, false
	}
	if key.ActorID == nil {
		return &principal.Principal{TenantID: key.TenantID, Kind: principal.KindAPIKey}, true
	}
	return a.adminPrincipal(ctx, key.TenantID, *key.ActorID, func(admin *models.AdminUser) bool {
		return admin.Kind == models.AdminService
	})
}

func (a *Auth) resolveJWT(ctx context.Context, tok string) (*principal.Principal, bool) {
	claims, err := a.keys.Verify(ctx, tok)
	if err != nil {
		return nil, false
	}
	return &principal.Principal{TenantID: claims.Tid, Kind: principal.KindUser, Subject: claims.Subject}, true
}

func (a *Auth) resolveAdmin(ctx context.Context, rawCookie string) (*principal.Principal, bool) {
	sess, err := a.store.Admins().GetSession(ctx, auth.HashSessionToken(rawCookie))
	if err != nil || sess.ExpiresAt.Before(time.Now()) {
		return nil, false
	}
	// Sessions minted before embedded_only was turned on, or before the link, end with it.
	return a.adminPrincipal(ctx, sess.TenantID, sess.AdminUserID, func(admin *models.AdminUser) bool {
		if admin.ExternalSub == nil {
			return true
		}
		p, err := a.store.Identity().GetProvider(ctx, admin.TenantID)
		return err != nil || !p.EmbeddedOnly
	})
}

// resolveEmbedded resolves an embedded-inbox bearer. The member must still be
// linked to the identity the session was minted for.
func (a *Auth) resolveEmbedded(ctx context.Context, raw string) (*principal.Principal, bool) {
	sess, err := a.store.Identity().GetSession(ctx, auth.HashSessionToken(raw))
	if err != nil || sess.ExpiresAt.Before(time.Now()) {
		return nil, false
	}
	return a.adminPrincipal(ctx, sess.TenantID, sess.AdminUserID, func(admin *models.AdminUser) bool {
		return admin.ExternalSub != nil && *admin.ExternalSub == sess.ExternalSub
	})
}

// adminPrincipal builds a member's principal from their role assignments, once
// accept (nil accepts anyone) has passed the member.
func (a *Auth) adminPrincipal(ctx context.Context, tenantID, adminID string, accept func(*models.AdminUser) bool) (*principal.Principal, bool) {
	admin, err := a.store.Admins().Get(ctx, tenantID, adminID)
	if err != nil || (accept != nil && !accept(admin)) {
		return nil, false
	}
	roles, err := a.store.RoleAssignments().ListByAdmin(ctx, admin.TenantID, admin.ID)
	if err != nil {
		return nil, false
	}
	return principal.ForAdmin(admin, roles), true
}

// ── Middleware (enforce a specific credential type) ─────────────────────────

// APIKey requires a server↔server API key (§2.1).
func (a *Auth) APIKey() gin.HandlerFunc {
	return func(c *gin.Context) {
		raw := Bearer(c)
		if kindOf(raw) != bearerAPIKey {
			httpx.Unauthorized(c, "API key required")
			return
		}
		p, ok := a.resolveAPIKey(c.Request.Context(), raw)
		if !ok {
			httpx.Unauthorized(c, "invalid API key")
			return
		}
		// A service account's key carries that account's roles, not server reach.
		if p.Kind != principal.KindAPIKey {
			httpx.Unauthorized(c, "a service account key cannot use server-to-server routes")
			return
		}
		setPrincipal(c, p)
		c.Next()
	}
}

// UserJWT requires a user JWT (header bearer or ?token= for the websocket, §2.2).
func (a *Auth) UserJWT() gin.HandlerFunc {
	return func(c *gin.Context) {
		tok := Bearer(c)
		if tok == "" {
			tok = c.Query("token")
		}
		p, ok := a.resolveJWT(c.Request.Context(), tok)
		if !ok {
			httpx.Unauthorized(c, "invalid token")
			return
		}
		setPrincipal(c, p)
		c.Next()
	}
}

// Admin requires an inbox session (§2.4): the cookie, an embedded-inbox bearer,
// or a service account's key.
func (a *Auth) Admin() gin.HandlerFunc {
	return func(c *gin.Context) {
		if raw := Bearer(c); raw != "" && kindOf(raw) != bearerJWT {
			p, ok := a.resolveBearer(c.Request.Context(), raw)
			if !ok || p.Kind != principal.KindAdmin {
				msg := "admin session or service account key required"
				if k := kindOf(raw); k == bearerEmbedded {
					msg = k.invalid()
				}
				httpx.Unauthorized(c, msg)
				return
			}
			setPrincipal(c, p)
			c.Next()
			return
		}
		raw, err := c.Cookie(AdminCookieName)
		if err != nil || raw == "" {
			httpx.Unauthorized(c, "admin session required")
			return
		}
		p, ok := a.resolveAdmin(c.Request.Context(), raw)
		if !ok {
			httpx.Unauthorized(c, "session expired")
			return
		}
		setPrincipal(c, p)
		c.Next()
	}
}

// Any accepts whichever credential is present (API key, user JWT, embedded
// bearer, or admin session). Used by the spec's shared paths (GET conversation,
// messages, uploads); handlers authorize by principal kind + membership.
func (a *Auth) Any() gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx := c.Request.Context()
		if raw := Bearer(c); raw != "" {
			if p, ok := a.resolveBearer(ctx, raw); ok {
				setPrincipal(c, p)
				c.Next()
				return
			}
		}
		if tok := c.Query("token"); tok != "" {
			if p, ok := a.resolveJWT(ctx, tok); ok {
				setPrincipal(c, p)
				c.Next()
				return
			}
		}
		if raw, err := c.Cookie(AdminCookieName); err == nil && raw != "" {
			if p, ok := a.resolveAdmin(ctx, raw); ok {
				setPrincipal(c, p)
				c.Next()
				return
			}
		}
		httpx.Unauthorized(c, "authentication required")
	}
}

// Bearer returns the request's bearer token, "" when there is none.
func Bearer(c *gin.Context) string {
	if v, ok := strings.CutPrefix(c.GetHeader("Authorization"), "Bearer "); ok {
		return strings.TrimSpace(v)
	}
	return ""
}

// OptionalAuth resolves a credential when one is present and leaves the request
// unauthenticated when none is. It backs GET /v1/brands/active, which is public
// when keyed by app_id and tenant-scoped when authenticated.
//
// A credential that IS present but invalid or expired is a 401 — never a silent
// downgrade to the public form, which would turn an expired session into a
// quiet tenant switch.
func (a *Auth) OptionalAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx := c.Request.Context()
		if raw := Bearer(c); raw != "" {
			p, ok := a.resolveBearer(ctx, raw)
			if !ok {
				httpx.Unauthorized(c, kindOf(raw).invalid())
				return
			}
			setPrincipal(c, p)
			c.Next()
			return
		}
		if raw, err := c.Cookie(AdminCookieName); err == nil && raw != "" {
			p, ok := a.resolveAdmin(ctx, raw)
			if !ok {
				httpx.Unauthorized(c, "session expired")
				return
			}
			setPrincipal(c, p)
			c.Next()
			return
		}
		c.Next() // genuinely anonymous
	}
}
