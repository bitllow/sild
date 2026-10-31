package domain

import (
	"context"
	"encoding/json"
	"gorm.io/datatypes"

	"github.com/bitllow/sild/backend/internal/id"
	"github.com/bitllow/sild/backend/internal/realtime"
	"github.com/bitllow/sild/backend/internal/storage"
	"github.com/bitllow/sild/backend/internal/store"
	"github.com/bitllow/sild/backend/internal/store/models"
	"github.com/bitllow/sild/backend/internal/views"
)

// AttachmentInput references a completed upload (§4.2).
type AttachmentInput struct {
	ObjectKey   string
	Disposition models.Disposition
}

// SendInput is a message to create.
type SendInput struct {
	SenderKind  models.SenderKind
	External    *string
	Internal    *string
	Body        string
	Visibility  models.Visibility
	Channel     models.Channel
	ClientMsgID *string
	Attachments []AttachmentInput
	// AllowInternal is true when the caller is an agent (admin/ingress); only
	// then may Visibility be internal (§4.2, §5.6).
	AllowInternal bool
	// Parts are structured blocks; partsBySender says which a sender may attach.
	Parts json.RawMessage
	// Conv, when set, is the caller's already-read conversation, so SendMessage can
	// apply the peer rules without reading the row again. Nil = read it here.
	Conv *models.Conversation
}

// SendMessage appends a message to a conversation with idempotency, visibility
// enforcement, and attachment validation. Emits message.created (§5.3) on the
// correct channel and a webhook (participants only).
func (s *Service) SendMessage(ctx context.Context, tenantID, convID string, in SendInput) (*models.Message, error) {
	if in.External == nil && in.Internal == nil {
		return nil, invalid("a sender identity is required")
	}
	// Checked before the internal gate below, which only recognises the exact
	// value: an unknown one would slip past it and become a record no participant
	// can read but webhooks still fire for.
	if !in.Visibility.Valid() {
		return nil, invalid("visibility must be participants or internal")
	}
	if in.Visibility == "" {
		in.Visibility = models.VisibilityParticipants
	}
	if !in.Channel.Valid() {
		return nil, invalid("channel must be app or email")
	}
	if in.Channel == "" {
		in.Channel = models.ChannelApp
	}
	if in.Visibility == models.VisibilityInternal && !in.AllowInternal {
		return nil, ErrForbidden // only agents may post internal notes
	}
	var parts json.RawMessage
	if len(in.Parts) > 0 {
		var err error
		if parts, err = validateParts(in.Parts, in.SenderKind); err != nil {
			return nil, err
		}
	}

	// Resolved once and reused below: a closed peer conversation is read-only, and a
	// peer conversation also fans out to the tenant peer channel.
	conv := in.Conv
	if conv == nil {
		conv, _ = s.store.Conversations().Get(ctx, tenantID, convID)
	}
	if conv != nil && conv.Kind == models.KindPeer && conv.Status == models.ConversationClosed {
		return nil, ErrForbidden
	}

	// Idempotency (§4.2): a repeat client_msg_id returns the original.
	if existing, ok := s.findByClientMsgID(ctx, tenantID, convID, in.ClientMsgID); ok {
		return existing, nil
	}

	atts, err := s.resolveAttachments(ctx, tenantID, in.Attachments)
	if err != nil {
		return nil, err
	}

	msg := &models.Message{
		TenantID: tenantID, ConversationID: convID,
		SenderKind: in.SenderKind, Visibility: in.Visibility, Channel: in.Channel,
		ExternalUserID: in.External, InternalActorID: in.Internal,
		Body: in.Body, ClientMsgID: in.ClientMsgID, CreatedAt: s.now(),
		Attachments: atts, Parts: datatypes.JSON(parts),
	}
	// internal notes go ONLY to agent channels (§5.6) — never webhooked/pushed/emailed.
	internal := in.Visibility == models.VisibilityInternal

	// The view is built before the transaction opens: signing an attachment URL
	// can be a network round-trip, and holding the message row's lock across it
	// would put a bucket's latency inside every send.
	msg.ID = id.New(id.Message)
	data := views.MessageFull(msg, s.attachmentURLFunc())
	// Surface the agent's display name so end-user surfaces (the web widget)
	// render the operator's first name instead of a generic "Support".
	if msg.InternalActorID != nil {
		if name := s.AgentDisplayName(ctx, tenantID, *msg.InternalActorID); name != "" {
			data["author_name"] = name
		}
	}

	// Read once for the webhook gate and requeueOnReply.
	var queue *models.Queue
	if !internal && conv != nil {
		queue = convQueue(ctx, s.store, conv)
	}

	// The message and everything queued off it commit together: a crash between
	// the insert and the enqueue would otherwise drop the notification and the
	// webhook, weakening the at-least-once guarantee both document.
	err = s.store.Tx(ctx, func(tx store.Store) error {
		if err := tx.Messages().Create(ctx, msg); err != nil {
			return err
		}
		_ = applyMessageActivity(ctx, tx, msg)
		if internal {
			return nil
		}
		if err := s.enqueuePush(ctx, tx, msg); err != nil {
			return err
		}
		if !sendsWebhooks(queue) {
			return nil
		}
		member, _ := views.WithoutInternalParts(data)
		return s.writeWebhook(ctx, tx, tenantID, convID, "message.created", member)
	})
	if err != nil {
		// A concurrent retry of the same client_msg_id lost the race to the unique
		// index; the winner's message is the answer both callers must get.
		if existing, ok := s.findByClientMsgID(ctx, tenantID, convID, in.ClientMsgID); ok {
			return existing, nil
		}
		return nil, err
	}

	s.emitMessage(ctx, tenantID, convID, conv, internal, realtime.EventMessageCreated, data)
	if !internal {
		s.requeueOnReply(ctx, tenantID, conv, queue, msg)
		s.maybeSendOutboundEmail(ctx, tenantID, convID, msg) // §6.2 outbound
	}
	return msg, nil
}

