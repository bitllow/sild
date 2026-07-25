package api

import (
	"net/http"

	"github.com/bitllow/sild/backend/internal/apiutil"
	"github.com/bitllow/sild/backend/internal/httpx"
	"github.com/bitllow/sild/backend/internal/middleware"
	"github.com/bitllow/sild/backend/internal/policy"
	"github.com/bitllow/sild/backend/internal/store/models"
	"github.com/bitllow/sild/backend/internal/views"
	"github.com/gin-gonic/gin"
)

// patchAssignment: PATCH /v1/assignments/:id, replacing
// POST /assignments/:id/claim and .../close.
//
// This is a spelling change, not a capability change. The accepted values are
// deliberately narrow:
//
//   - assignee_actor_id may only be "me". Claiming for ANOTHER operator has no
//     route today, and accepting an arbitrary id here would quietly introduce
//     reassignment — letting a plain agent move another operator's work.
//   - status may only be "closed". queued/assigned transitions are legal in the
//     state machine but have no route, so they stay unexposed.
//
// Both fields at once is a 400: claim-then-close is two calls, and allowing the
// combination would need a transactional ordering rule for no gain.
func (h *Handler) patchAssignment(c *gin.Context) {
	var req struct {
		AssigneeActorID *string `json:"assignee_actor_id"`
		Status          *string `json:"status"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.BadRequest(c, "invalid body")
		return
	}
	if req.AssigneeActorID != nil && req.Status != nil {
		httpx.BadRequest(c, "set assignee_actor_id or status, not both")
		return
	}

	ctx, tenant, id := c.Request.Context(), apiutil.Tenant(c), c.Param("id")

	// Authorize against the CONVERSATION, not the assignment id: an assignment
	// carries no access rules of its own, and an id-only check would let an
	// operator mutate work on a conversation they cannot see.
	convID, err := h.svc.AssignmentConversation(ctx, tenant, id)
	if err != nil {
		apiutil.Fail(c, err)
		return
	}

	switch {
	case req.AssigneeActorID != nil:
		if *req.AssigneeActorID != "me" {
			httpx.FieldError(c, http.StatusBadRequest, httpx.CodeBadRequest,
				"only \"me\" is accepted", map[string]string{
					"assignee_actor_id": "reassigning to another operator is not supported",
				})
			return
		}
		if !apiutil.AuthorizeConversation(c, h.svc, policy.AssignmentsClaim, convID) {
			return
		}
		a, err := h.svc.ClaimAssignment(ctx, tenant, id, middleware.Get(c).AdminID)
		if err != nil {
			apiutil.Fail(c, err)
			return
		}
		c.JSON(http.StatusOK, views.Assignment(a))

	case req.Status != nil:
		if models.AssignmentStatus(*req.Status) != models.AssignmentClosed {
			httpx.FieldError(c, http.StatusBadRequest, httpx.CodeBadRequest,
				"only \"closed\" is accepted", map[string]string{
					"status": "queued and assigned transitions are not exposed",
				})
			return
		}
		if !apiutil.AuthorizeConversation(c, h.svc, policy.AssignmentsClose, convID) {
			return
		}
		a, err := h.svc.CloseAssignment(ctx, tenant, id)
		if err != nil {
			apiutil.Fail(c, err)
			return
		}
		c.JSON(http.StatusOK, views.Assignment(a))

	default:
		httpx.BadRequest(c, "assignee_actor_id or status is required")
	}
}
