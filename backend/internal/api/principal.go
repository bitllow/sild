package api

import (
	"net/http"

	"github.com/bitllow/sild/backend/internal/httpx"
	"github.com/bitllow/sild/backend/internal/middleware"
	"github.com/bitllow/sild/backend/internal/policy"
	"github.com/bitllow/sild/backend/internal/principal"
	"github.com/gin-gonic/gin"
)

// getPrincipal: GET /v1/principal — who am I, for any credential.
//
// Replaces GET /v1/admin/me, which was consumer-shaped ("me the operator").
// The response is discriminated by kind rather than the route being restricted.
//
// Grants carry the SCOPE each action is held over, not just its name: an agent
// with peer_access and one without both hold conversations.list, and only the
// scope distinguishes them. A flat list would send the inbox back to reading
// peer_access itself — the duplicate policy logic this exists to remove.
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
		subject := gin.H{"id": p.AdminID, "role": p.Role}
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