// findByClientMsgID resolves an idempotency key to the message it already
// created. A lookup failure other than not-found is treated as "no original":
// the caller is either about to insert (and will hit the unique index) or
// already has the insert error worth reporting.
func (s *Service) findByClientMsgID(ctx context.Context, tenantID, convID string, key *string) (*models.Message, bool) {
	if key == nil || *key == "" {
		return nil, false
	}
	existing, err := s.store.Messages().FindByClientMsgID(ctx, tenantID, convID, *key)
	if err != nil {
		return nil, false
	}
	return existing, true
}

// applyMessageActivity maintains the denormalized last-activity (timestamp +
// preview) the inbox queue sorts and renders on. It's the single definition every
// message-ingress path calls right after creating a message — SendMessage and the
// inbound-email paths — so no ingress can leave the queue ordering stale. Mirrors
// the UI preview rule: participant-visible, non-system. `st` may be a transaction.
func applyMessageActivity(ctx context.Context, st store.Store, msg *models.Message) error {
	if msg.Visibility != models.VisibilityParticipants || msg.SenderKind == models.SenderSystem {
		return nil
	}
	return st.Conversations().TouchLastMessage(ctx, msg.TenantID, msg.ConversationID, msg.CreatedAt, previewText(msg.Body))
}

// previewText trims a message body to a short, single-line snippet for the inbox
// queue row (the column is sized at 512; we keep it well under that).
func previewText(body string) string {
	const max = 280
	r := []rune(body)
	if len(r) > max {
		return string(r[:max])
	}
	return string(r)
}

// resolveAttachments validates each object_key against a completed upload owned
// by the tenant (review finding), copying its mime/size/filename.
func (s *Service) resolveAttachments(ctx context.Context, tenantID string, in []AttachmentInput) ([]models.MessageAttachment, error) {
	out := make([]models.MessageAttachment, 0, len(in))
	for _, a := range in {
		up, err := s.store.Uploads().GetByObjectKey(ctx, tenantID, a.ObjectKey)
		if err != nil {
			return nil, invalid("unknown attachment object_key")
		}
		if up.Status != models.UploadCompleted {
			return nil, invalid("attachment upload not completed")
		}
		disp := a.Disposition
		if disp == "" {
			disp = models.DispositionAttachment
		}
		out = append(out, models.MessageAttachment{
			TenantID: tenantID, Disposition: disp, ObjectKey: up.ObjectKey,
			MimeType: up.MimeType, SizeBytes: up.SizeBytes, Filename: up.Filename,
		})
	}
	return out, nil
}

// ListMessagesBefore returns a history page (§4.2). includeInternal hides
// internal notes from non-agents (§5.6).
func (s *Service) ListMessagesBefore(ctx context.Context, tenantID, convID, before string, limit int, includeInternal bool) (*store.MessagePage, error) {
	return s.store.Messages().ListBefore(ctx, tenantID, convID, before, limit, includeInternal)
}

// attachmentURLFunc returns a resolver that mints short-lived download URLs.
func (s *Service) attachmentURLFunc() views.URLFunc {
	return func(objectKey string) string {
		if s.bucket == nil {
			return ""
		}
		u, err := s.bucket.SignGet(context.Background(), objectKey, storage.SignTTL)
		if err != nil {
			return ""
		}
		return u
	}
}

