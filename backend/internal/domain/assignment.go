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
	conv, err := s.store.Conversations().Get(ctx, tenantID, convID)
	if err != nil {
		return nil, mapStoreErr(err)
	}
	// A conversation's kind is fixed at creation and never re-derived. A peer
	// conversation stays peer for life (agents observe/step in via peer_access,
	// which never adds an assignment), so it must never be queued — otherwise it
	// would carry an assignment while every kind-based path still treats it as
	// peer, leaving it on the peer surface yet also in the raw queue.
	if conv.Kind == models.KindPeer {
		return nil, ErrForbidden
	}
	a := &models.Assignment{TenantID: tenantID, ConversationID: convID, Status: models.AssignmentQueued, CreatedAt: s.now()}
	if err := s.store.Assignments().Create(ctx, a); err != nil {
		return nil, err
	}
	_ = s.fireWebhook(ctx, tenantID, convID, "assignment.created", views.Assignment(a))
	s.emit(ctx, realtime.Target{Conversation: convID, Tenant: tenantID}, realtime.EventAssignmentUpdated, convID, views.Assignment(a))
	return a, nil
}

// ListQueue returns one cursor-paginated, sorted page of inbox assignments (§4.3).
func (s *Service) ListQueue(ctx context.Context, tenantID string, p store.QueueParams) (store.QueuePage, error) {
	return s.store.Assignments().ListQueue(ctx, tenantID, p)
}

// CountOpenConversations returns the tenant's open-conversation count for the
// inbox badge (§8).
func (s *Service) CountOpenConversations(ctx context.Context, tenantID string) (int64, error) {
	return s.store.Conversations().CountOpenSupport(ctx, tenantID)
}

// CountQueue returns the inbox scope-tab counters (assigned-to-me / unassigned /
// closed) for the calling agent (§4.3).
func (s *Service) CountQueue(ctx context.Context, tenantID, actorID string) (store.QueueCounts, error) {
	return s.store.Assignments().CountQueue(ctx, tenantID, actorID)
}

// AssignmentConversation returns the conversation an assignment belongs to, so a
// caller can authorize against the conversation rather than the assignment id.
func (s *Service) AssignmentConversation(ctx context.Context, tenantID, assignmentID string) (string, error) {
	a, err := s.store.Assignments().Get(ctx, tenantID, assignmentID)
	if err != nil {
		return "", mapStoreErr(err)
	}
	return a.ConversationID, nil
}

// ClaimAssignment assigns a queued assignment to the calling agent (§4.3).
// State: queued → assigned. Two agents claiming at once resolve in the database:
// the loser sees the assignment already taken.
func (s *Service) ClaimAssignment(ctx context.Context, tenantID, assignmentID, agentActorID string) (*models.Assignment, error) {
	return s.transition(ctx, tenantID, assignmentID, store.AssignmentTransition{
		From: []models.AssignmentStatus{models.AssignmentQueued}, To: models.AssignmentAssigned,
		Assignee: &agentActorID,
	}, func(current *models.Assignment) error {
		switch {
		case current.Status == models.AssignmentClosed:
			return conflict(CodeAssignmentAlreadyClosed, "assignment is already closed")
		case current.AssigneeActorID != nil && *current.AssigneeActorID == agentActorID:
			return nil // this agent already holds it; a retried claim is not a conflict
		default:
			return conflict(CodeAssignmentAlreadyTaken, "assignment is already claimed by another agent")
		}
	})
}

// CloseAssignment closes an assignment (terminal, §1). Idempotent: closing an
// already-closed assignment succeeds without re-emitting.
func (s *Service) CloseAssignment(ctx context.Context, tenantID, assignmentID string) (*models.Assignment, error) {
	now := s.now()
	return s.transition(ctx, tenantID, assignmentID, store.AssignmentTransition{
		From: []models.AssignmentStatus{models.AssignmentQueued, models.AssignmentAssigned},
		To:   models.AssignmentClosed, ClosedAt: &now,
	}, func(current *models.Assignment) error { return nil })
}

// ReturnToQueue moves an assigned assignment back to the queue (assigned → queued).
func (s *Service) ReturnToQueue(ctx context.Context, tenantID, assignmentID string) (*models.Assignment, error) {
	return s.transition(ctx, tenantID, assignmentID, store.AssignmentTransition{
		From: []models.AssignmentStatus{models.AssignmentAssigned}, To: models.AssignmentQueued,
		ClearAssignee: true,
	}, func(current *models.Assignment) error {
		return conflict(CodeAssignmentAlreadyClosed, "assignment is not currently assigned")
	})
}

// transition applies a guarded state change, emitting only when the write took
// the row. When the guard rejects it, `lost` decides from the current row whether
// that is idempotent (nil) or a conflict.
//
// Closing the assignment does NOT close the conversation (review finding):
// conversation close is its own action; archival keys on conversation.status.
func (s *Service) transition(ctx context.Context, tenantID, assignmentID string, t store.AssignmentTransition, lost func(*models.Assignment) error) (*models.Assignment, error) {
	ok, err := s.store.Assignments().Transition(ctx, tenantID, assignmentID, t)
	if err != nil {
		return nil, err
	}
	a, err := s.store.Assignments().Get(ctx, tenantID, assignmentID)
	if err != nil {
		return nil, mapStoreErr(err)
	}
	if !ok {
		if err := lost(a); err != nil {
			return nil, err
		}
		return a, nil
	}
	data := views.Assignment(a)
	// An assignment exists, so this is a support conversation: the agents channel
	// without a lookup to rediscover it.
	s.emit(ctx, realtime.Target{Conversation: a.ConversationID, Tenant: tenantID},
		realtime.EventAssignmentUpdated, a.ConversationID, data)
	_ = s.fireWebhook(ctx, tenantID, a.ConversationID, "assignment.updated", data)
	return a, nil
}
