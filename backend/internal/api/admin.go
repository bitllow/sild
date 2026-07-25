package api

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"sort"
	"time"

	"github.com/bitllow/sild/backend/internal/apiutil"
	"github.com/bitllow/sild/backend/internal/httpx"
	"github.com/bitllow/sild/backend/internal/middleware"
	"github.com/bitllow/sild/backend/internal/store"
	"github.com/bitllow/sild/backend/internal/store/models"
	"github.com/gin-gonic/gin"
)

func randomState() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// ── Auth (§4.3) ─────────────────────────────────────────────────────────────

func (h *Handler) adminGoogleLogin(c *gin.Context) {
	state := randomState()
	c.SetCookie("sild_oauth_state", state, 600, "/", "", h.cfg.Env == "production", true)
	c.Redirect(http.StatusFound, h.authn.LoginURL(state))
}

func (h *Handler) adminGoogleCallback(c *gin.Context) {
	want, _ := c.Cookie("sild_oauth_state")
	if want == "" || c.Query("state") != want {
		httpx.Unauthorized(c, "invalid oauth state")
		return
	}
	email, err := h.authn.Resolve(c.Request.Context(), c.Query("code"))
	if err != nil {
		httpx.Unauthorized(c, "oauth failed")
		return
	}
	h.startSession(c, email)
}

// adminDevLogin is a non-production stub: ?email=<admin email> → session.
func (h *Handler) adminDevLogin(c *gin.Context) {
	email := c.Query("email")
	if email == "" {
		email = c.Query("code")
	}
	h.startSession(c, email)
}

func (h *Handler) startSession(c *gin.Context, email string) {
	raw, exp, err := h.svc.CreateSession(c.Request.Context(), email)
	if err != nil {
		apiutil.Fail(c, err)
		return
	}
	c.SetCookie(middleware.AdminCookieName, raw, h.cfg.Auth.AdminSessionTTLHours*3600, "/", "", h.cfg.Env == "production", true)
	c.JSON(http.StatusOK, gin.H{"status": "authenticated", "expires_at": exp})
}

// adminPasswordLogin: POST /v1/admin/auth/password (§2.4 email/password method).
func (h *Handler) adminPasswordLogin(c *gin.Context) {
	var req struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.Email == "" || req.Password == "" {
		httpx.BadRequest(c, "email and password are required")
		return
	}
	raw, exp, err := h.svc.CreateSessionWithPassword(c.Request.Context(), req.Email, req.Password)
	if err != nil {
		httpx.Unauthorized(c, "invalid credentials")
		return
	}
	c.SetCookie(middleware.AdminCookieName, raw, h.cfg.Auth.AdminSessionTTLHours*3600, "/", "", h.cfg.Env == "production", true)
	c.JSON(http.StatusOK, gin.H{"status": "authenticated", "expires_at": exp})
}

// guardOwnerMutation: only an owner may create an owner or touch an owner's record.
// Every team route calls it — invite mints owners, PATCH promotes, password resets
// take over. targetID is "" on create; newRole is nil when no role is being set.
func (h *Handler) guardOwnerMutation(c *gin.Context, targetID string, newRole *models.PlatformRole) bool {
	if p := middleware.Get(c); p != nil && p.Role == models.PlatformOwner {
		return true
	}
	if newRole != nil && *newRole == models.PlatformOwner {
		httpx.Forbidden(c, "only the owner may assign the owner role")
		return false
	}
	if targetID == "" {
		return true
	}
	target, err := h.svc.GetAdmin(c.Request.Context(), apiutil.Tenant(c), targetID)
	if err != nil {
		apiutil.Fail(c, err)
		return false
	}
	if target.PlatformRole == models.PlatformOwner {
		httpx.Forbidden(c, "only the owner may change the owner's record")
		return false
	}
	return true
}

