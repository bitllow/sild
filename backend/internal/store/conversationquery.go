package store

import (
	"time"

	"github.com/bitllow/sild/backend/internal/store/models"
)

// ConversationQuery is the client-chosen narrowing of a conversation list. It
// can only narrow what the policy scope allows, which is why the scope is a
// separate argument to ConversationRepo.List.
type ConversationQuery struct {
	PageParams

	// TenantID is the verified tenant, explicit so a missing scope is a
	// compile-visible omission rather than a cross-tenant read (§1).
	TenantID string

	// Kind narrows to one conversation kind. nil = every kind the scope allows.
	Kind *models.ConversationKind
	// Status filters the CONVERSATION lifecycle (open|closed) — the inbox's
	// notion of "closed".
	Status *models.ConversationStatus
	// AssignmentStatus filters the ASSIGNMENT state machine, support only.
	AssignmentStatus *models.AssignmentStatus
	// AssigneeActorID filters to a specific operator; "" means no filter.
	AssigneeActorID string
	// Unassigned filters to a representative assignment with no assignee.
	Unassigned bool
	// Participant filters to one external user's conversations (contact history).
	Participant string
	// ConvRole filters to conversations having a member in this role.
	ConvRole string
	// IDs restricts the result to a pre-computed set, so search reuses this query.
	IDs []string
}

// ConversationItem is one enriched row, batch-loaded so a page has no N+1.
type ConversationItem struct {
	Conversation models.Conversation
	Members      []models.ConversationMember
	Assignment   *models.Assignment
	LastActivity time.Time
}
