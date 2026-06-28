// Package api wires the REST surface (§4). Routes are grouped by audience
// (integration/user/admin/public) at the file level; shared paths that accept
// multiple credential types use the Any() middleware and authorize by principal.
package api

import (
	"github.com/bitllow/sild/backend/internal/auth"
	"github.com/bitllow/sild/backend/internal/config"
	"github.com/bitllow/sild/backend/internal/domain"
	"github.com/bitllow/sild/backend/internal/middleware"
	"github.com/bitllow/sild/backend/internal/storage"
	"github.com/bitllow/sild/backend/internal/store/models"
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

	v1 := e.Group("/v1")

	// Email inbound (§6.2): provider posts here, signature is the gate.
	v1.POST("/email/inbound", h.emailInbound)

	// Shared paths (API key | user JWT | admin), §4.1/§4.2 same URLs.
	any := v1.Group("", h.mw.Any())
	any.GET("/conversations/:id", h.getConversation)
	any.POST("/conversations/:id/messages", h.postMessage)
	any.GET("/conversations/:id/messages", h.listMessages)
	any.POST("/conversations/:id/read", h.markRead)
	any.POST("/conversations/:id/typing", h.typing)
	any.POST("/conversations/:id/close", h.closeConversation)
	any.POST("/uploads", h.issueUpload)

	// Integration (API key only), §4.1.
	key := v1.Group("", h.mw.APIKey())
	key.POST("/tokens", h.mintToken)
	key.POST("/conversations", h.createConversation)
	key.POST("/conversations/:id/members", h.addMember)
	key.DELETE("/conversations/:id/members/:user_id", h.removeMember)
	key.POST("/conversations/:id/assignments", h.addAssignment)
	key.POST("/conversations/:id/members/remap", h.remap)

	// User (JWT only), §4.2.
	me := v1.Group("/me", h.mw.UserJWT())
	me.GET("/conversations", h.listMyConversations)
	me.POST("/support-requests", h.openSupportRequest)
	me.POST("/push-tokens", h.registerPush)
	me.DELETE("/push-tokens", h.deregisterPush)

	// Admin auth (no session yet), §4.3.
	adminAuth := v1.Group("/admin/auth")
	adminAuth.GET("/google", h.adminGoogleLogin)
	adminAuth.GET("/google/callback", h.adminGoogleCallback)
	if h.authn.IsStub() && h.cfg.Env != "production" {
		adminAuth.GET("/google/dev", h.adminDevLogin)
	}
	adminAuth.POST("/logout", h.adminLogout)

	// Admin session (inbox), §4.3.
	admin := v1.Group("/admin", h.mw.Admin())
	admin.GET("/assignments", h.listAssignments)
	admin.POST("/support-requests", h.adminOpenSupportRequest)
	admin.POST("/assignments/:id/claim", h.claimAssignment)
	admin.POST("/assignments/:id/close", h.closeAssignmentAdmin)
	admin.GET("/search", h.adminSearch)

	// Admin owner/admin only, §7.
	priv := v1.Group("/admin", h.mw.Admin(), middleware.RequireRole(models.PlatformOwner, models.PlatformAdmin))
	priv.POST("/api-keys", h.createAPIKey)
	priv.GET("/api-keys", h.listAPIKeys)
	priv.DELETE("/api-keys/:id", h.revokeAPIKey)
	priv.POST("/webhooks", h.createWebhook)
	priv.GET("/webhooks", h.listWebhooks)
	priv.DELETE("/webhooks/:id", h.deleteWebhook)
	priv.GET("/webhooks/:id/deliveries", h.listDeliveries)
	priv.POST("/team", h.inviteAgent)
}
