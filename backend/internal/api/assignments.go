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

// patchAssignment: PATCH /v1/assignments/:id. "me" and "closed" are the only
// accepted values, so this cannot become reassignment or an unexposed transition.
func (h *Handler) patchAssignment(c *gin.Context) {
	var req struct {
		AssigneeActorID *string `json:"assignee_actor_id"`
		Status          *string `json:"status"`
	}
	if !httpx.DecodeJSON(c, &req) {
		return
	}
	if req.AssigneeActorID != nil && req.Status != nil {
		httpx.BadRequest(c, "set assignee_actor_id or status, not both")
		return
	}

	ctx, tenant, id := c.Request.Context(), apiutil.Tenant(c), c.Param("id")

	// Authorize against the CONVERSATION: an assignment carries no access rules of
	// its own.
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
