package api

import (
	"net/http"
	"time"

	"github.com/bitllow/sild/backend/internal/apiutil"
	"github.com/bitllow/sild/backend/internal/domain"
	"github.com/bitllow/sild/backend/internal/httpx"
	"github.com/bitllow/sild/backend/internal/middleware"
	"github.com/bitllow/sild/backend/internal/policy"
	"github.com/bitllow/sild/backend/internal/principal"
	"github.com/bitllow/sild/backend/internal/store"
	"github.com/bitllow/sild/backend/internal/store/models"
	"github.com/bitllow/sild/backend/internal/views"
	"github.com/gin-gonic/gin"
)

// getConversation: GET /v1/conversations/:id (§4.1/§4.2 shared).
func (h *Handler) getConversation(c *gin.Context) {
	convID := c.Param("id")
	if !apiutil.AuthorizeConversation(c, h.svc, policy.ConversationsRead, convID) {
		return
	}
	conv, members, assignment, err := h.svc.GetConversation(c.Request.Context(), apiutil.Tenant(c), convID)
	if err != nil {
		apiutil.Fail(c, err)
		return
	}
	view := views.Conversation(conv, members, assignment)
	view["kind"] = conv.Kind
	// Archival keeps the conversation and its members, so the view is served from
	// live rows; only the message history moved to the sink. The flag tells a
	// client its history comes from cold storage.
	if conv.ArchivedAt != nil {
		view["archived"] = true
	}
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
	if !apiutil.AuthorizeConversation(c, h.svc, policy.MessagesSend, convID) {
		return
	}
	// Read once: it selects the send path below and is reused as in.Conv by
	// SendMessage.
	conv, _ := h.svc.Conversation(c.Request.Context(), apiutil.Tenant(c), convID)
	// An operator sending into a peer conversation implicitly joins it (adds the
	// agent participant + a system join-note). That is a property of the DATA —
	// operator + peer conversation — not of the URL the client chose, so it fires
	// here rather than behind a separate route.
	if p := middleware.Get(c); p != nil && p.Kind == principal.KindAdmin &&
		conv != nil && conv.Kind == models.KindPeer {
		h.postPeerMessage(c)
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
		AllowInternal: apiutil.IsAgent(c), Conv: conv,
	}
	if req.ClientMsgID != "" {
		in.ClientMsgID = &req.ClientMsgID
	}
	p := middleware.Get(c)
	switch p.Kind {
	case principal.KindUser:
		in.SenderKind = models.SenderUser
		uid := p.Subject
		in.External = &uid
	case principal.KindAdmin:
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
	out := views.Message(msg, h.attachmentURL(c))
	if msg.InternalActorID != nil {
		if name := h.svc.AgentDisplayName(c.Request.Context(), apiutil.Tenant(c), *msg.InternalActorID); name != "" {
			out["author_name"] = name
		}
	}
	c.JSON(http.StatusCreated, out)
}

// listMessages: GET /v1/conversations/:id/messages?before=&after=&limit= (§4.2).
// listMessages: GET /v1/conversations/:id/messages
//
// Two different reads, deliberately spelled differently:
//
//	?cursor=  pagination — newest-first, bounded by limit, standard envelope
//	?since=   catch-up   — oldest-first, everything after a message id
//
// ?since= is the realtime reconnect primitive (§5.4), NOT a page: it answers
// "give me everything I missed". Sharing one handler with ?before=/?after= is
// what produced the old asymmetry where one returned has_more and the other
// silently truncated at 500 with no way to tell.
//
// Continuation for ?since= is the message id itself — re-issue with the last id
// received while has_more is true. There is no cursor, because the id IS the
// position.
func (h *Handler) listMessages(c *gin.Context) {
	convID := c.Param("id")
	if !apiutil.AuthorizeConversation(c, h.svc, policy.MessagesRead, convID) {
		return
	}
	includeInternal := apiutil.IsAgent(c)
	urlFn := h.attachmentURL(c)
	ctx, tenant := c.Request.Context(), apiutil.Tenant(c)

	page, ok := apiutil.PageParams(c, apiutil.PageDefaults{
		Resource: resourceMessages,
		Limit:    50,
		Sort:     store.SortID,
		Order:    store.OrderDesc,
	})
	if !ok {
		return
	}

	// §12 read fallback: an archived conversation's history lives in the sink.
	// It pages through the same contract via store.SlicePage, so "every
	// collection" has no exception here.
	if msgs, archived, err := h.svc.ArchivedMessages(ctx, tenant, convID, includeInternal); archived {
		if err != nil {
			apiutil.Fail(c, err)
			return
		}
		if since := c.Query("since"); since != "" {
			apiutil.RespondCatchUp(c, archivedSince(msgs, since, page.Limit))
			return
		}
		apiutil.RespondPage(c, resourceMessages, archivedBefore(msgs, cursorID(page), page.Limit))
		return
	}

	if since := c.Query("since"); since != "" {
		msgs, hasMore, err := h.svc.CatchUpMessages(ctx, tenant, convID, since, page.Limit, includeInternal)
		if err != nil {
			apiutil.Fail(c, err)
			return
		}
		apiutil.RespondCatchUp(c, store.Page[map[string]any]{
			Items:   h.renderMessagesAuthored(c, msgs, urlFn),
			HasMore: hasMore,
		})
		return
	}

	res, err := h.svc.ListMessagesBefore(ctx, tenant, convID, cursorID(page), page.Limit, includeInternal)
	if err != nil {
		apiutil.Fail(c, err)
		return
	}
	rendered := h.renderMessagesAuthored(c, res.Messages, urlFn)
	out := store.Page[map[string]any]{Items: rendered, HasMore: res.HasMore}
	if res.HasMore && len(res.Messages) > 0 {
		out.NextCursor = &store.Cursor{
			Key: store.SortID, Order: page.Order, ID: res.Messages[0].ID,
		}
	}
	apiutil.RespondPage(c, resourceMessages, out)
}

// cursorID is the keyset position for message history ("" on the first page).
func cursorID(p store.PageParams) string {
	if p.Cursor == nil {
		return ""
	}
	return p.Cursor.ID
}

func archivedMessageID(m *map[string]any) string {
	id, _ := (*m)["id"].(string)
	return id
}

// archivedBefore mirrors messageRepo.ListBefore over a rehydrated archive: the
// newest `limit` messages strictly before the cursor, returned OLDEST-FIRST with
// the cursor set to the page's oldest id.
//
// The sink stores messages ascending (the archive job drains ListAfter), so a
// generic descending slicer would read the wrong end and re-serve page one
// forever. Matching the hot path exactly is what lets a client page an archived
// conversation without knowing it is archived.
func archivedBefore(msgs []map[string]any, before string, limit int) store.Page[map[string]any] {
	end := len(msgs)
	if before != "" {
		end = 0
		for i := range msgs {
			if archivedMessageID(&msgs[i]) >= before {
				break
			}
			end = i + 1
		}
	}
	window := msgs[:end]

	page := store.Page[map[string]any]{}
	if len(window) > limit {
		page.HasMore = true
		window = window[len(window)-limit:]
	}
	page.Items = window
	if page.HasMore && len(window) > 0 {
		page.NextCursor = &store.Cursor{
			Key: store.SortID, Order: store.OrderDesc, ID: archivedMessageID(&window[0]),
		}
	}
	return page
}

// archivedSince is the catch-up read over a rehydrated archive: everything after
// an id, oldest-first, bounded.
func archivedSince(msgs []map[string]any, since string, limit int) store.Page[map[string]any] {
	out := make([]map[string]any, 0, limit)
	for i := range msgs {
		if archivedMessageID(&msgs[i]) > since {
			out = append(out, msgs[i])
		}
	}
	page := store.Page[map[string]any]{}
	if len(out) > limit {
		page.HasMore = true
		out = out[:limit]
	}
	page.Items = out
	return page
}

// markRead: POST /v1/conversations/:id/read (§4.2).
func (h *Handler) markRead(c *gin.Context) {
	convID := c.Param("id")
	if !apiutil.AuthorizeConversation(c, h.svc, policy.ReceiptsWrite, convID) {
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
	if !apiutil.AuthorizeConversation(c, h.svc, policy.TypingWrite, convID) {
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
	// Being an agent isn't enough — closing needs the same scope check as reading.
	if !apiutil.AuthorizeConversation(c, h.svc, policy.ConversationsClose, c.Param("id")) {
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

// renderMessagesAuthored renders a page of messages and stamps agent-authored
// ones with the operator's display name (author_name), so the web widget shows a
// real first name in place of "Support". Names are resolved once per distinct
// actor per page.
func (h *Handler) renderMessagesAuthored(c *gin.Context, msgs []models.Message, urlFn views.URLFunc) []map[string]any {
	out := renderMessages(msgs, urlFn)
	tenant := apiutil.Tenant(c)
	cache := make(map[string]string)
	for i := range msgs {
		if msgs[i].InternalActorID == nil {
			continue
		}
		id := *msgs[i].InternalActorID
		name, ok := cache[id]
		if !ok {
			name = h.svc.AgentDisplayName(c.Request.Context(), tenant, id)
			cache[id] = name
		}
		if name != "" {
			out[i]["author_name"] = name
		}
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
