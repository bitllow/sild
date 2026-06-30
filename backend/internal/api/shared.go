package api

import (
	"errors"
	"net/http"
	"time"

	"github.com/bitllow/sild/backend/internal/apiutil"
	"github.com/bitllow/sild/backend/internal/domain"
	"github.com/bitllow/sild/backend/internal/httpx"
	"github.com/bitllow/sild/backend/internal/middleware"
	"github.com/bitllow/sild/backend/internal/store/models"
	"github.com/bitllow/sild/backend/internal/views"
	"github.com/gin-gonic/gin"
)

// getConversation: GET /v1/conversations/:id (§4.1/§4.2 shared).
func (h *Handler) getConversation(c *gin.Context) {
	convID := c.Param("id")
	if !apiutil.AuthorizeConversation(c, h.svc, convID) {
		return
	}
	conv, members, assignment, err := h.svc.GetConversation(c.Request.Context(), apiutil.Tenant(c), convID)
	if errors.Is(err, domain.ErrNotFound) {
		// §12 read fallback: hot rows gone → read from the archive sink.
		if view, archived, aerr := h.svc.ArchivedConversation(c.Request.Context(), apiutil.Tenant(c), convID); archived {
			if aerr != nil {
				apiutil.Fail(c, aerr)
				return
			}
			c.JSON(http.StatusOK, view)
			return
		}
	}
	if err != nil {
		apiutil.Fail(c, err)
		return
	}
	view := views.Conversation(conv, members, assignment)
	// Email conversations carry a subject (the inbox renders it instead of the
	// opaque conversation id); app conversations have none.
	if subject := h.svc.EmailSubject(c.Request.Context(), apiutil.Tenant(c), convID); subject != "" {
		view["subject"] = subject
	}
	c.JSON(http.StatusOK, view)
}

