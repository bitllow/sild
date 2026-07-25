package store

import (
	"context"
	"time"

	"github.com/bitllow/sild/backend/internal/policy"
)

// Contact is a person the tenant has talked to, projected from membership rows.
// The wire identity is the host's external_user_id; the projection has no id.
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
	Search string // free text over member_search_text + external_user_id
	Exact  string // single-contact lookup
}

// ContactRepo reads the contact projection. The scope is applied BEFORE
// aggregation: an aggregate over unauthorized rows leaks through ordering.
type ContactRepo interface {
	ListContacts(ctx context.Context, tenantID string, scope policy.ResourceScope, q ContactQuery) (Page[Contact], error)
	GetContact(ctx context.Context, tenantID string, scope policy.ResourceScope, externalUserID string) (*Contact, error)
}
