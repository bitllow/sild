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

		// The handling agent's display name (their first name) so the widget can
		// label the conversation with a real person instead of "Support".
		if assignment != nil && assignment.AssigneeActorID != nil {
			if name := s.AgentDisplayName(ctx, tenantID, *assignment.AssigneeActorID); name != "" {
				summary["agent_name"] = name
			}
		}

		var lastReadID string
		uid := userID
		if rr, err := s.store.Receipts().Get(ctx, tenantID, c.ID, store.Participant{
			Kind: models.MemberUser, ExternalUserID: &uid,
		}); err == nil {
			lastReadID = rr.LastReadMessageID
		}
		if last, err := s.store.Messages().Last(ctx, tenantID, c.ID, includeInternal); err == nil {
			lm := views.Message(last, s.attachmentURLFunc())
			// If an agent spoke last, prefer their actual name for the row label.
			if last.InternalActorID != nil {
				if name := s.AgentDisplayName(ctx, tenantID, *last.InternalActorID); name != "" {
					lm["author_name"] = name
					summary["agent_name"] = name
				}
			}
			summary["last_message"] = lm
		}
		if n, err := s.store.Messages().UnreadCount(ctx, tenantID, c.ID, lastReadID, includeInternal); err == nil {
			summary["unread_count"] = n
		}
		out = append(out, summary)
	}
	return out, nil
}

// ListContactConversations returns every conversation a contact (identified by
// external_user_id) takes part in, newest-first, in the inbox queue-row shape
// ({assignment, conversation:{…, last_activity, last_message}}). It powers the
// Details-panel "Earlier from …" history and the "View all from" contact filter
// (§4.3). Unlike the queue it isn't paginated — a single contact's thread count
// is small — and it reads the denormalized last-activity/preview, so it never
// touches the messages table.
func (s *Service) ListContactConversations(ctx context.Context, tenantID, externalUserID string) ([]map[string]any, error) {
	if externalUserID == "" {
		return nil, invalid("external_user_id is required")
	}
	convs, err := s.store.Conversations().ListForUser(ctx, tenantID, externalUserID)
	if err != nil {
		return nil, err
	}
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
		conv := views.Conversation(c, members, nil)
		lastAt := c.CreatedAt
		if c.LastMessageAt != nil {
			lastAt = *c.LastMessageAt
		}
		conv["last_activity"] = lastAt
		if c.LastMessagePreview != "" {
			conv["last_message"] = map[string]any{"body": c.LastMessagePreview, "created_at": c.LastMessageAt}
		}
		if subject := s.EmailSubject(ctx, tenantID, c.ID); subject != "" {
			conv["subject"] = subject
		}
		row := map[string]any{"conversation": conv}
		if assignment != nil {
			row["assignment"] = views.Assignment(assignment)
		}
		out = append(out, row)
	}
	return out, nil
}
