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
	"github.com/bitllow/sild/backend/internal/storage"
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
	expansion, ok := apiutil.ParseExpand(c, expandContacts)
	if !ok {
		return
	}
	conv, members, assignment, err := h.svc.GetConversation(c.Request.Context(), apiutil.Tenant(c), convID)
	if err != nil {
		apiutil.Fail(c, err)
		return
	}
	load := h.svc.MemberNames
	if wantsProfiles(expansion) {
		load = h.svc.MemberProfiles
	}
	profiles, err := load(c.Request.Context(), apiutil.Tenant(c), members)
	if err != nil {
		apiutil.Fail(c, err)
		return
	}
	view := views.Conversation(conv, members, assignment, profiles)
	if expansion.Has(resourceContacts) {
		view[resourceContacts] = contactsBlock(
			domain.ExternalParticipants(members), profiles.Names, profiles.Contacts, expansion)
	}
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
	// Read once: it selects the send path below and is reused as in.Conv.
	conv, _ := h.svc.Conversation(c.Request.Context(), apiutil.Tenant(c), convID)
	// An operator sending into a peer conversation implicitly joins it. That is a
	// property of the data, not of the URL, so it fires on the shared route.
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
		Parts json.RawMessage `json:"parts"`
	}
	if !httpx.DecodeJSON(c, &req) {
		return
	}
	// A shared URL is not a shared body: only an API key may name the sender.
	// Dropping these silently is how a client comes to believe it posted as an
	// agent when it did not.
	if p := middleware.Get(c); p != nil && p.Kind != principal.KindAPIKey {
		if !httpx.RejectFields(c, map[string]bool{
			"sender_kind":       req.SenderKind != "",
			"internal_actor_id": req.InternalActorID != "",
			"external_user_id":  req.ExternalUserID != "",
		}, "sender_kind", "internal_actor_id", "external_user_id") {
			return
		}
	}

	in := domain.SendInput{
		Body: req.Body, Visibility: req.Visibility, Channel: req.Channel,
		AllowInternal: apiutil.InternalOn(c, h.svc, policy.NotesWrite, convID), Conv: conv, Parts: req.Parts,
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
		if p.Service {
			in.SenderKind = models.SenderBot
		}
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
	// Only an assistant sends parts, so the full form reaches only their author.
	out := views.MessageFull(msg, h.attachmentURL(c))
	if msg.InternalActorID != nil {
		if name := h.svc.AgentDisplayName(c.Request.Context(), apiutil.Tenant(c), *msg.InternalActorID); name != "" {
			out["author_name"] = name
		}
	}
	c.JSON(http.StatusCreated, out)
}

// listMessages: GET /v1/conversations/:id/messages
//
//	?cursor=  pagination — newest-first, bounded by limit
//	?since=   catch-up   — oldest-first, everything after a message id (§5.4)
//
// ?since= is a sync read, not a page: continuation is the last message id
// received, because the id IS the position.
func (h *Handler) listMessages(c *gin.Context) {
	convID := c.Param("id")
	if !apiutil.AuthorizeConversation(c, h.svc, policy.MessagesRead, convID) {
		return
	}
	includeInternal := apiutil.InternalOn(c, h.svc, policy.NotesRead, convID)
	urlFn := h.attachmentURL(c)
	ctx, tenant := c.Request.Context(), apiutil.Tenant(c)

	page, ok := apiutil.PageParams(c, apiutil.PageDefaults{
		Resource:   resourceMessages,
		Limit:      50,
		Sort:       store.SortID,
		Order:      store.OrderDesc,
		FixedOrder: true,
	})
	if !ok {
		return
	}

	// §12 read fallback: an archived conversation's history lives in the sink, and
	// pages through the same wire contract.
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
			Items:   h.renderMessagesAuthored(c, msgs, urlFn, includeInternal),
			HasMore: hasMore,
		})
		return
	}

	res, err := h.svc.ListMessagesBefore(ctx, tenant, convID, cursorID(page), page.Limit, includeInternal)
	if err != nil {
		apiutil.Fail(c, err)
		return
	}
	rendered := h.renderMessagesAuthored(c, res.Messages, urlFn, includeInternal)
	out := store.Page[map[string]any]{Items: rendered, HasMore: res.HasMore}
	if res.HasMore && len(res.Messages) > 0 {
		out.NextCursor = &store.Cursor{
			Key: store.SortID, Order: page.Order, ID: res.Messages[0].ID,
		}
	}
	apiutil.RespondPage(c, resourceMessages, out)
}

// cursorID is the keyset position ("" on the first page).
func cursorID(p store.PageParams) string {
	if p.Cursor == nil {
		return ""
	}
	return p.Cursor.ID
}

