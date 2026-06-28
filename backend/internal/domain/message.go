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
}

// SendMessage appends a message to a conversation with idempotency, visibility
// enforcement, and attachment validation. Emits message.created (§5.3) on the
// correct channel and a webhook (participants only).
func (s *Service) SendMessage(ctx context.Context, tenantID, convID string, in SendInput) (*models.Message, error) {
	if in.External == nil && in.Internal == nil {
		return nil, invalid("a sender identity is required")
	}
	if in.Visibility == "" {
		in.Visibility = models.VisibilityParticipants
	}
	if in.Channel == "" {
		in.Channel = models.ChannelApp
	}
	if in.Visibility == models.VisibilityInternal && !in.AllowInternal {
		return nil, ErrForbidden // only agents may post internal notes
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

	data := views.Message(msg, s.attachmentURLFunc())
	if in.Visibility == models.VisibilityInternal {
		// internal notes go ONLY to the agents-only channel (§5.6) — never
		// webhooked/pushed/emailed.
		s.emit(ctx, realtime.Target{Conversation: convID, Internal: true}, realtime.EventMessageCreated, convID, data)
	} else {
		s.emit(ctx, realtime.Target{Conversation: convID}, realtime.EventMessageCreated, convID, data)
		_ = s.fireWebhook(ctx, tenantID, convID, "message.created", data)
		s.maybeSendOutboundEmail(ctx, tenantID, convID, msg) // §6.2 outbound
	}
	return msg, nil
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

// ListMessagesAfter returns reconnect catch-up messages (§4.2, §5.4).
func (s *Service) ListMessagesAfter(ctx context.Context, tenantID, convID, after string, includeInternal bool) ([]models.Message, error) {
	return s.store.Messages().ListAfter(ctx, tenantID, convID, after, includeInternal)
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