// postMessage: POST /v1/conversations/:id/messages (ingress §4.1, send §4.2).
func (h *Handler) postMessage(c *gin.Context) {
	convID := c.Param("id")
	if !apiutil.AuthorizeConversation(c, h.svc, convID) {
		return
	}
	var req struct {
		Body            string            `json:"body"`
		ClientMsgID     string            `json:"client_msg_id"`
		Visibility      models.Visibility `json:"visibility"`
		Channel         models.Channel    `json:"channel"`
		SenderKind      models.SenderKind `json:"sender_kind"`
		InternalActorID string            `json:"internal_actor_id"`
		ExternalUserID  string            `json:"external_user_id"`
		Attachments     []struct {
			ObjectKey   string             `json:"object_key"`
			Disposition models.Disposition `json:"disposition"`
		} `json:"attachments"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.BadRequest(c, "invalid body")
		return
	}

	in := domain.SendInput{
		Body: req.Body, Visibility: req.Visibility, Channel: req.Channel,
		AllowInternal: apiutil.IsAgent(c),
	}
	if req.ClientMsgID != "" {
		in.ClientMsgID = &req.ClientMsgID
	}
	p := middleware.Get(c)
	switch p.Kind {
	case middleware.KindUser:
		in.SenderKind = models.SenderUser
		uid := p.Subject
		in.External = &uid
	case middleware.KindAdmin:
		in.SenderKind = models.SenderAgent
		aid := p.AdminID
		in.Internal = &aid
	default: // API key ingress — sender comes from the body
		in.SenderKind = req.SenderKind
		if in.SenderKind == "" {
			in.SenderKind = models.SenderAgent
		}
		if req.InternalActorID != "" {
			v := req.InternalActorID
			in.Internal = &v
		}
		if req.ExternalUserID != "" {
			v := req.ExternalUserID
			in.External = &v
		}
	}

	// Lazily mark referenced uploads complete, then attach (validated in domain).
	for _, a := range req.Attachments {
		_ = h.svc.CompleteUpload(c.Request.Context(), apiutil.Tenant(c), a.ObjectKey)
		in.Attachments = append(in.Attachments, domain.AttachmentInput{ObjectKey: a.ObjectKey, Disposition: a.Disposition})
	}

	msg, err := h.svc.SendMessage(c.Request.Context(), apiutil.Tenant(c), convID, in)
	if err != nil {
		apiutil.Fail(c, err)
		return
	}
	c.JSON(http.StatusCreated, views.Message(msg, h.attachmentURL(c)))
}

// listMessages: GET /v1/conversations/:id/messages?before=&after=&limit= (§4.2).
func (h *Handler) listMessages(c *gin.Context) {
	convID := c.Param("id")
	if !apiutil.AuthorizeConversation(c, h.svc, convID) {
		return
	}
	includeInternal := apiutil.IsAgent(c)
	urlFn := h.attachmentURL(c)

	// §12 read fallback: if the conversation has been archived, read from the
	// sink (hot rows are gone). Rare — the inbox only touches open conversations.
	if msgs, archived, err := h.svc.ArchivedMessages(c.Request.Context(), apiutil.Tenant(c), convID, includeInternal); archived {
		if err != nil {
			apiutil.Fail(c, err)
			return
		}
		c.JSON(http.StatusOK, gin.H{"messages": msgs, "has_more": false})
		return
	}

	if after := c.Query("after"); after != "" {
		msgs, err := h.svc.ListMessagesAfter(c.Request.Context(), apiutil.Tenant(c), convID, after, includeInternal)
		if err != nil {
			apiutil.Fail(c, err)
			return
		}
		c.JSON(http.StatusOK, gin.H{"messages": renderMessages(msgs, urlFn)})
		return
	}
	limit := atoiDefault(c.Query("limit"), 50)
	page, err := h.svc.ListMessagesBefore(c.Request.Context(), apiutil.Tenant(c), convID, c.Query("before"), limit, includeInternal)
	if err != nil {
		apiutil.Fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"messages": renderMessages(page.Messages, urlFn), "has_more": page.HasMore})
}

// markRead: POST /v1/conversations/:id/read (§4.2).
func (h *Handler) markRead(c *gin.Context) {
	convID := c.Param("id")
	if !apiutil.AuthorizeConversation(c, h.svc, convID) {
		return
	}
	var req struct {
		LastReadMessageID string `json:"last_read_message_id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.BadRequest(c, "invalid body")
		return
	}
	if err := h.svc.MarkRead(c.Request.Context(), apiutil.Tenant(c), convID, apiutil.CallerParticipant(c), req.LastReadMessageID); err != nil {
		apiutil.Fail(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

// typing: POST /v1/conversations/:id/typing (§4.2).
func (h *Handler) typing(c *gin.Context) {
	convID := c.Param("id")
	if !apiutil.AuthorizeConversation(c, h.svc, convID) {
		return
	}
	p := middleware.Get(c)
	userID := p.Subject
	if userID == "" {
		userID = p.AdminID
	}
	h.svc.Typing(c.Request.Context(), convID, userID)
	c.Status(http.StatusNoContent)
}

// closeConversation: POST /v1/conversations/:id/close. Agent/key only (§1).
func (h *Handler) closeConversation(c *gin.Context) {
	if !apiutil.IsAgent(c) {
		httpx.Forbidden(c, "only agents may close a conversation")
		return
	}
	if err := h.svc.CloseConversation(c.Request.Context(), apiutil.Tenant(c), c.Param("id")); err != nil {
		apiutil.Fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "closed"})
}

// issueUpload: POST /v1/uploads (§4.1/§4.2 shared).
func (h *Handler) issueUpload(c *gin.Context) {
	var req struct {
		MimeType  string `json:"mime_type"`
		SizeBytes int64  `json:"size_bytes"`
		Filename  string `json:"filename"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.BadRequest(c, "invalid body")
		return
	}
	signed, err := h.svc.IssueUpload(c.Request.Context(), apiutil.Tenant(c), domain.IssueUploadInput{
		MimeType: req.MimeType, SizeBytes: req.SizeBytes, Filename: req.Filename,
		Uploader: apiutil.CallerParticipant(c),
	})
	if err != nil {
		apiutil.Fail(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{
		"object_key": signed.ObjectKey, "upload_url": signed.UploadURL, "expires_at": signed.ExpiresAt,
	})
}

// attachmentURL returns a per-request signed-GET resolver for rendering.
func (h *Handler) attachmentURL(c *gin.Context) views.URLFunc {
	return func(objectKey string) string {
		if h.bucket == nil {
			return ""
		}
		u, err := h.bucket.SignGet(c.Request.Context(), objectKey, 15*time.Minute)
		if err != nil {
			return ""
		}
		return u
	}
}

func renderMessages(msgs []models.Message, urlFn views.URLFunc) []map[string]any {
	out := make([]map[string]any, 0, len(msgs))
	for i := range msgs {
		out = append(out, views.Message(&msgs[i], urlFn))
	}
	return out
}

func atoiDefault(s string, def int) int {
	if s == "" {
		return def
	}
	n := 0
	for _, r := range s {
		if r < '0' || r > '9' {
			return def
		}
		n = n*10 + int(r-'0')
	}
	if n == 0 {
		return def
	}
	return n
}
