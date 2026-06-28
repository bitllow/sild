package domain

import (
	"context"

	"github.com/bitllow/sild/backend/internal/realtime"
	"github.com/bitllow/sild/backend/internal/store"
	"github.com/bitllow/sild/backend/internal/store/models"
)

// MarkRead upserts a monotonic read receipt and emits message.read (§4.2, §5.3).
// The store enforces monotonicity (a stale id is ignored, §3).
func (s *Service) MarkRead(ctx context.Context, tenantID, convID string, p store.Participant, lastReadMessageID string) error {
	if lastReadMessageID == "" {
		return invalid("last_read_message_id is required")
	}
	rr := &models.ReadReceipt{
		TenantID: tenantID, ConversationID: convID,
		ParticipantKind: p.Kind, ExternalUserID: p.ExternalUserID, InternalActorID: p.InternalActorID,
		LastReadMessageID: lastReadMessageID, UpdatedAt: s.now(),
	}
	if err := s.store.Receipts().Upsert(ctx, rr); err != nil {
		return err
	}
	user := derefStr(p.ExternalUserID)
	s.emit(ctx, realtime.Target{Conversation: convID}, realtime.EventMessageRead, convID,
		map[string]any{"user_id": user, "last_read_message_id": lastReadMessageID})
	return nil
}

// Typing fans out a transient typing event (§4.2, §5.3). Throttling is enforced
// at the realtime layer.
func (s *Service) Typing(ctx context.Context, convID, userID string) {
	s.emit(ctx, realtime.Target{Conversation: convID}, realtime.EventTyping, convID,
		map[string]any{"user_id": userID})
}