// setAgentPassword: POST /v1/admin/team/:id/password (owner/admin set a password).
func (h *Handler) setAgentPassword(c *gin.Context) {
	var req struct {
		Password string `json:"password"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.BadRequest(c, "invalid body")
		return
	}
	// A password reset is account takeover by another name.
	if !h.guardOwnerMutation(c, c.Param("id"), nil) {
		return
	}
	if err := h.svc.SetAdminPassword(c.Request.Context(), apiutil.Tenant(c), c.Param("id"), req.Password); err != nil {
		apiutil.Fail(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *Handler) adminLogout(c *gin.Context) {
	if raw, err := c.Cookie(middleware.AdminCookieName); err == nil {
		_ = h.svc.Logout(c.Request.Context(), raw)
	}
	c.SetCookie(middleware.AdminCookieName, "", -1, "/", "", h.cfg.Env == "production", true)
	c.Status(http.StatusNoContent)
}

// realtimeToken mints a short-lived agent JWT the inbox uses to open its
// egress-only realtime connection (§5). The session cookie can't ride a
// cross-origin WebSocket, so the browser swaps it for this token over REST.
func (h *Handler) realtimeToken(c *gin.Context) {
	p := middleware.Get(c)
	tok, exp, err := h.km.MintAgent(c.Request.Context(), p.AdminID, p.TenantID, time.Hour)
	if err != nil {
		apiutil.Fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"token": tok, "expires_at": exp})
}

// ── Inbox (§4.3) ────────────────────────────────────────────────────────────

// ── Settings: API keys, webhooks, team (§4.3, owner/admin only) ──────────────

func (h *Handler) createAPIKey(c *gin.Context) {
	var req struct {
		Label string `json:"label"`
	}
	_ = c.ShouldBindJSON(&req)
	full, rec, err := h.svc.CreateAPIKey(c.Request.Context(), apiutil.Tenant(c), req.Label)
	if err != nil {
		apiutil.Fail(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"id": rec.ID, "key": full, "label": rec.Label, "prefix": rec.Prefix})
}

func (h *Handler) listAPIKeys(c *gin.Context) {
	page, ok := apiutil.PageParams(c, settingsPageDefaults(resourceAPIKeys))
	if !ok {
		return
	}
	keys, err := h.svc.ListAPIKeys(c.Request.Context(), apiutil.Tenant(c))
	if err != nil {
		apiutil.Fail(c, err)
		return
	}
	out := make([]map[string]any, 0, len(keys))
	for _, k := range keys {
		out = append(out, map[string]any{
			"id": k.ID, "label": k.Label, "prefix": k.Prefix,
			"created_at": k.CreatedAt, "revoked_at": k.RevokedAt,
		})
	}
	apiutil.RespondPage(c, resourceAPIKeys, store.SlicePage(descByID(out), page, mapID))
}

func (h *Handler) revokeAPIKey(c *gin.Context) {
	if err := h.svc.RevokeAPIKey(c.Request.Context(), apiutil.Tenant(c), c.Param("id")); err != nil {
		apiutil.Fail(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *Handler) createWebhook(c *gin.Context) {
	var req struct {
		URL    string   `json:"url"`
		Events []string `json:"events"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.BadRequest(c, "invalid body")
		return
	}
	ep, err := h.svc.CreateWebhook(c.Request.Context(), apiutil.Tenant(c), req.URL, req.Events)
	if err != nil {
		apiutil.Fail(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"id": ep.ID, "secret": ep.Secret})
}

func (h *Handler) listWebhooks(c *gin.Context) {
	page, ok := apiutil.PageParams(c, settingsPageDefaults(resourceWebhooks))
	if !ok {
		return
	}
	eps, err := h.svc.ListWebhooks(c.Request.Context(), apiutil.Tenant(c))
	if err != nil {
		apiutil.Fail(c, err)
		return
	}
	out := make([]map[string]any, 0, len(eps))
	for _, e := range eps {
		events := make([]string, 0, len(e.Events))
		for _, ev := range e.Events {
			events = append(events, ev.Event)
		}
		out = append(out, map[string]any{"id": e.ID, "url": e.URL, "events": events, "active": e.Active, "created_at": e.CreatedAt})
	}
	apiutil.RespondPage(c, resourceWebhooks, store.SlicePage(descByID(out), page, mapID))
}

