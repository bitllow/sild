package api

import (
	"encoding/json"
	"net/http"

	"github.com/bitllow/sild/backend/internal/apiutil"
	"github.com/bitllow/sild/backend/internal/domain"
	"github.com/bitllow/sild/backend/internal/httpx"
	"github.com/bitllow/sild/backend/internal/middleware"
	"github.com/bitllow/sild/backend/internal/policy"
	"github.com/bitllow/sild/backend/internal/principal"
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

// createConversation: POST /v1/conversations. Untyped; atomic create.
//
// Absorbs POST /me/support-requests and POST /admin/support-requests. kind is
// derived from open_assignment (support when an assignment opens, peer
// otherwise), so merging the routes naively would let a user JWT omit the flag
// and mint a PEER conversation — visible to every peer_access operator, i.e. a
// way to inject rows into the operator peer inbox.
//
// Per-principal contract:
//
//	user JWT      members forced to [self],       open_assignment forced true
//	admin session may name one external_user_id,  open_assignment forced true
//	API key       arbitrary members,              caller's choice
//
// A non-key principal asking for open_assignment:false gets 403 — the client
// asked for something it may not have, and a silent upgrade would hide that.
func (h *Handler) createConversation(c *gin.Context) {
	var req struct {
		Reference string          `json:"reference"`
		Metadata  json.RawMessage `json:"metadata"`
		Members   []struct {
			UserID   string          `json:"user_id"`
			ConvRole models.ConvRole `json:"conv_role"`
			Metadata json.RawMessage `json:"metadata"`
		} `json:"members"`
		OpenAssignment *bool  `json:"open_assignment"`
		ExternalUserID string `json:"external_user_id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.BadRequest(c, "invalid body")
		return
	}

	p := middleware.Get(c)

	// An API key keeps the original default: absent open_assignment means false,
	// i.e. a peer conversation. User and admin principals have no such default —
	// they may only open support, so absent means true for them.
	openAssignment := true
	if p.Kind == principal.KindAPIKey {
		openAssignment = req.OpenAssignment != nil && *req.OpenAssignment
	} else if req.OpenAssignment != nil {
		openAssignment = *req.OpenAssignment
	}
	wantsPeer := !openAssignment

	action := policy.ConversationsCreateSupport
	if wantsPeer {
		action = policy.ConversationsCreatePeer
	}
	if !apiutil.Authorize(c, action) {
		return
	}

	in := domain.CreateConversationInput{
		Reference:      req.Reference,
		Metadata:       req.Metadata,
		OpenAssignment: openAssignment,
	}

	switch p.Kind {
	case principal.KindUser:
		// Self as the only member, in the client role — the body's member list is
		// ignored rather than trusted, so a user cannot add anyone to a
		// conversation they open.
		in.Members = []domain.MemberInput{
			{UserID: p.Subject, ConvRole: models.RoleClient, Metadata: req.Metadata},
		}

	case principal.KindAdmin:
		if req.ExternalUserID == "" {
			httpx.BadRequest(c, "external_user_id is required")
			return
		}
		in.Members = []domain.MemberInput{
			{UserID: req.ExternalUserID, ConvRole: models.RoleClient, Metadata: req.Metadata},
		}

	default: // API key
		for _, m := range req.Members {
			in.Members = append(in.Members, domain.MemberInput{UserID: m.UserID, ConvRole: m.ConvRole, Metadata: m.Metadata})
		}
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
