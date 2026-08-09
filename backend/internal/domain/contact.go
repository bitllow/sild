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
	changed, err := s.store.Contacts().Upsert(ctx, tenantID, externalUserID, canonical, text, profileName(canonical))
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

// profileName lifts the display name out of the blob so it can live in a narrow
// column. Only a string counts: `name` is host-defined and may be anything.
func profileName(metadata []byte) string {
	var m map[string]json.RawMessage
	if json.Unmarshal(metadata, &m) != nil {
		return ""
	}
	var name string
	if json.Unmarshal(m["name"], &name) != nil {
		return ""
	}
	return name
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

// MemberNames resolves what member views render: the narrow display name for
// every contact among the members, one lookup per distinct agent. Batched
// because the conversation list renders a page of them at once. extraActors are
// agents named by something other than membership (a page's assignees).
//
// The profile blob is NOT read here — it is a contacts expansion, and reading it
// for every page is the fan-out the split exists to avoid.
func (s *Service) MemberNames(ctx context.Context, tenantID string, members []models.ConversationMember, extraActors ...string) (views.Profiles, error) {
	actors := make([]string, 0, len(extraActors)+len(members))
	actors = append(actors, extraActors...)
	for i := range members {
		if id := members[i].InternalActorID; id != nil {
			actors = append(actors, *id)
		}
	}
	names, err := s.store.Contacts().Names(ctx, tenantID, ExternalParticipants(members))
	if err != nil {
		return views.Profiles{}, err
	}
	return views.Profiles{Names: names, Agents: s.agentNames(ctx, tenantID, actors)}, nil
}

// MemberProfiles is MemberNames plus the profile blobs, for a caller that asked
// for them with `expand=contacts.metadata`.
func (s *Service) MemberProfiles(ctx context.Context, tenantID string, members []models.ConversationMember, extraActors ...string) (views.Profiles, error) {
	p, err := s.MemberNames(ctx, tenantID, members, extraActors...)
	if err != nil {
		return views.Profiles{}, err
	}
	p.Contacts, err = s.store.Contacts().Profiles(ctx, tenantID, ExternalParticipants(members))
	if err != nil {
		return views.Profiles{}, err
	}
	return p, nil
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
