package store

import (
	"time"

	"github.com/bitllow/sild/backend/internal/store/models"
)

// ConversationQuery is the client-chosen narrowing of a conversation list. It
// can only ever narrow what the policy scope already allows — the scope is a
// separate argument to ConversationRepo.List precisely so the two cannot be
// confused.
//
// AssigneeActorID lives here and NOT on the scope: "assigned to me" is the
// inbox's You tab, a filter the client picks, never an authorization ceiling.
type ConversationQuery struct {
	PageParams

	// TenantID is the verified tenant. Explicit on every repo call so a missing
	// scope is a compile-visible omission, not a silent cross-tenant read (§1).
	TenantID string

	// Kind narrows to one conversation kind. nil = every kind the scope allows.
	Kind *models.ConversationKind
	// Status filters the CONVERSATION lifecycle (open|closed). Distinct from
	// AssignmentStatus: "closed" in the inbox means the conversation is closed,
	// which is what the old exclude_closed flag actually filtered on.
	Status *models.ConversationStatus
	// AssignmentStatus filters the ASSIGNMENT state machine
	// (queued → assigned → closed). Only meaningful alongside Kind=support.
	AssignmentStatus *models.AssignmentStatus
	// AssigneeActorID filters to a specific operator; "" means no filter.
	AssigneeActorID string
	// Unassigned filters to conversations whose representative assignment has no
	// assignee, for the Unassigned scope.
	Unassigned bool
	// Participant filters to one external user's conversations (contact history).
	Participant string
	// ConvRole filters to conversations having a member in this role.
	ConvRole string
	// IDs restricts the result to a pre-computed set, used to layer free-text
	// search on top of the same query rather than forking a second one.
	IDs []string
}

// ConversationItem is one enriched row: the conversation, its active members,
// the representative assignment (if any) and the denormalized last activity, so
// a page renders without an N+1.
type ConversationItem struct {
	Conversation models.Conversation
	Members      []models.ConversationMember
	Assignment   *models.Assignment
	LastActivity time.Time
}
