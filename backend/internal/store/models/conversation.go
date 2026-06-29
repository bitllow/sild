package models

import (
	"time"

	"github.com/bitllow/sild/backend/internal/id"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

// Conversation is the single untyped primitive (§1). No type field; "support" is
// any conversation carrying an Assignment.
type Conversation struct {
	ID        string             `gorm:"primaryKey;size:40"`
	TenantID  string             `gorm:"size:40;not null;index:idx_conv_tenant_status,priority:1"`
	Reference string             `gorm:"size:255;index:idx_conv_reference"` // host object id (trip_id, order_id)
	Metadata  datatypes.JSON     `gorm:"type:json"`                         // host-defined, opaque
	Status    ConversationStatus `gorm:"size:16;not null;default:'open';index:idx_conv_tenant_status,priority:2"`
	CreatedAt time.Time

	// Denormalized last participant-visible activity, maintained on each message
	// (domain.SendMessage → TouchLastMessage). Lets the inbox queue sort/keyset on
	// an indexed column and render a row preview WITHOUT touching the messages
	// table (the list fetches only the last message). NULL until the first message
	// — queries COALESCE to CreatedAt.
	LastMessageAt      *time.Time `gorm:"index:idx_conv_last_activity"`
	LastMessagePreview string     `gorm:"size:512"`

	Members    []ConversationMember `gorm:"constraint:OnDelete:CASCADE"`
	Assignment *Assignment          `gorm:"-"` // loaded explicitly when needed
}

func (c *Conversation) BeforeCreate(*gorm.DB) error {
	if c.ID == "" {
		c.ID = id.New(id.Conversation)
	}
	return nil
}

// ConversationMember is a participant. Exactly one of ExternalUserID /
// InternalActorID is set (§3 identity namespaces). LeftAt!=nil = removed.
type ConversationMember struct {
	ID             string  `gorm:"primaryKey;size:40"`
	TenantID       string  `gorm:"size:40;not null;index:idx_member_tenant"`
	ConversationID string  `gorm:"size:40;not null;index:idx_member_conv"`
	MemberKind     MemberKind `gorm:"size:16;not null"`
	ExternalUserID *string `gorm:"size:255;index:idx_member_external"` // host namespace (incl. guests, email address)
	InternalActorID *string `gorm:"size:40"`                          // our namespace (admin_users.id)
	ConvRole       ConvRole       `gorm:"size:16"`
	Metadata       datatypes.JSON `gorm:"type:json"` // per-participant, host-defined
	// MemberSearchText is the materialized concat of searchable_metadata_keys
	// values, refreshed on write; the trigram/LIKE index targets THIS (§3).
	MemberSearchText string `gorm:"type:text"`
	JoinedAt         time.Time
	LeftAt           *time.Time
}

func (m *ConversationMember) BeforeCreate(*gorm.DB) error {
	if m.ID == "" {
		m.ID = id.New(id.Member)
	}
	if m.JoinedAt.IsZero() {
		m.JoinedAt = nowFn()
	}
	return nil
}

// Active reports whether the member is currently in the conversation.
func (m *ConversationMember) Active() bool { return m.LeftAt == nil }

// Assignment turns a conversation into a support request (§1, §4.0). Multiple
// assignments per user are fine (each its own conversation). State machine
// enforced in the domain layer.
type Assignment struct {
	ID              string           `gorm:"primaryKey;size:40"`
	TenantID        string           `gorm:"size:40;not null;index:idx_assign_queue,priority:1"`
	ConversationID  string           `gorm:"size:40;not null;index:idx_assign_conv"`
	AssigneeActorID *string          `gorm:"size:40;index:idx_assign_queue,priority:3"` // null until claimed
	Status          AssignmentStatus `gorm:"size:16;not null;default:'queued';index:idx_assign_queue,priority:2"`
	CreatedAt       time.Time
	ClosedAt        *time.Time
}

func (a *Assignment) BeforeCreate(*gorm.DB) error {
	if a.ID == "" {
		a.ID = id.New(id.Assignment)
	}
	return nil
}

// ReadReceipt is one row per participant per conversation (upsert). The stored
// LastReadMessageID is monotonic: a write with an older id is ignored via a
// GREATEST guard (or read-modify-write on SQLite) (§3).
type ReadReceipt struct {
	ID                string  `gorm:"primaryKey;size:40"`
	TenantID          string  `gorm:"size:40;not null"`
	ConversationID    string  `gorm:"size:40;not null;uniqueIndex:idx_receipt_participant,priority:1"`
	ParticipantKind   MemberKind `gorm:"size:16;not null;uniqueIndex:idx_receipt_participant,priority:2"`
	ExternalUserID    *string `gorm:"size:255;uniqueIndex:idx_receipt_participant,priority:3"`
	InternalActorID   *string `gorm:"size:40;uniqueIndex:idx_receipt_participant,priority:4"`
	LastReadMessageID string  `gorm:"size:40;not null"`
	UpdatedAt         time.Time
}

func (r *ReadReceipt) BeforeCreate(*gorm.DB) error {
	if r.ID == "" {
		r.ID = id.New(id.ReadReceipt)
	}
	return nil
}

// PushToken is a registered device token (§5.5). One user may have many devices;
// deregistered on logout.
type PushToken struct {
	ID              string  `gorm:"primaryKey;size:40"`
	TenantID        string  `gorm:"size:40;not null;index"`
	MemberKind      MemberKind `gorm:"size:16;not null"`
	ExternalUserID  *string `gorm:"size:255;index:idx_push_external"`
	InternalActorID *string `gorm:"size:40"`
	Platform        PushPlatform `gorm:"size:16;not null"`
	Token           string       `gorm:"size:512;not null;uniqueIndex:idx_push_token"`
	UpdatedAt       time.Time
}

func (p *PushToken) BeforeCreate(*gorm.DB) error {
	if p.ID == "" {
		p.ID = id.New(id.PushToken)
	}
	return nil
}
