package domain

import (
	"context"
	"errors"
	"time"

	"github.com/bitllow/sild/backend/internal/realtime"
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
	if in.ClientMsgID != nil && *in.ClientMsgID != "" {
		if existing, err := s.store.Messages().FindByClientMsgID(ctx, tenantID, convID, *in.ClientMsgID); err == nil {
			return existing, nil
		} else if !errors.Is(err, store.ErrNotFound) {
			return nil, err
		}
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
		Attachments: atts,
	}
	if err := s.store.Messages().Create(ctx, msg); err != nil {
		return nil, err
	}

	_ = applyMessageActivity(ctx, s.store, msg)

	data := views.Message(msg, s.attachmentURLFunc())
	// Surface the agent's display name so end-user surfaces (the web widget)
	// render the operator's first name instead of a generic "Support".
	if msg.InternalActorID != nil {
		if name := s.AgentDisplayName(ctx, tenantID, *msg.InternalActorID); name != "" {
			data["author_name"] = name
		}
	}
	if in.Visibility == models.VisibilityInternal {
		// internal notes go ONLY to the agents-only channel (§5.6) — never
		// webhooked/pushed/emailed.
		s.emit(ctx, realtime.Target{Conversation: convID, Internal: true}, realtime.EventMessageCreated, convID, data)
	} else {
		tgt := realtime.Target{Conversation: convID}
		// peer_access operators observe this channel; they aren't conversation members.
		if conv != nil && conv.Kind == models.KindPeer {
			tgt.Peer = tenantID
		}
		s.emit(ctx, tgt, realtime.EventMessageCreated, convID, data)
		_ = s.fireWebhook(ctx, tenantID, convID, "message.created", data)
		s.maybeSendOutboundEmail(ctx, tenantID, convID, msg) // §6.2 outbound
	}
	return msg, nil
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
		u, err := s.bucket.SignGet(context.Background(), objectKey, 15*time.Minute)
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
