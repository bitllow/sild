package api

import (
	"encoding/json"
	"net/http"

	"github.com/bitllow/sild/backend/internal/apiutil"
	"github.com/bitllow/sild/backend/internal/domain"
	"github.com/bitllow/sild/backend/internal/httpx"
	"github.com/bitllow/sild/backend/internal/store/models"
	"github.com/bitllow/sild/backend/internal/views"
	"github.com/gin-gonic/gin"
)

// mintToken: POST /v1/tokens (§4.1). Mints a user JWT (authed or guest).
func (h *Handler) mintToken(c *gin.Context) {
	var req struct {
		UserID     string `json:"user_id"`
		TTLSeconds int    `json:"ttl_seconds"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.BadRequest(c, "invalid body")
		return
	}
	token, exp, err := h.svc.MintToken(c.Request.Context(), apiutil.Tenant(c), req.UserID, req.TTLSeconds)
	if err != nil {
		apiutil.Fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"token": token, "expires_at": exp})
}

// createConversation: POST /v1/conversations (§4.1). Untyped; atomic create.
func (h *Handler) createConversation(c *gin.Context) {
	var req struct {
		Reference string `json:"reference"`
		Metadata  json.RawMessage `json:"metadata"`
		Members   []struct {
			UserID   string          `json:"user_id"`
			ConvRole models.ConvRole `json:"conv_role"`
			Metadata json.RawMessage `json:"metadata"`
		} `json:"members"`
		OpenAssignment bool `json:"open_assignment"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.BadRequest(c, "invalid body")
		return
	}
	in := domain.CreateConversationInput{
		Reference: req.Reference, Metadata: req.Metadata, OpenAssignment: req.OpenAssignment,
	}
	for _, m := range req.Members {
		in.Members = append(in.Members, domain.MemberInput{UserID: m.UserID, ConvRole: m.ConvRole, Metadata: m.Metadata})
	}
	conv, err := h.svc.CreateConversation(c.Request.Context(), apiutil.Tenant(c), in)
	if err != nil {
		apiutil.Fail(c, err)
		return
	}
	c.JSON(http.StatusCreated, views.Conversation(conv, conv.Members, conv.Assignment))
}

// addMember: POST /v1/conversations/:id/members (§4.1).
func (h *Handler) addMember(c *gin.Context) {
	var req struct {
		UserID   string          `json:"user_id"`
		ConvRole models.ConvRole `json:"conv_role"`
		Metadata json.RawMessage `json:"metadata"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.BadRequest(c, "invalid body")
		return
	}
	m, err := h.svc.AddMember(c.Request.Context(), apiutil.Tenant(c), c.Param("id"),
		domain.MemberInput{UserID: req.UserID, ConvRole: req.ConvRole, Metadata: req.Metadata})
	if err != nil {
		apiutil.Fail(c, err)
		return
	}
	c.JSON(http.StatusCreated, views.Member(m))
}

// removeMember: DELETE /v1/conversations/:id/members/:user_id (§4.1). 409 if it
// would leave an open conversation empty.
func (h *Handler) removeMember(c *gin.Context) {
	if err := h.svc.RemoveMember(c.Request.Context(), apiutil.Tenant(c), c.Param("id"), c.Param("user_id")); err != nil {
		apiutil.Fail(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

// addAssignment: POST /v1/conversations/:id/assignments (§4.1).
func (h *Handler) addAssignment(c *gin.Context) {
	a, err := h.svc.AddAssignment(c.Request.Context(), apiutil.Tenant(c), c.Param("id"))
	if err != nil {
		apiutil.Fail(c, err)
		return
	}
	c.JSON(http.StatusCreated, views.Assignment(a))
}

// remap: POST /v1/conversations/:id/members/remap (§4.5, guest claim).
func (h *Handler) remap(c *gin.Context) {
	var req struct {
		FromUserID string `json:"from_user_id"`
		ToUserID   string `json:"to_user_id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.BadRequest(c, "invalid body")
		return
	}
	if err := h.svc.Remap(c.Request.Context(), apiutil.Tenant(c), c.Param("id"), req.FromUserID, req.ToUserID); err != nil {
		apiutil.Fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}
