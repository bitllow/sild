package middleware

import (
	"context"
	"strings"
	"time"

	"github.com/bitllow/sild/backend/internal/auth"
	"github.com/bitllow/sild/backend/internal/httpx"
	"github.com/bitllow/sild/backend/internal/store"
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

func (a *Auth) resolveAPIKey(ctx context.Context, raw string) (*Principal, bool) {
	prefix, secret, err := auth.ParseAPIKey(raw)
	if err != nil {
		return nil, false
	}
	key, err := a.store.APIKeys().FindByPrefix(ctx, prefix)
	if err != nil || !key.Active() || !auth.VerifySecret(secret, key.Hash) {
		return nil, false
	}
	return &Principal{TenantID: key.TenantID, Kind: KindAPIKey}, true
}

func (a *Auth) resolveJWT(ctx context.Context, tok string) (*Principal, bool) {
	claims, err := a.keys.Verify(ctx, tok)
	if err != nil {
		return nil, false
	}
	return &Principal{TenantID: claims.Tid, Kind: KindUser, Subject: claims.Subject}, true
}

func (a *Auth) resolveAdmin(ctx context.Context, rawCookie string) (*Principal, bool) {
	sess, err := a.store.Admins().GetSession(ctx, auth.HashSessionToken(rawCookie))
	if err != nil || sess.ExpiresAt.Before(time.Now()) {
		return nil, false
	}
	admin, err := a.store.Admins().Get(ctx, sess.TenantID, sess.AdminUserID)
	if err != nil {
		return nil, false
	}
	return &Principal{TenantID: admin.TenantID, Kind: KindAdmin, AdminID: admin.ID, Role: admin.PlatformRole}, true
}

// ── Middleware (enforce a specific credential type) ─────────────────────────

// APIKey requires a server↔server API key (§2.1).
func (a *Auth) APIKey() gin.HandlerFunc {
	return func(c *gin.Context) {
		raw := bearer(c)
		if !strings.HasPrefix(raw, "sild_live_") {
			httpx.Unauthorized(c, "API key required")
			return
		}
		p, ok := a.resolveAPIKey(c.Request.Context(), raw)
		if !ok {
			httpx.Unauthorized(c, "invalid API key")
			return
		}
		setPrincipal(c, p)
		c.Next()
	}
}

// UserJWT requires a user JWT (header bearer or ?token= for the websocket, §2.2).
func (a *Auth) UserJWT() gin.HandlerFunc {
	return func(c *gin.Context) {
		tok := bearer(c)
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

// Admin requires an inbox session (§2.4).
func (a *Auth) Admin() gin.HandlerFunc {
	return func(c *gin.Context) {
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

// Any accepts whichever credential is present (API key, user JWT, or admin
// session). Used by the spec's shared paths (GET conversation, messages,
// uploads); handlers authorize by principal kind + membership.
func (a *Auth) Any() gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx := c.Request.Context()
		if raw := bearer(c); raw != "" {
			if strings.HasPrefix(raw, "sild_live_") {
				if p, ok := a.resolveAPIKey(ctx, raw); ok {
					setPrincipal(c, p)
					c.Next()
					return
				}
			} else if p, ok := a.resolveJWT(ctx, raw); ok {
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

func bearer(c *gin.Context) string {
	if v, ok := strings.CutPrefix(c.GetHeader("Authorization"), "Bearer "); ok {
		return strings.TrimSpace(v)
	}
	return ""
}
