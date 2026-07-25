package api

import (
	"net/http"

	"github.com/bitllow/sild/backend/internal/apiutil"
	"github.com/bitllow/sild/backend/internal/httpx"
	"github.com/bitllow/sild/backend/internal/store/models"
	"github.com/gin-gonic/gin"
)

// registerPush: POST /v1/me/push-tokens (§4.2, §5.5).
func (h *Handler) registerPush(c *gin.Context) {
	var req struct {
		Platform models.PushPlatform `json:"platform"`
		Token    string              `json:"token"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.BadRequest(c, "invalid body")
		return
	}
	if err := h.svc.RegisterPush(c.Request.Context(), apiutil.Tenant(c), apiutil.CallerParticipant(c), req.Platform, req.Token); err != nil {
		apiutil.Fail(c, err)
		return
	}
	c.Status(http.StatusCreated)
}

// deregisterPush: DELETE /v1/me/push-tokens (§4.2). Token in body, scoped to sub.
func (h *Handler) deregisterPush(c *gin.Context) {
	var req struct {
		Token string `json:"token"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.BadRequest(c, "invalid body")
		return
	}
	if err := h.svc.DeregisterPush(c.Request.Context(), apiutil.Tenant(c), apiutil.CallerParticipant(c), req.Token); err != nil {
		apiutil.Fail(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}
