package api

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"sort"
	"time"

	"github.com/bitllow/sild/backend/internal/apiutil"
	"github.com/bitllow/sild/backend/internal/httpx"
	"github.com/bitllow/sild/backend/internal/middleware"
	"github.com/bitllow/sild/backend/internal/policy"
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
	if !httpx.DecodeJSON(c, &req) {
		return
	}
	if req.Email == "" || req.Password == "" {
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
	if p := middleware.Get(c); p.HasRole(models.PlatformOwner) {
		return true
	}
	if newRole != nil && *newRole == models.PlatformOwner {
		httpx.Forbidden(c, "only the owner may assign the owner role")
		return false
	}
	if targetID == "" {
		return true
	}
	held, err := h.svc.RoleAssignments(c.Request.Context(), apiutil.Tenant(c), targetID)
	if err != nil {
		apiutil.Fail(c, err)
		return false
	}
	for _, a := range held {
		if a.Role == models.PlatformOwner {
			httpx.Forbidden(c, "only the owner may change the owner's record")
			return false
		}
	}
	return true
}

// setAgentPassword: POST /v1/admin/team/:id/password (owner/admin set a password).
func (h *Handler) setAgentPassword(c *gin.Context) {
	var req struct {
		Password string `json:"password"`
	}
	if !httpx.DecodeJSON(c, &req) {
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
	if !apiutil.Authorize(c, policy.RealtimeToken) {
		return
	}
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
	if !httpx.DecodeJSONOptional(c, &req) {
		return
	}
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
	if !httpx.DecodeJSON(c, &req) {
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
	res := store.SlicePage(ds, page, func(d *models.WebhookDelivery) string { return d.ID })
	// Rendered, not serialized from the model: the row has no JSON tags, so it
	// would go out with Go field names and carry tenant_id to the client.
	items := make([]gin.H, 0, len(res.Items))
	for _, d := range res.Items {
		items = append(items, gin.H{
			"id": d.ID, "event_id": d.EventID, "event_type": d.EventType,
			"attempt": d.Attempt, "status": d.Status, "status_code": d.StatusCode,
			"response": d.Response, "created_at": d.CreatedAt,
		})
	}
	apiutil.RespondPage(c, resourceDeliveries, store.Page[gin.H]{
		Items: items, NextCursor: res.NextCursor, HasMore: res.HasMore,
	})
}

func (h *Handler) listTeam(c *gin.Context) {
	page, ok := apiutil.PageParams(c, settingsPageDefaults(resourceTeam))
	if !ok {
		return
	}
	ctx, tenant := c.Request.Context(), apiutil.Tenant(c)
	admins, err := h.svc.ListAdmins(ctx, tenant)
	if err != nil {
		apiutil.Fail(c, err)
		return
	}
	// One read for the whole roster: a member's chips are their rows, grouped.
	assignments, err := h.svc.TenantRoleAssignments(ctx, tenant)
	if err != nil {
		apiutil.Fail(c, err)
		return
	}
	byMember := map[string][]map[string]any{}
	for _, a := range assignments {
		byMember[a.AdminUserID] = append(byMember[a.AdminUserID], roleAssignmentView(a))
	}
	out := make([]map[string]any, 0, len(admins))
	for i := range admins {
		a := &admins[i]
		held := byMember[a.ID]
		if held == nil {
			held = []map[string]any{}
		}
		out = append(out, map[string]any{
			"id": a.ID, "email": a.Email, "assignments": held,
			"first_name": a.FirstName, "last_name": a.LastName,
			"has_password": a.PasswordHash != nil, "created_at": a.CreatedAt,
		})
	}
	apiutil.RespondPage(c, resourceTeam, store.SlicePage(descByID(out), page, mapID))
}

func (h *Handler) updateWebhook(c *gin.Context) {
	var req struct {
		Active *bool `json:"active"`
	}
	if !httpx.DecodeJSON(c, &req) {
		return
	}
	if req.Active == nil {
		httpx.BadRequest(c, "active is required")
		return
	}
	if err := h.svc.SetWebhookActive(c.Request.Context(), apiutil.Tenant(c), c.Param("id"), *req.Active); err != nil {
		apiutil.Fail(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

// roleAssignmentView is the wire shape of one chip: the role and its scope.
func roleAssignmentView(a models.RoleAssignment) map[string]any {
	return map[string]any{"role": a.Role, "scope": a.Scope}
}

// listRoles: GET /v1/roles — the catalogue the Team screen renders from, so a
// dimension added here reaches the UI without a client release.
func (h *Handler) listRoles(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"roles": policy.Roles()})
}

// assignRole: POST /v1/team/:id/roles — give a member a role, or replace the
// scope of the one they hold.
func (h *Handler) assignRole(c *gin.Context) {
	var req struct {
		Role  models.PlatformRole `json:"role"`
		Scope models.RoleScope    `json:"scope"`
	}
	if !httpx.DecodeJSON(c, &req) {
		return
	}
	h.writeAssignment(c, c.Param("id"), req.Role, req.Scope, h.svc.AssignRole)
}

// updateRoleScope: PUT /v1/team/:id/roles/:role — rescope one assignment.
func (h *Handler) updateRoleScope(c *gin.Context) {
	var req struct {
		Scope models.RoleScope `json:"scope"`
	}
	if !httpx.DecodeJSON(c, &req) {
		return
	}
	h.writeAssignment(c, c.Param("id"), models.PlatformRole(c.Param("role")), req.Scope, h.svc.RescopeRole)
}

type assignmentWrite func(ctx context.Context, tenantID, adminID string, role models.PlatformRole, scope models.RoleScope) error

func (h *Handler) writeAssignment(c *gin.Context, targetID string, role models.PlatformRole, scope models.RoleScope, write assignmentWrite) {
	if !h.guardOwnerMutation(c, targetID, &role) {
		return
	}
	// Peer conversations stay the owner's grant to give: they are private chats
	// no support role reaches by default.
	if scope.Peer && !middleware.Get(c).HasRole(models.PlatformOwner) {
		httpx.Forbidden(c, "only the owner may grant peer access")
		return
	}
	if err := write(c.Request.Context(), apiutil.Tenant(c), targetID, role, scope); err != nil {
		apiutil.Fail(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

// removeRole: DELETE /v1/team/:id/roles/:role.
func (h *Handler) removeRole(c *gin.Context) {
	role := models.PlatformRole(c.Param("role"))
	if !h.guardOwnerMutation(c, c.Param("id"), &role) {
		return
	}
	if err := h.svc.RemoveRole(c.Request.Context(), apiutil.Tenant(c), c.Param("id"), role); err != nil {
		apiutil.Fail(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *Handler) inviteAgent(c *gin.Context) {
	var req struct {
		Email     string              `json:"email"`
		FirstName string              `json:"first_name"`
		LastName  string              `json:"last_name"`
		Role      models.PlatformRole `json:"role"`
		Scope     models.RoleScope    `json:"scope"`
	}
	if !httpx.DecodeJSON(c, &req) {
		return
	}
	if !h.guardOwnerMutation(c, "", &req.Role) {
		return
	}
	if req.Scope.Peer && !middleware.Get(c).HasRole(models.PlatformOwner) {
		httpx.Forbidden(c, "only the owner may grant peer access")
		return
	}
	a, err := h.svc.InviteAgent(c.Request.Context(), apiutil.Tenant(c), req.Email, req.FirstName, req.LastName, req.Role, req.Scope)
	if err != nil {
		apiutil.Fail(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"id": a.ID, "email": a.Email, "first_name": a.FirstName, "last_name": a.LastName, "assignments": []map[string]any{{"role": req.Role, "scope": req.Scope}}})
}

// settingsPageDefaults is the paging contract for tenant-settings collections.
// They are bounded per tenant, so the default limit of 100 means one page covers
// a whole team or key list in practice — but the mechanism is real, not a stub:
// seed 150 keys and the second page works, because nothing here is special-cased.
func settingsPageDefaults(resource string) apiutil.PageDefaults {
	return apiutil.PageDefaults{
		Resource:   resource,
		Limit:      100,
		Sort:       store.SortID,
		Order:      store.OrderDesc,
		FixedOrder: true,
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
