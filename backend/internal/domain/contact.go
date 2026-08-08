package domain

import (
	"bytes"
	"context"
	"encoding/json"

	"github.com/bitllow/sild/backend/internal/realtime"
	"github.com/bitllow/sild/backend/internal/store/models"
	"github.com/bitllow/sild/backend/internal/views"
)

// UpsertContact stores a person's profile, replacing it whole: a key the writer
// dropped is gone, so a read always returns a profile someone asserted. Only the
// host's blob and its search text move — Sild's own columns are untouched, or an
// app launch could un-suppress an opted-out person.
func (s *Service) UpsertContact(ctx context.Context, tenantID, externalUserID string, metadata json.RawMessage) error {
	if err := ValidateExternalUserID(externalUserID); err != nil {
		return err
	}
	canonical, err := canonicalJSON(metadata)
	if err != nil {
		return invalid("metadata must be a JSON object")
	}
	text, err := s.searchText(ctx, tenantID, canonical)
	if err != nil {
		return err
	}
	changed, err := s.store.Contacts().Upsert(ctx, tenantID, externalUserID, canonical, text)
	if err != nil {
		return err
	}
	// Invalidation only, and silent on a no-op: a profile is readable solely
	// through a conversation the caller's scope admits, so clients re-read.
	if changed {
		s.emit(ctx, realtime.Target{Tenant: tenantID},
			realtime.EventContactUpdated, "", map[string]any{})
	}
	return nil
}

// SetContactPush turns nudges on or off for one contact, on behalf of the
// tenant's own backend. Suppression survives the app re-registering — that is
// what makes it a preference rather than a token deletion.
func (s *Service) SetContactPush(ctx context.Context, tenantID, externalUserID string, enabled bool) error {
	if err := ValidateExternalUserID(externalUserID); err != nil {
		return err
	}
	return s.store.Contacts().SetPushOptOut(ctx, tenantID, externalUserID, !enabled)
}

// MemberProfiles resolves what member views render inline: one query for every
// contact profile among the members, one lookup per distinct agent. Batched
// because the conversation list renders a page of them at once. extraActors are
// agents named by something other than membership (a page's assignees).
func (s *Service) MemberProfiles(ctx context.Context, tenantID string, members []models.ConversationMember, extraActors ...string) (views.Profiles, error) {
	actors := extraActors
	for i := range members {
		if id := members[i].InternalActorID; id != nil {
			actors = append(actors, *id)
		}
	}
	contacts, err := s.store.Contacts().Profiles(ctx, tenantID, ExternalParticipants(members))
	if err != nil {
		return views.Profiles{}, err
	}
	return views.Profiles{Contacts: contacts, Agents: s.agentNames(ctx, tenantID, actors)}, nil
}

// ExternalParticipants lists the distinct end users among a set of members, in a
// stable order — the people an expansion can carry profiles for, and the ids the
// profile query is keyed on.
func ExternalParticipants(members []models.ConversationMember) []string {
	seen := map[string]bool{}
	out := []string{}
	for i := range members {
		id := members[i].ExternalUserID
		if id == nil || *id == "" || seen[*id] {
			continue
		}
		seen[*id] = true
		out = append(out, *id)
	}
	return out
}

// canonicalJSON normalizes a profile blob so an unchanged write is recognizable
// as one: key order and whitespace must not read as a change. An absent or null
// metadata field clears the profile.
//
// UseNumber keeps every number as the digits the host wrote. The blob is opaque
// and may carry ids past float64's exact range, so decoding into a float would
// silently rewrite the host's data.
func canonicalJSON(raw json.RawMessage) ([]byte, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var v map[string]any
	if err := dec.Decode(&v); err != nil {
		return nil, err
	}
	if v == nil {
		return nil, nil
	}
	return json.Marshal(v)
}