func (h *Handler) deleteWebhook(c *gin.Context) {
	if err := h.svc.DeleteWebhook(c.Request.Context(), apiutil.Tenant(c), c.Param("id")); err != nil {
		apiutil.Fail(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *Handler) listDeliveries(c *gin.Context) {
	page, ok := apiutil.PageParams(c, settingsPageDefaults(resourceDeliveries))
	if !ok {
		return
	}
	ds, err := h.svc.ListDeliveries(c.Request.Context(), apiutil.Tenant(c), c.Param("id"))
	if err != nil {
		apiutil.Fail(c, err)
		return
	}
	sort.Slice(ds, func(i, j int) bool { return ds[i].ID > ds[j].ID })
	apiutil.RespondPage(c, resourceDeliveries, store.SlicePage(ds, page, func(d *models.WebhookDelivery) string { return d.ID }))
}

func (h *Handler) listTeam(c *gin.Context) {
	page, ok := apiutil.PageParams(c, settingsPageDefaults(resourceTeam))
	if !ok {
		return
	}
	admins, err := h.svc.ListAdmins(c.Request.Context(), apiutil.Tenant(c))
	if err != nil {
		apiutil.Fail(c, err)
		return
	}
	out := make([]map[string]any, 0, len(admins))
	for i := range admins {
		a := &admins[i]
		out = append(out, map[string]any{
			"id": a.ID, "email": a.Email, "platform_role": a.PlatformRole,
			"first_name": a.FirstName, "last_name": a.LastName,
			"peer_access":  a.PeerAccess,
			"has_password": a.PasswordHash != nil, "created_at": a.CreatedAt,
		})
	}
	apiutil.RespondPage(c, resourceTeam, store.SlicePage(descByID(out), page, mapID))
}

func (h *Handler) updateWebhook(c *gin.Context) {
	var req struct {
		Active *bool `json:"active"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.Active == nil {
		httpx.BadRequest(c, "active is required")
		return
	}
	if err := h.svc.SetWebhookActive(c.Request.Context(), apiutil.Tenant(c), c.Param("id"), *req.Active); err != nil {
		apiutil.Fail(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *Handler) updateAgent(c *gin.Context) {
	var req struct {
		PlatformRole *models.PlatformRole `json:"platform_role"`
		PeerAccess   *bool                `json:"peer_access"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.BadRequest(c, "invalid body")
		return
	}
	if req.PlatformRole == nil && req.PeerAccess == nil {
		httpx.BadRequest(c, "nothing to update")
		return
	}
	ctx, tenant, id := c.Request.Context(), apiutil.Tenant(c), c.Param("id")
	if !h.guardOwnerMutation(c, id, req.PlatformRole) {
		return
	}
	// Peer access is the owner's grant to give.
	if req.PeerAccess != nil {
		if p := middleware.Get(c); p == nil || p.Role != models.PlatformOwner {
			httpx.Forbidden(c, "only the owner may change peer access")
			return
		}
	}
	if req.PlatformRole != nil {
		if err := h.svc.SetAdminRole(ctx, tenant, id, *req.PlatformRole); err != nil {
			apiutil.Fail(c, err)
			return
		}
	}
	if req.PeerAccess != nil {
		if err := h.svc.SetPeerAccess(ctx, tenant, id, *req.PeerAccess); err != nil {
			apiutil.Fail(c, err)
			return
		}
	}
	c.Status(http.StatusNoContent)
}

func (h *Handler) inviteAgent(c *gin.Context) {
	var req struct {
		Email        string              `json:"email"`
		FirstName    string              `json:"first_name"`
		LastName     string              `json:"last_name"`
		PlatformRole models.PlatformRole `json:"platform_role"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.BadRequest(c, "invalid body")
		return
	}
	if !h.guardOwnerMutation(c, "", &req.PlatformRole) {
		return
	}
	a, err := h.svc.InviteAgent(c.Request.Context(), apiutil.Tenant(c), req.Email, req.FirstName, req.LastName, req.PlatformRole)
	if err != nil {
		apiutil.Fail(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"id": a.ID, "email": a.Email, "first_name": a.FirstName, "last_name": a.LastName, "platform_role": a.PlatformRole})
}

// settingsPageDefaults is the paging contract for tenant-settings collections.
// They are bounded per tenant, so the default limit of 100 means one page covers
// a whole team or key list in practice — but the mechanism is real, not a stub:
// seed 150 keys and the second page works, because nothing here is special-cased.
func settingsPageDefaults(resource string) apiutil.PageDefaults {
	return apiutil.PageDefaults{
		Resource: resource,
		Limit:    100,
		Sort:     store.SortID,
		Order:    store.OrderDesc,
	}
}

// mapID is the keyset id for a rendered settings row.
func mapID(m *map[string]any) string {
	id, _ := (*m)["id"].(string)
	return id
}

// descByID orders rendered settings rows newest-first, matching the descending
// cursor SlicePage mints. The repos return them oldest-first, and a cursor taken
// from an ascending slice would re-serve page one instead of advancing.
func descByID(rows []map[string]any) []map[string]any {
	sort.Slice(rows, func(i, j int) bool { return mapID(&rows[i]) > mapID(&rows[j]) })
	return rows
}
