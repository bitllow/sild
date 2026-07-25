// Package api wires the REST surface (§4). Routes are grouped by audience
// (integration/user/admin/public) at the file level; shared paths that accept
// multiple credential types use the Any() middleware and authorize by principal.
package api

import (
	"net/http"

	"github.com/bitllow/sild/backend/internal/auth"
	"github.com/bitllow/sild/backend/internal/config"
	"github.com/bitllow/sild/backend/internal/domain"
	"github.com/bitllow/sild/backend/internal/middleware"
	"github.com/bitllow/sild/backend/internal/storage"
	"github.com/bitllow/sild/backend/internal/store/models"
	"github.com/bitllow/sild/backend/internal/webasset"
	"github.com/gin-gonic/gin"
)

// Handler holds the dependencies for all REST handlers.
type Handler struct {
	svc    *domain.Service
	search *domain.SearchService
	mw     *middleware.Auth
	km     *auth.KeyManager
	authn  auth.AdminAuthenticator
	bucket storage.Bucket
	cfg    *config.Config
}

// New constructs the api Handler. dig provides it.
func New(svc *domain.Service, search *domain.SearchService, mw *middleware.Auth, km *auth.KeyManager, authn auth.AdminAuthenticator, bucket storage.Bucket, cfg *config.Config) *Handler {
	return &Handler{svc: svc, search: search, mw: mw, km: km, authn: authn, bucket: bucket, cfg: cfg}
}

// Mount attaches every route to the engine.
func (h *Handler) Mount(e *gin.Engine) {
	e.GET("/.well-known/jwks.json", h.jwks)

	// Web drop-in bundle (§9), embedded at build time so the binary is
	// self-contained. CORS is already applied engine-wide, so customer sites can
	// load it cross-origin; front it with a CDN in production.
	e.GET("/widget.js", func(c *gin.Context) {
		c.Header("Cache-Control", "public, max-age=300")
		c.Data(http.StatusOK, "application/javascript; charset=utf-8", webasset.Widget)
	})

	// Every JSON route is capped; the raw-body routes below carry their own,
	// larger limits. A cap inside the JSON decoder would miss them entirely.
	v1 := e.Group("/v1", middleware.BodyLimit(middleware.BodyLimitJSON))

	// Email inbound (§6.2): provider posts here, signature is the gate.
	v1.POST("/email/inbound", middleware.BodyLimit(middleware.BodyLimitEmail), h.mw.RateLimitIngress(), h.emailInbound)

	// Local storage backend serves attachment bytes here (§11). GCS/S3 use
	// direct-to-bucket signed URLs and don't mount these.
	if h.cfg.Storage.Backend == "local" || h.cfg.Storage.Backend == "" {
		v1.PUT("/uploads/local/*objectKey", middleware.BodyLimit(middleware.BodyLimitUpload), h.mw.RateLimitIngress(), h.localUploadPut)
		v1.GET("/uploads/local/*objectKey", h.localUploadGet)
	}

	// ── Resource routes ────────────────────────────────────────────────────
	//
	// Paths name the DATA MODEL, not the consumer. The credential decides which
	// subset of a resource a caller sees, never which URL they call — the inbox,
	// the web drop-in, the native SDK and a host backend all GET /v1/conversations.
	// The one deliberate exception is /v1/admin/auth/*: obtaining a credential is
	// genuinely consumer-specific.
	//
	// The prefix no longer carries authentication, so the GROUP does: each route
	// is registered into the group holding its credential requirement.

	// Any credential (API key | user JWT | admin session).
	any := v1.Group("", h.mw.Any())
	any.GET("/conversations", h.listConversations)
	any.POST("/conversations", h.createConversation)
	any.GET("/conversations/:id", h.getConversation)
	any.POST("/conversations/:id/messages", h.postMessage)
	any.GET("/conversations/:id/messages", h.listMessages)
	any.POST("/conversations/:id/read", h.markRead)
	any.POST("/conversations/:id/typing", h.typing)
	any.POST("/conversations/:id/close", h.closeConversation)
	any.POST("/conversations/:id/assignments", h.addAssignment)
	any.POST("/uploads", h.issueUpload)
	any.GET("/principal", h.getPrincipal)

	// Optional credential: the active brand is public when keyed by app_id, and
	// tenant-scoped when a credential is present. Any() would 401 the public form.
	v1.GET("/brands/active", h.mw.OptionalAuth(), h.getActiveBrand)

	// Integration (API key only), §4.1.
	key := v1.Group("", h.mw.APIKey())
	key.POST("/tokens", h.mintToken)
	key.POST("/conversations/:id/members", h.addMember)
	key.DELETE("/conversations/:id/members/:user_id", h.removeMember)
	key.POST("/conversations/:id/members/remap", h.remap)

	// User (JWT only), §4.2.
	user := v1.Group("", h.mw.UserJWT())
	user.POST("/push-tokens", h.registerPush)
	user.DELETE("/push-tokens", h.deregisterPush)

	// Admin auth (no session yet), §4.3 — the consumer-shaped exception.
	adminAuth := v1.Group("/admin/auth")
	adminAuth.GET("/google", h.adminGoogleLogin)
	adminAuth.GET("/google/callback", h.adminGoogleCallback)
	adminAuth.POST("/password", h.mw.RateLimitAuth(), h.adminPasswordLogin)
	if h.authn.IsStub() && h.cfg.Env != "production" {
		adminAuth.GET("/google/dev", h.adminDevLogin)
	}
	adminAuth.POST("/logout", h.adminLogout)

	// Admin session (inbox), §4.3.
	admin := v1.Group("", h.mw.Admin())
	admin.GET("/realtime/token", h.realtimeToken)
	admin.PATCH("/assignments/:id", h.patchAssignment)
	admin.GET("/contacts", h.listContacts)
	admin.GET("/contacts/:external_user_id", h.getContact)

	// Owner/admin only, §7. RBAC moved from the path prefix to the middleware
	// chain — which is where it was already enforced.
	priv := v1.Group("", h.mw.Admin(), middleware.RequireRole(models.PlatformOwner, models.PlatformAdmin))
	priv.POST("/api-keys", h.createAPIKey)
	priv.GET("/api-keys", h.listAPIKeys)
	priv.DELETE("/api-keys/:id", h.revokeAPIKey)
	priv.POST("/webhooks", h.createWebhook)
	priv.GET("/webhooks", h.listWebhooks)
	priv.PATCH("/webhooks/:id", h.updateWebhook)
	priv.DELETE("/webhooks/:id", h.deleteWebhook)
	priv.GET("/webhooks/:id/deliveries", h.listDeliveries)
	priv.GET("/channels/email", h.getEmailChannel)
	priv.PATCH("/channels/email", h.updateEmailChannel)
	priv.GET("/brands", h.listBrands)
	priv.PUT("/brands", h.saveBrands)
	priv.GET("/team", h.listTeam)
	priv.POST("/team", h.inviteAgent)
	priv.PATCH("/team/:id", h.updateAgent)
	priv.POST("/team/:id/password", h.setAgentPassword)
}
