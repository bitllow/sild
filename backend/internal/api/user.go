package api

import (
	"encoding/json"
	"net/http"

	"github.com/bitllow/sild/backend/internal/apiutil"
	"github.com/bitllow/sild/backend/internal/httpx"
	"github.com/bitllow/sild/backend/internal/store/models"
	"github.com/bitllow/sild/backend/internal/views"
	"github.com/gin-gonic/gin"
)

// listMyConversations: GET /v1/me/conversations (§4.2).
func (h *Handler) listMyConversations(c *gin.Context) {
	convs, err := h.svc.ListUserConversations(c.Request.Context(), apiutil.Tenant(c), apiutil.Subject(c))
	if err != nil {
		apiutil.Fail(c, err)
		return
	}
	c.JSON(http.StatusOK, convs)
}

// openSupportRequest: POST /v1/me/support-requests (§4.2). Self as client; not
// deduped — a user may have many open at once.
func (h *Handler) openSupportRequest(c *gin.Context) {
	var req struct {
		Metadata json.RawMessage `json:"metadata"`
	}
	_ = c.ShouldBindJSON(&req)
	conv, assignment, err := h.svc.OpenSupportRequest(c.Request.Context(), apiutil.Tenant(c), apiutil.Subject(c), req.Metadata)
	if err != nil {
		apiutil.Fail(c, err)
		return
	}
	c.JSON(http.StatusCreated, views.Conversation(conv, conv.Members, assignment))
}

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
