package domain

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/bitllow/sild/backend/internal/realtime"
	"github.com/bitllow/sild/backend/internal/store"
	"github.com/bitllow/sild/backend/internal/store/models"
	"github.com/bitllow/sild/backend/internal/views"
	"gorm.io/datatypes"
)

// MemberInput describes a member to add (§4.1).
type MemberInput struct {
	UserID   string
	ConvRole models.ConvRole
	Kind     models.MemberKind // defaults to user
	Metadata json.RawMessage
}

// CreateConversationInput is the host-backend create (§4.1).
type CreateConversationInput struct {
	Reference      string
	Metadata       json.RawMessage
	Members        []MemberInput
	OpenAssignment bool
}

// CreateConversation creates an untyped conversation with members and an
// optional assignment, atomically (§1). Host-backend only (API key).
func (s *Service) CreateConversation(ctx context.Context, tenantID string, in CreateConversationInput) (*models.Conversation, error) {
	if len(in.Members) == 0 {
		return nil, invalid("at least one member is required")
	}
	conv := &models.Conversation{
		TenantID:  tenantID,
		Reference: in.Reference,
		Metadata:  datatypes.JSON(in.Metadata),
		Status:    models.ConversationOpen,
		CreatedAt: s.now(),
	}
	var members []models.ConversationMember
	var assignment *models.Assignment

	err := s.store.Tx(ctx, func(tx store.Store) error {
		if err := tx.Conversations().Create(ctx, conv); err != nil {
			return err
		}
		for _, mi := range in.Members {
			m, err := s.buildMember(ctx, tenantID, conv.ID, mi)
			if err != nil {
				return err
			}
			if err := tx.Members().Add(ctx, m); err != nil {
				return err
			}
			members = append(members, *m)
		}
		if in.OpenAssignment {
			assignment = &models.Assignment{
				TenantID: tenantID, ConversationID: conv.ID,
				Status: models.AssignmentQueued, CreatedAt: s.now(),
			}
			if err := tx.Assignments().Create(ctx, assignment); err != nil {
				return err
			}
		}
		// webhook events, atomic with the create (§6.1)
		data := views.Conversation(conv, members, assignment)
		if err := s.enqueueWebhook(ctx, tx, tenantID, conv.ID, "conversation.created", data); err != nil {
			return err
		}
		if assignment != nil {
			if err := s.enqueueWebhook(ctx, tx, tenantID, conv.ID, "assignment.created", views.Assignment(assignment)); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	conv.Members = members
	conv.Assignment = assignment
	return conv, nil
}

// buildMember constructs a member row with materialized search text.
func (s *Service) buildMember(ctx context.Context, tenantID, convID string, mi MemberInput) (*models.ConversationMember, error) {
	if mi.UserID == "" {
		return nil, invalid("member user_id is required")
	}
	kind := mi.Kind
	if kind == "" {
		kind = models.MemberUser
	}
	st, _ := s.searchText(ctx, tenantID, mi.Metadata)
	uid := mi.UserID
	return &models.ConversationMember{
		TenantID:         tenantID,
		ConversationID:   convID,
		MemberKind:       kind,
		ExternalUserID:   &uid,
		ConvRole:         mi.ConvRole,
		Metadata:         datatypes.JSON(mi.Metadata),
		MemberSearchText: st,
		JoinedAt:         s.now(),
	}, nil
}

// GetConversation loads a conversation with members and current assignment.
func (s *Service) GetConversation(ctx context.Context, tenantID, convID string) (*models.Conversation, []models.ConversationMember, *models.Assignment, error) {
	conv, err := s.store.Conversations().Get(ctx, tenantID, convID)
	if err != nil {
		return nil, nil, nil, mapStoreErr(err)
	}
	members, err := s.store.Members().ListActive(ctx, tenantID, convID)
	if err != nil {
		return nil, nil, nil, err
	}
	assignment, err := s.store.Assignments().GetByConversation(ctx, tenantID, convID)
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		return nil, nil, nil, err
	}
	return conv, members, assignment, nil
}

// AddMember adds a member to an existing conversation (§4.1, API key).
func (s *Service) AddMember(ctx context.Context, tenantID, convID string, mi MemberInput) (*models.ConversationMember, error) {
	if _, err := s.store.Conversations().Get(ctx, tenantID, convID); err != nil {
		return nil, mapStoreErr(err)
	}
	m, err := s.buildMember(ctx, tenantID, convID, mi)
	if err != nil {
		return nil, err
	}
	if err := s.store.Members().Add(ctx, m); err != nil {
		return nil, err
	}
	uid := derefStr(m.ExternalUserID)
	s.emit(ctx, realtime.Target{Conversation: convID, Users: []string{uid}},
		realtime.EventMemberAdded, convID, map[string]any{"user_id": uid, "conv_role": m.ConvRole})
	_ = s.fireWebhook(ctx, tenantID, convID, "member.added", map[string]any{"user_id": uid, "conv_role": m.ConvRole})
	return m, nil
}

// RemoveMember removes a member, rejecting a removal that would leave an OPEN
// conversation with zero members (§1 — close it instead).
func (s *Service) RemoveMember(ctx context.Context, tenantID, convID, userID string) error {
	conv, err := s.store.Conversations().Get(ctx, tenantID, convID)
	if err != nil {
		return mapStoreErr(err)
	}
	if conv.Status == models.ConversationOpen {
		n, err := s.store.Members().CountActive(ctx, tenantID, convID)
		if err != nil {
			return err
		}
		if n <= 1 {
			return ErrConflict // would leave an open conversation empty
		}
	}
	if err := s.store.Members().RemoveExternal(ctx, tenantID, convID, userID); err != nil {
		return mapStoreErr(err)
	}
	s.emit(ctx, realtime.Target{Conversation: convID, Users: []string{userID}},
		realtime.EventMemberRemoved, convID, map[string]any{"user_id": userID})
	_ = s.fireWebhook(ctx, tenantID, convID, "member.removed", map[string]any{"user_id": userID})
	return nil
}

// CloseConversation transitions a conversation to closed (terminal, §1).
func (s *Service) CloseConversation(ctx context.Context, tenantID, convID string) error {
	conv, err := s.store.Conversations().Get(ctx, tenantID, convID)
	if err != nil {
		return mapStoreErr(err)
	}
	if conv.Status == models.ConversationClosed {
		return nil // idempotent
	}
	if err := s.store.Conversations().UpdateStatus(ctx, tenantID, convID, models.ConversationClosed); err != nil {
		return err
	}
	s.emit(ctx, realtime.Target{Conversation: convID}, realtime.EventConversationClosed, convID, map[string]any{})
	_ = s.fireWebhook(ctx, tenantID, convID, "conversation.closed", map[string]any{})
	return nil
}

// Remap rewrites a guest id to a real user, preserving history (§4.5).
func (s *Service) Remap(ctx context.Context, tenantID, convID, fromUserID, toUserID string) error {
	if fromUserID == "" || toUserID == "" {
		return invalid("from_user_id and to_user_id are required")
	}
	if err := s.store.Members().Remap(ctx, tenantID, convID, fromUserID, toUserID); err != nil {
		return mapStoreErr(err)
	}
	s.emit(ctx, realtime.Target{Conversation: convID, Users: []string{toUserID}},
		realtime.EventMemberAdded, convID, map[string]any{"user_id": toUserID})
	return nil
}

// fireWebhook enqueues a webhook event outside a caller tx (its own tx).
func (s *Service) fireWebhook(ctx context.Context, tenantID, convID, eventType string, data any) error {
	return s.store.Tx(ctx, func(tx store.Store) error {
		return s.enqueueWebhook(ctx, tx, tenantID, convID, eventType, data)
	})
}

func derefStr(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

func mapStoreErr(err error) error {
	if errors.Is(err, store.ErrNotFound) {
		return ErrNotFound
	}
	return err
}
