package domain

import (
	"context"
	"errors"

	"github.com/bitllow/sild/backend/internal/store"
	"github.com/bitllow/sild/backend/internal/store/models"
	"github.com/bitllow/sild/backend/internal/views"
)

// IsMember reports whether a user is an active member of a conversation. This is
// the authorization check for every user endpoint and realtime channel (§4.2, §7).
func (s *Service) IsMember(ctx context.Context, tenantID, convID, userID string) (bool, error) {
	return s.store.Members().IsActiveMember(ctx, tenantID, convID, userID)
}

// ActiveConversationIDs returns the conversation ids a user currently belongs to
// (drives server-side realtime subscriptions, §5.2).
func (s *Service) ActiveConversationIDs(ctx context.Context, tenantID, userID string) ([]string, error) {
	members, err := s.store.Members().ListActiveForUser(ctx, tenantID, userID)
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(members))
	for _, m := range members {
		ids = append(ids, m.ConversationID)
	}
	return ids, nil
}

// ListUserConversations builds the §4.2 GET /me/conversations payload:
// each conversation with last_message, unread_count, members, assignment.
// Non-agent: internal notes are excluded from last_message and unread_count (§5.6).
func (s *Service) ListUserConversations(ctx context.Context, tenantID, userID string) ([]map[string]any, error) {
	convs, err := s.store.Conversations().ListForUser(ctx, tenantID, userID)
	if err != nil {
		return nil, err
	}
	const includeInternal = false
	out := make([]map[string]any, 0, len(convs))
	for i := range convs {
		c := &convs[i]
		members, err := s.store.Members().ListActive(ctx, tenantID, c.ID)
		if err != nil {
			return nil, err
		}
		assignment, err := s.store.Assignments().GetByConversation(ctx, tenantID, c.ID)
		if err != nil && !errors.Is(err, store.ErrNotFound) {
			return nil, err
		}
		summary := views.Conversation(c, members, assignment)

		var lastReadID string
		uid := userID
		if rr, err := s.store.Receipts().Get(ctx, tenantID, c.ID, store.Participant{
			Kind: models.MemberUser, ExternalUserID: &uid,
		}); err == nil {
			lastReadID = rr.LastReadMessageID
		}
		if last, err := s.store.Messages().Last(ctx, tenantID, c.ID, includeInternal); err == nil {
			summary["last_message"] = views.Message(last, s.attachmentURLFunc())
		}
		if n, err := s.store.Messages().UnreadCount(ctx, tenantID, c.ID, lastReadID, includeInternal); err == nil {
			summary["unread_count"] = n
		}
		out = append(out, summary)
	}
	return out, nil
}
