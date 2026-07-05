package api

import (
	"net/http"

	"github.com/bitllow/sild/backend/internal/apiutil"
	"github.com/bitllow/sild/backend/internal/domain"
	"github.com/bitllow/sild/backend/internal/httpx"
	"github.com/bitllow/sild/backend/internal/middleware"
	"github.com/bitllow/sild/backend/internal/store"
	"github.com/bitllow/sild/backend/internal/store/models"
	"github.com/bitllow/sild/backend/internal/views"
	"github.com/gin-gonic/gin"
)

// requirePeerAccess gates the peer-conversation surface on the operator's
// per-user peer_access flag (Settings → Team). Owner/admin have tenant-wide
// conversation access, but the peer surface is still their own opt-in.
func requirePeerAccess(c *gin.Context) bool {
	if p := middleware.Get(c); p != nil && p.PeerAccess {
		return true
	}
	httpx.Forbidden(c, "peer access is not enabled for this operator")
	return false
}

// listPeerConversations: GET /v1/admin/peer-conversations?role=&limit=&cursor=
// — the peer inbox list. Peer conversations (open, no assignment) are invisible
// to the queue endpoints, so this is their dedicated listing. Keyset-paginated by
// last activity, the same limits + infinite-scroll cursor as the queue; free-text
// search reuses GET /admin/search?peer=true.
func (h *Handler) listPeerConversations(c *gin.Context) {
	if !requirePeerAccess(c) {
		return
	}
	p := store.PeerParams{Role: c.Query("role"), Limit: atoiDefault(c.Query("limit"), 30)}
	if cur := c.Query("cursor"); cur != "" {
		cc, err := decodeQueueCursor(cur)
		if err != nil {
			httpx.BadRequest(c, "invalid cursor")
			return
		}
		p.Cursor = cc
	}
	page, err := h.svc.ListPeerConversations(c.Request.Context(), apiutil.Tenant(c), p)
	if err != nil {
		apiutil.Fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"conversations": page.Conversations,
		"next_cursor":   encodeQueueCursor(page.NextCursor),
		"has_more":      page.HasMore,
	})
}

// postPeerMessage: POST /v1/admin/peer-conversations/:id/messages — an agent
// message into a peer conversation, implicitly joining on the first send
// (adds the agent participant + a system join-note). Separate from the support
// send path precisely because of that implicit join.
func (h *Handler) postPeerMessage(c *gin.Context) {
	if !requirePeerAccess(c) {
		return
	}
	convID := c.Param("id")
	if !apiutil.AuthorizeConversation(c, h.svc, convID) {
		return
	}
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
	var atts []domain.AttachmentInput
	for _, a := range req.Attachments {
		_ = h.svc.CompleteUpload(c.Request.Context(), tenant, a.ObjectKey)
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
