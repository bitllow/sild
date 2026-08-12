package api

import (
	"net/http"

	"github.com/bitllow/sild/backend/internal/httpx"
	"github.com/bitllow/sild/backend/internal/middleware"
	"github.com/bitllow/sild/backend/internal/policy"
	"github.com/bitllow/sild/backend/internal/principal"
	"github.com/gin-gonic/gin"
)

// getPrincipal: GET /v1/principal — who am I, discriminated by kind. Grants carry
// each action's SCOPE, not just its name: two agents can both hold
// conversations.list and only the scope tells them apart.
func (h *Handler) getPrincipal(c *gin.Context) {
	p := middleware.Get(c)
	if p == nil {
		httpx.Unauthorized(c, "authentication required")
		return
	}
	out := gin.H{
		"kind":      p.Kind,
		"tenant_id": p.TenantID,
		"grants":    policy.Grants(p),
	}
	switch p.Kind {
	case principal.KindAdmin:
		// Assignments, not role names: a translator refused /v1/team has no other
		// way to see which projects and languages they were granted.
		held := make([]map[string]any, 0, len(p.Assignments))
		for _, a := range p.Assignments {
			held = append(held, map[string]any{"role": a.Role, "scope": a.Scope})
		}
		subject := gin.H{"id": p.AdminID, "assignments": held}
		if a, err := h.svc.GetAdmin(c.Request.Context(), p.TenantID, p.AdminID); err == nil {
			subject["email"] = a.Email
			subject["first_name"] = a.FirstName
			subject["last_name"] = a.LastName
		}
		out["subject"] = subject
	case principal.KindUser:
		out["subject"] = gin.H{"id": p.Subject}
	}
	c.JSON(http.StatusOK, out)
}