// CatchUpMessages returns messages after an id, oldest-first, bounded. hasMore
// tells the caller to re-issue with the last id received — without it a client
// that missed more than one page loses the remainder silently.
func (s *Service) CatchUpMessages(ctx context.Context, tenantID, convID, since string, limit int, includeInternal bool) ([]models.Message, bool, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	msgs, err := s.store.Messages().ListAfter(ctx, tenantID, convID, since, limit+1, includeInternal)
	if err != nil {
		return nil, false, err
	}
	if len(msgs) > limit {
		return msgs[:limit], true, nil
	}
	return msgs, false, nil
}

// emitMessage publishes a message event rendered in full. Internal step fields
// never reach a conversation channel: members get a stripped copy, operators the full one.
func (s *Service) emitMessage(ctx context.Context, tenantID, convID string, conv *models.Conversation, internal bool, eventType string, data map[string]any) {
	member, stripped := views.WithoutInternalParts(data)
	if internal || !stripped {
		s.emitObserved(ctx, realtime.Target{Conversation: convID, Internal: internal}, tenantID, conv, eventType, data)
		return
	}
	s.emit(ctx, realtime.Target{Conversation: convID}, eventType, convID, member)
	s.emitObserved(ctx, realtime.Target{Conversation: convID, Internal: true}, tenantID, conv, eventType, data)
}

// UpdateMessageParts replaces the parts of a message its author sent, while
// the conversation is open.
func (s *Service) UpdateMessageParts(ctx context.Context, tenantID, convID, msgID, actorID string, raw json.RawMessage) (map[string]any, error) {
	msg, err := s.store.Messages().Get(ctx, tenantID, msgID)
	if err != nil || msg.ConversationID != convID {
		return nil, ErrNotFound
	}
	if msg.SenderKind != models.SenderBot || msg.InternalActorID == nil || *msg.InternalActorID != actorID {
		return nil, ErrForbidden
	}
	conv, err := s.store.Conversations().Get(ctx, tenantID, convID)
	if err != nil {
		return nil, mapStoreErr(err)
	}
	if conv.Status == models.ConversationClosed {
		return nil, conflict(CodeConversationClosed, "the conversation is closed")
	}
	parts, err := validateParts(raw, models.SenderBot)
	if err != nil {
		return nil, err
	}
	now := s.now()
	if err := s.store.Messages().UpdateParts(ctx, tenantID, msgID, parts, now); err != nil {
		return nil, mapStoreErr(err)
	}
	msg.Parts, msg.UpdatedAt = datatypes.JSON(parts), &now
	data := views.MessageFull(msg, s.attachmentURLFunc())
	if name := s.AgentDisplayName(ctx, tenantID, actorID); name != "" {
		data["author_name"] = name
	}
	s.emitMessage(ctx, tenantID, convID, conv, msg.Visibility == models.VisibilityInternal, realtime.EventMessageUpdated, data)
	return data, nil
}

// requeueOnReply re-queues a conversation whose assignment was closed when its
// client writes again, in queues that ask for it. The support queue never does.
func (s *Service) requeueOnReply(ctx context.Context, tenantID string, conv *models.Conversation, queue *models.Queue, msg *models.Message) {
	if msg.SenderKind == models.SenderBot || (msg.SenderKind == models.SenderSystem && !hasApproval(msg.Parts)) {
		return
	}
	if conv == nil || conv.Status != models.ConversationOpen || queue == nil || !queue.RequeueOnReply {
		return
	}
	if !s.fromClient(ctx, tenantID, conv.ID, msg) {
		return
	}
	if a, err := s.store.Assignments().GetByConversation(ctx, tenantID, conv.ID); err == nil && a.Status != models.AssignmentClosed {
		return
	}
	_, _ = s.AddAssignment(ctx, tenantID, conv.ID)
}

// fromClient reports whether a message's sender is a member in the client role.
func (s *Service) fromClient(ctx context.Context, tenantID, convID string, msg *models.Message) bool {
	members, err := s.store.Members().ListActive(ctx, tenantID, convID)
	if err != nil {
		return false
	}
	for _, m := range members {
		if m.ConvRole != models.RoleClient {
			continue
		}
		if (msg.ExternalUserID != nil && m.ExternalUserID != nil && *m.ExternalUserID == *msg.ExternalUserID) ||
			(msg.InternalActorID != nil && m.InternalActorID != nil && *m.InternalActorID == *msg.InternalActorID) {
			return true
		}
	}
	return false
}
