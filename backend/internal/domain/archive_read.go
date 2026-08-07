package domain

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/bitllow/sild/backend/internal/storage"

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

// IsArchived reports whether a conversation's message bulk has moved to the
// sink. Archival retains the conversation row, so this reads a column rather
// than inferring archival from a missing row.
func (s *Service) IsArchived(ctx context.Context, tenantID, convID string) bool {
	c, err := s.store.Conversations().Get(ctx, tenantID, convID)
	return err == nil && c.ArchivedAt != nil
}

// IsArchivedMember authorizes an archived read for a user against the membership
// snapshot preserved in the tombstone (hot members are gone, review finding).
func (s *Service) IsArchivedMember(ctx context.Context, tenantID, convID, userID string) bool {
	// Membership rows survive archival, so ask the live table first. The
	// tombstone snapshot below only covers conversations archived before the
	// retention change, whose rows were deleted.
	if ok, err := s.store.Members().IsActiveMember(ctx, tenantID, convID, userID); err == nil && ok {
		return true
	}
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
	// Archival now RETAINS the conversation row, so "archived" is a column, not
	// the absence of a row. Only the message bulk moved to the sink.
	if !s.IsArchived(ctx, tenantID, convID) {
		return nil, false, nil
	}
	tomb, ok := s.tombstone(ctx, tenantID, convID)
	if !ok {
		return nil, true, ErrNotFound
	}
	if s.sink == nil {
		return nil, true, ErrNotFound
	}
	ser, err := s.sink.Read(ctx, tomb.SinkRef)
	if err != nil {
		return nil, true, archiveReadErr(err)
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

// archiveReadErr maps a missing archive object to ErrNotFound: the tombstone says
// the conversation was archived, so an object the bucket cannot produce is a gone
// conversation, not a broken server.
func archiveReadErr(err error) error {
	if errors.Is(err, storage.ErrObjectNotFound) {
		return ErrNotFound
	}
	return err
}
