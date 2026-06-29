package api

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"time"

	"github.com/bitllow/sild/backend/internal/apiutil"
	"github.com/bitllow/sild/backend/internal/httpx"
	"github.com/bitllow/sild/backend/internal/middleware"
	"github.com/bitllow/sild/backend/internal/store"
	"github.com/bitllow/sild/backend/internal/store/models"
	"github.com/bitllow/sild/backend/internal/views"
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

// setAgentPassword: POST /v1/admin/team/:id/password (owner/admin set a password).
func (h *Handler) setAgentPassword(c *gin.Context) {
	var req struct {
		Password string `json:"password"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.BadRequest(c, "invalid body")
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

func (h *Handler) listAssignments(c *gin.Context) {
	p := store.QueueParams{Sort: store.QueueSortLastActivity, Desc: true, Limit: 30}
	if s := c.Query("status"); s != "" {
		v := models.AssignmentStatus(s)
		p.Status = &v
	}
	if a := c.Query("assignee"); a != "" {
		if a == "me" {
			a = middleware.Get(c).AdminID
		}
		p.Assignee = &a
	}
	if c.Query("sort") == string(store.QueueSortCreated) {
		p.Sort = store.QueueSortCreated
	}
	if c.Query("order") == "asc" {
		p.Desc = false
	}
	p.Limit = atoiDefault(c.Query("limit"), p.Limit) // store clamps to [1,100]
	if cur := c.Query("cursor"); cur != "" {
		cc, err := decodeQueueCursor(cur)
		if err != nil {
			httpx.BadRequest(c, "invalid cursor")
			return
		}
		p.Cursor = cc
	}

	page, err := h.svc.ListQueue(c.Request.Context(), apiutil.Tenant(c), p)
	if err != nil {
		apiutil.Fail(c, err)
		return
	}
	items := make([]map[string]any, 0, len(page.Items))
	for i := range page.Items {
		items = append(items, views.QueueRow(&page.Items[i]))
	}
	c.JSON(http.StatusOK, gin.H{
		"items":       items,
		"next_cursor": encodeQueueCursor(page.NextCursor),
		"has_more":    page.HasMore,
	})
}

// queueCursorDTO is the wire form of a keyset cursor, base64(JSON).
type queueCursorDTO struct {
	V time.Time `json:"v"`
	ID string   `json:"id"`
}

func encodeQueueCursor(c *store.QueueCursor) any {
	if c == nil {
		return nil
	}
	b, err := json.Marshal(queueCursorDTO{V: c.Value, ID: c.ID})
	if err != nil {
		return nil
	}
	return base64.RawURLEncoding.EncodeToString(b)
}

func decodeQueueCursor(s string) (*store.QueueCursor, error) {
	b, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return nil, err
	}
	var dto queueCursorDTO
	if err := json.Unmarshal(b, &dto); err != nil {
		return nil, err
	}
	return &store.QueueCursor{Value: dto.V, ID: dto.ID}, nil
}

func (h *Handler) adminOpenSupportRequest(c *gin.Context) {
	var req struct {
		ExternalUserID string          `json:"external_user_id"`
		Metadata       json.RawMessage `json:"metadata"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.BadRequest(c, "invalid body")
		return
	}
	conv, assignment, err := h.svc.OpenSupportRequest(c.Request.Context(), apiutil.Tenant(c), req.ExternalUserID, req.Metadata)
	if err != nil {
		apiutil.Fail(c, err)
		return
	}
	c.JSON(http.StatusCreated, views.Conversation(conv, conv.Members, assignment))
}

func (h *Handler) claimAssignment(c *gin.Context) {
	a, err := h.svc.ClaimAssignment(c.Request.Context(), apiutil.Tenant(c), c.Param("id"), middleware.Get(c).AdminID)
	if err != nil {
		apiutil.Fail(c, err)
		return
	}
	c.JSON(http.StatusOK, views.Assignment(a))
}

func (h *Handler) closeAssignmentAdmin(c *gin.Context) {
	a, err := h.svc.CloseAssignment(c.Request.Context(), apiutil.Tenant(c), c.Param("id"))
	if err != nil {
		apiutil.Fail(c, err)
		return
	}
	c.JSON(http.StatusOK, views.Assignment(a))
}

func (h *Handler) adminSearch(c *gin.Context) {
	res, err := h.search.Search(c.Request.Context(), apiutil.Tenant(c),
		c.Query("q"), middleware.Get(c).AdminID, c.Query("before"), atoiDefault(c.Query("limit"), 25))
	if err != nil {
		apiutil.Fail(c, err)
		return
	}
	c.JSON(http.StatusOK, res)
}

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
	c.JSON(http.StatusOK, out)
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
	c.JSON(http.StatusOK, out)
}

func (h *Handler) deleteWebhook(c *gin.Context) {
	if err := h.svc.DeleteWebhook(c.Request.Context(), apiutil.Tenant(c), c.Param("id")); err != nil {
		apiutil.Fail(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *Handler) listDeliveries(c *gin.Context) {
	ds, err := h.svc.ListDeliveries(c.Request.Context(), apiutil.Tenant(c), c.Param("id"))
	if err != nil {
		apiutil.Fail(c, err)
		return
	}
	c.JSON(http.StatusOK, ds)
}

func (h *Handler) listTeam(c *gin.Context) {
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
			"has_password": a.PasswordHash != nil, "created_at": a.CreatedAt,
		})
	}
	c.JSON(http.StatusOK, out)
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
		PlatformRole models.PlatformRole `json:"platform_role"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.BadRequest(c, "invalid body")
		return
	}
	if err := h.svc.SetAdminRole(c.Request.Context(), apiutil.Tenant(c), c.Param("id"), req.PlatformRole); err != nil {
		apiutil.Fail(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *Handler) inviteAgent(c *gin.Context) {
	var req struct {
		Email        string              `json:"email"`
		PlatformRole models.PlatformRole `json:"platform_role"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.BadRequest(c, "invalid body")
		return
	}
	a, err := h.svc.InviteAgent(c.Request.Context(), apiutil.Tenant(c), req.Email, req.PlatformRole)
	if err != nil {
		apiutil.Fail(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"id": a.ID, "email": a.Email, "platform_role": a.PlatformRole})
}
