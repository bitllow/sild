package api

import (
	"net/http"

	"github.com/bitllow/sild/backend/internal/apiutil"
	"github.com/bitllow/sild/backend/internal/domain"
	"github.com/bitllow/sild/backend/internal/httpx"
	"github.com/bitllow/sild/backend/internal/middleware"
	"github.com/bitllow/sild/backend/internal/store/models"
	"github.com/bitllow/sild/backend/internal/views"
	"github.com/gin-gonic/gin"
)

// postPeerMessage: POST /v1/admin/peer-conversations/:id/messages — an agent
// message into a peer conversation, implicitly joining on the first send
// (adds the agent participant + a system join-note). Separate from the support
// send path precisely because of that implicit join.
func (h *Handler) postPeerMessage(c *gin.Context) {
	convID := c.Param("id")
	var req struct {
		Body        string `json:"body"`
		ClientMsgID string `json:"client_msg_id"`
		Attachments []struct {
			ObjectKey   string `json:"object_key"`
			Disposition string `json:"disposition"`
		} `json:"attachments"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.BadRequest(c, "invalid body")
		return
	}
	tenant := apiutil.Tenant(c)
	// The uploads are finalized inside PeerAgentSend, AFTER it validates the send
	// (peer + open), so a rejected send leaves no orphaned committed uploads.
	var atts []domain.AttachmentInput
	for _, a := range req.Attachments {
		atts = append(atts, domain.AttachmentInput{ObjectKey: a.ObjectKey, Disposition: models.Disposition(a.Disposition)})
	}
	msg, err := h.svc.PeerAgentSend(c.Request.Context(), tenant, convID, middleware.Get(c).AdminID, req.Body, atts, req.ClientMsgID)
	if err != nil {
		apiutil.Fail(c, err)
		return
	}
	out := views.Message(msg, h.attachmentURL(c))
	if msg.InternalActorID != nil {
		if name := h.svc.AgentDisplayName(c.Request.Context(), tenant, *msg.InternalActorID); name != "" {
			out["author_name"] = name
		}
	}
	c.JSON(http.StatusCreated, out)
}
