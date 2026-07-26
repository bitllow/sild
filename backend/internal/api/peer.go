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
		Body        string            `json:"body"`
		ClientMsgID string            `json:"client_msg_id"`
		Visibility  models.Visibility `json:"visibility"`
		Attachments []struct {
			ObjectKey   string `json:"object_key"`
			Disposition string `json:"disposition"`
		} `json:"attachments"`
	}
	if !httpx.DecodeJSON(c, &req) {
		return
	}
	// One URL, one body: the shared send path takes visibility, so this branch must
	// accept it too. PeerAgentSend does not carry the field, so an unchecked value
	// would be dropped and the message sent participant-visible regardless.
	//
	// Peer conversations carry no internal side. Refused rather than downgraded, or an
	// operator would believe they left a private note in a chat the end user reads.
	if !req.Visibility.Valid() || req.Visibility == models.VisibilityInternal {
		httpx.FieldError(c, http.StatusBadRequest, httpx.CodeBadRequest,
			"a peer conversation has no internal side",
			map[string]string{"visibility": "only \"participants\" is accepted here"})
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
