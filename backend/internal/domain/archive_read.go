package domain

import (
	"context"
	"encoding/json"

	"github.com/bitllow/sild/backend/internal/store/models"
)

// HasAssignment reports whether a conversation carries an assignment (i.e. it is
// a support conversation). Used to scope platform agents to the support inbox
// (§7: agents have inbox access, not all-conversations access).
func (s *Service) HasAssignment(ctx context.Context, tenantID, convID string) bool {
	_, err := s.store.Assignments().GetByConversation(ctx, tenantID, convID)
	return err == nil
}

func (s *Service) tombstone(ctx context.Context, tenantID, convID string) (*models.ConversationArchive, bool) {
	tomb, err := s.store.Archives().GetTombstone(ctx, tenantID, convID)
	if err != nil {
		return nil, false
	}
	return tomb, true
}

// IsArchived reports whether the conversation has been moved to a cold sink (§12).
func (s *Service) IsArchived(ctx context.Context, tenantID, convID string) bool {
	_, ok := s.tombstone(ctx, tenantID, convID)
	return ok
}

// IsArchivedMember authorizes an archived read for a user against the membership
// snapshot preserved in the tombstone (hot members are gone, review finding).
func (s *Service) IsArchivedMember(ctx context.Context, tenantID, convID, userID string) bool {
	tomb, ok := s.tombstone(ctx, tenantID, convID)
	if !ok {
		return false
	}
	var members []map[string]any
	if err := json.Unmarshal(tomb.MembersSnapshot, &members); err != nil {
		return false
	}
	for _, m := range members {
		if id, _ := m["external_user_id"].(string); id == userID {
			return true
		}
	}
	return false
}

// ArchivedMessages reads a conversation's messages from the sink (§12 fallback).
// archived is true when a tombstone exists (regardless of read success).
func (s *Service) ArchivedMessages(ctx context.Context, tenantID, convID string, includeInternal bool) (msgs []map[string]any, archived bool, err error) {
	tomb, ok := s.tombstone(ctx, tenantID, convID)
	if !ok {
		return nil, false, nil
	}
	if s.sink == nil {
		return nil, true, ErrNotFound
	}
	ser, err := s.sink.Read(ctx, tomb.SinkRef)
	if err != nil {
		return nil, true, err
	}
	for _, m := range ser.Messages {
		if !includeInternal {
			if v, _ := m["visibility"].(string); v == string(models.VisibilityInternal) {
				continue // strip internal notes for non-agents (§5.6)
			}
		}
		msgs = append(msgs, m)
	}
	return msgs, true, nil
}

// ArchivedConversation rehydrates a conversation view from the sink (§12).
func (s *Service) ArchivedConversation(ctx context.Context, tenantID, convID string) (view map[string]any, archived bool, err error) {
	tomb, ok := s.tombstone(ctx, tenantID, convID)
	if !ok {
		return nil, false, nil
	}
	if s.sink == nil {
		return nil, true, ErrNotFound
	}
	ser, err := s.sink.Read(ctx, tomb.SinkRef)
	if err != nil {
		return nil, true, err
	}
	view = map[string]any{
		"id":        ser.ConversationID,
		"status":    ser.Status,
		"reference": ser.Reference,
		"metadata":  ser.Metadata,
		"members":   ser.Members,
		"archived":  true,
	}
	return view, true, nil
}
