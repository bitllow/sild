package domain

import (
	"context"
	"encoding/json"

	"github.com/bitllow/sild/backend/internal/realtime"
	"github.com/bitllow/sild/backend/internal/store"
	"github.com/bitllow/sild/backend/internal/store/models"
	"github.com/bitllow/sild/backend/internal/views"
	"gorm.io/datatypes"
)

// OpenSupportRequest creates a conversation with the client as sole member plus a
// queued assignment, atomically (§4.0). Used by the authed client (self), the
// agent (with a user), and the host-backend guest path. Never deduped (§4.2).
func (s *Service) OpenSupportRequest(ctx context.Context, tenantID, clientUserID string, metadata json.RawMessage) (*models.Conversation, *models.Assignment, error) {
	if clientUserID == "" {
		return nil, nil, invalid("client user id is required")
	}
	conv := &models.Conversation{TenantID: tenantID, Status: models.ConversationOpen, CreatedAt: s.now()}
	assignment := &models.Assignment{TenantID: tenantID, Status: models.AssignmentQueued, CreatedAt: s.now()}
	var member models.ConversationMember

	err := s.store.Tx(ctx, func(tx store.Store) error {
		if err := tx.Conversations().Create(ctx, conv); err != nil {
			return err
		}
		st, _ := s.searchText(ctx, tenantID, metadata)
		uid := clientUserID
		member = models.ConversationMember{
			TenantID: tenantID, ConversationID: conv.ID, MemberKind: models.MemberUser,
			ExternalUserID: &uid, ConvRole: models.RoleClient,
			Metadata: datatypes.JSON(metadata), MemberSearchText: st, JoinedAt: s.now(),
		}
		if err := tx.Members().Add(ctx, &member); err != nil {
			return err
		}
		assignment.ConversationID = conv.ID
		if err := tx.Assignments().Create(ctx, assignment); err != nil {
			return err
		}
		if err := s.enqueueWebhook(ctx, tx, tenantID, conv.ID, "conversation.created",
			views.Conversation(conv, []models.ConversationMember{member}, assignment)); err != nil {
			return err
		}
		return s.enqueueWebhook(ctx, tx, tenantID, conv.ID, "assignment.created", views.Assignment(assignment))
	})
	if err != nil {
		return nil, nil, err
	}
	conv.Members = []models.ConversationMember{member}
	conv.Assignment = assignment
	// notify the client's user channel + the tenant agents channel (new queue item)
	s.emit(ctx, realtime.Target{Users: []string{clientUserID}, Tenant: tenantID},
		realtime.EventAssignmentUpdated, conv.ID, views.Assignment(assignment))
	return conv, assignment, nil
}

// AddAssignment queues an existing conversation for an agent (§4.0).
func (s *Service) AddAssignment(ctx context.Context, tenantID, convID string) (*models.Assignment, error) {
	if _, err := s.store.Conversations().Get(ctx, tenantID, convID); err != nil {
		return nil, mapStoreErr(err)
	}
	a := &models.Assignment{TenantID: tenantID, ConversationID: convID, Status: models.AssignmentQueued, CreatedAt: s.now()}
	if err := s.store.Assignments().Create(ctx, a); err != nil {
		return nil, err
	}
	_ = s.fireWebhook(ctx, tenantID, convID, "assignment.created", views.Assignment(a))
	s.emit(ctx, realtime.Target{Conversation: convID, Tenant: tenantID}, realtime.EventAssignmentUpdated, convID, views.Assignment(a))
	return a, nil
}

// ListQueue returns inbox assignments (§4.3).
func (s *Service) ListQueue(ctx context.Context, tenantID string, status *models.AssignmentStatus, assignee *string) ([]models.Assignment, error) {
	return s.store.Assignments().ListQueue(ctx, tenantID, status, assignee)
}

// ClaimAssignment assigns a queued assignment to the calling agent (§4.3).
// State: queued → assigned.
func (s *Service) ClaimAssignment(ctx context.Context, tenantID, assignmentID, agentActorID string) (*models.Assignment, error) {
	return s.transition(ctx, tenantID, assignmentID, func(a *models.Assignment) error {
		if a.Status == models.AssignmentClosed {
			return ErrConflict
		}
		a.Status = models.AssignmentAssigned
		a.AssigneeActorID = &agentActorID
		return nil
	})
}

// CloseAssignment closes an assignment (terminal, §1).
func (s *Service) CloseAssignment(ctx context.Context, tenantID, assignmentID string) (*models.Assignment, error) {
	return s.transition(ctx, tenantID, assignmentID, func(a *models.Assignment) error {
		if a.Status == models.AssignmentClosed {
			return nil
		}
		now := s.now()
		a.Status = models.AssignmentClosed
		a.ClosedAt = &now
		return nil
	})
}

// ReturnToQueue moves an assigned assignment back to the queue (assigned → queued).
func (s *Service) ReturnToQueue(ctx context.Context, tenantID, assignmentID string) (*models.Assignment, error) {
	return s.transition(ctx, tenantID, assignmentID, func(a *models.Assignment) error {
		if a.Status != models.AssignmentAssigned {
			return ErrConflict
		}
		a.Status = models.AssignmentQueued
		a.AssigneeActorID = nil
		return nil
	})
}

// transition applies a state change and emits the update. Closing the assignment
// does NOT close the conversation (review finding): conversation close is its own
// action; archival keys on conversation.status.
func (s *Service) transition(ctx context.Context, tenantID, assignmentID string, mutate func(*models.Assignment) error) (*models.Assignment, error) {
	a, err := s.store.Assignments().Get(ctx, tenantID, assignmentID)
	if err != nil {
		return nil, mapStoreErr(err)
	}
	if err := mutate(a); err != nil {
		return nil, err
	}
	if err := s.store.Assignments().Update(ctx, a); err != nil {
		return nil, err
	}
	data := views.Assignment(a)
	s.emit(ctx, realtime.Target{Conversation: a.ConversationID}, realtime.EventAssignmentUpdated, a.ConversationID, data)
	_ = s.fireWebhook(ctx, tenantID, a.ConversationID, "assignment.updated", data)
	return a, nil
}
