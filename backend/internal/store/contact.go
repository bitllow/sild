package store

import (
	"context"
	"time"

	"github.com/bitllow/sild/backend/internal/policy"
)

// Contact is one directory entry: a stored profile joined to the aggregates of
// the memberships the caller may see. The wire identity is the host's
// external_user_id; there is no id.
type Contact struct {
	ExternalUserID string    `json:"external_user_id"`
	Metadata       []byte    `json:"-"`
	LastActivity   time.Time `json:"last_activity"`
	// ConversationCount counts CURRENT conversations; LastActivity spans all
	// authorized memberships, so an archived-only contact stays findable.
	ConversationCount int64 `json:"conversation_count"`
}

// ContactQuery narrows a contact list.
type ContactQuery struct {
	PageParams
	Search string // free text over contacts.search_text + external_user_id
	Exact  string // single-contact lookup
}

// ContactRepo stores profiles and reads the directory projection over them.
// Directory existence stays derived from membership, and the scope is applied
// BEFORE aggregation: an aggregate over unauthorized rows leaks through ordering.
type ContactRepo interface {
	ListContacts(ctx context.Context, tenantID string, scope policy.ResourceScope, q ContactQuery) (Page[Contact], error)
	GetContact(ctx context.Context, tenantID string, scope policy.ResourceScope, externalUserID string) (*Contact, error)
	// Upsert replaces the profile whole and rematerializes the search text. It
	// never writes a system column, so an app launch cannot un-suppress someone.
	// changed reports whether the stored metadata actually moved.
	Upsert(ctx context.Context, tenantID, externalUserID string, metadata []byte, searchText, name string) (changed bool, err error)
	// Profiles returns stored metadata by external_user_id, missing rows omitted:
	// a person may be in a conversation without anyone ever writing their profile.
	Profiles(ctx context.Context, tenantID string, externalUserIDs []string) (map[string][]byte, error)
	// Names returns display names from the narrow table, missing ones omitted.
	// Separate from Profiles so rendering participants never reads the blob.
	Names(ctx context.Context, tenantID string, externalUserIDs []string) (map[string]string, error)
	// SetPushOptOut sets or clears the suppression, creating a profile-less row
	// when the person has none — the tenant may suppress before they ever talk.
	SetPushOptOut(ctx context.Context, tenantID, externalUserID string, optedOut bool) error
	// OptedOut answers for a whole conversation's membership at once, so fan-out
	// costs one query rather than one per member.
	OptedOut(ctx context.Context, tenantID string, externalUserIDs []string) (map[string]bool, error)
}