// archivedBefore mirrors messageRepo.ListBefore over a rehydrated archive. The
// sink stores messages ascending, so store.SlicePage would read the wrong end.
func archivedBefore(msgs []map[string]any, before string, limit int) store.Page[map[string]any] {
	end := len(msgs)
	if before != "" {
		end = 0
		for i := range msgs {
			if mapID(&msgs[i]) >= before {
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
			Key: store.SortID, Order: store.OrderDesc, ID: mapID(&window[0]),
		}
	}
	return page
}

// archivedSince is the catch-up read over a rehydrated archive.
func archivedSince(msgs []map[string]any, since string, limit int) store.Page[map[string]any] {
	out := make([]map[string]any, 0, limit)
	for i := range msgs {
		if mapID(&msgs[i]) > since {
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
	if !httpx.DecodeJSON(c, &req) {
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
	h.svc.Typing(c.Request.Context(), p.TenantID, convID, userID)
	c.Status(http.StatusNoContent)
}

// closeConversation: POST /v1/conversations/:id/close. Agent/key only (§1).
func (h *Handler) closeConversation(c *gin.Context) {
	if !apiutil.AuthorizeConversation(c, h.svc, policy.ConversationsClose, c.Param("id")) {
		return
	}
	var req struct {
		Reason string `json:"reason"`
	}
	if !httpx.DecodeJSONOptional(c, &req) {
		return
	}
	if err := h.svc.CloseConversation(c.Request.Context(), apiutil.Tenant(c), c.Param("id"), req.Reason); err != nil {
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
	if !httpx.DecodeJSON(c, &req) {
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
		u, err := h.bucket.SignGet(c.Request.Context(), objectKey, storage.SignTTL)
		if err != nil {
			return ""
		}
		return u
	}
}

// renderMessagesAuthored renders a page of messages and stamps agent-authored
// ones with the operator's display name (author_name), so the web widget shows a
// real first name in place of "Support". Names are resolved once per distinct
// actor per page.
func (h *Handler) renderMessagesAuthored(c *gin.Context, msgs []models.Message, urlFn views.URLFunc, full bool) []map[string]any {
	render := views.Message
	if full {
		render = views.MessageFull
	}
	out := make([]map[string]any, 0, len(msgs))
	for i := range msgs {
		out = append(out, render(&msgs[i], urlFn))
	}
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

// patchMessageParts: PATCH /v1/conversations/:id/messages/:mid. The assistant
// edits the parts of its own message, e.g. a step finishing.
func (h *Handler) patchMessageParts(c *gin.Context) {
	convID := c.Param("id")
	if !apiutil.AuthorizeConversation(c, h.svc, policy.MessagesSend, convID) {
		return
	}
	var req struct {
		Parts json.RawMessage `json:"parts"`
	}
	if !httpx.DecodeJSON(c, &req) {
		return
	}
	p := middleware.Get(c)
	if !p.Service {
		httpx.Forbidden(c, "only an assistant edits message parts")
		return
	}
	out, err := h.svc.UpdateMessageParts(c.Request.Context(), apiutil.Tenant(c), convID, c.Param("mid"), p.AdminID, req.Parts)
	if err != nil {
		apiutil.Fail(c, err)
		return
	}
	c.JSON(http.StatusOK, out)
}

// approveDataAccess: POST /v1/conversations/:id/approvals. The requester tells
// the assistant they approved data access at the integrator.
func (h *Handler) approveDataAccess(c *gin.Context) {
	convID := c.Param("id")
	if !apiutil.AuthorizeConversation(c, h.svc, policy.MessagesSend, convID) {
		return
	}
	msg, err := h.svc.ApproveDataAccess(c.Request.Context(), apiutil.Tenant(c), convID, middleware.Get(c).AdminID)
	if err != nil {
		apiutil.Fail(c, err)
		return
	}
	c.JSON(http.StatusCreated, views.Message(msg, h.attachmentURL(c)))
}

// mintRequesterAssertion: POST /v1/conversations/:id/requester-assertions. The
// body is the exact data-adapter request the assistant is about to send.
func (h *Handler) mintRequesterAssertion(c *gin.Context) {
	convID := c.Param("id")
	if !apiutil.AuthorizeConversation(c, h.svc, policy.MessagesSend, convID) {
		return
	}
	p := middleware.Get(c)
	if !p.Service {
		httpx.Forbidden(c, "only an assistant asks for requester assertions")
		return
	}
	var req struct {
		Body string `json:"body"`
	}
	if !httpx.DecodeJSON(c, &req) {
		return
	}
	assertion, exp, err := h.svc.MintRequesterAssertion(c.Request.Context(), apiutil.Tenant(c), convID, p.AdminID, []byte(req.Body))
	if err != nil {
		apiutil.Fail(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"assertion": assertion, "expires_at": exp})
}
