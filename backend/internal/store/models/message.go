package models

import (
	"time"

	"github.com/bitllow/sild/backend/internal/id"
	"gorm.io/gorm"
)

// Message is an entry in a conversation. Exactly one of ExternalUserID /
// InternalActorID is set. Sortable ULID id drives ?before=/?after= pagination
// and read-receipt monotonicity (§3, §4.2).
type Message struct {
	ID              string     `gorm:"primaryKey;size:40;index:idx_msg_page,priority:3"`
	TenantID        string     `gorm:"size:40;not null;index:idx_msg_page,priority:1"`
	ConversationID  string     `gorm:"size:40;not null;index:idx_msg_page,priority:2;uniqueIndex:idx_msg_idem,priority:1"`
	SenderKind      SenderKind `gorm:"size:16;not null"`
	Visibility      Visibility `gorm:"size:16;not null;default:'participants'"`
	Channel         Channel    `gorm:"size:16;not null;default:'app'"`
	ExternalUserID  *string    `gorm:"size:255"`
	InternalActorID *string    `gorm:"size:40"`
	Body            string     `gorm:"type:text"`
	// ClientMsgID is the caller-supplied idempotency key (§4.2). Unique per
	// conversation; NULL for server ingress (multiple NULLs allowed on all
	// three dialects, so ingress never collides).
	ClientMsgID *string `gorm:"size:64;uniqueIndex:idx_msg_idem,priority:2"`
	CreatedAt   time.Time

	Attachments []MessageAttachment `gorm:"constraint:OnDelete:CASCADE"`
}

func (m *Message) BeforeCreate(*gorm.DB) error {
	if m.ID == "" {
		m.ID = id.New(id.Message)
	}
	return nil
}

// MessageAttachment references a bucket object (§11). disposition is a render
// hint, not storage. tenant_id present for scoping (review finding).
type MessageAttachment struct {
	ID          string      `gorm:"primaryKey;size:40"`
	TenantID    string      `gorm:"size:40;not null;index"`
	MessageID   string      `gorm:"size:40;not null;index"`
	Disposition Disposition `gorm:"size:16;not null;default:'attachment'"`
	ObjectKey   string      `gorm:"size:512;not null"`
	MimeType    string      `gorm:"size:255"`
	SizeBytes   int64
	Filename    string `gorm:"size:512"`
}

func (a *MessageAttachment) BeforeCreate(*gorm.DB) error {
	if a.ID == "" {
		a.ID = id.New(id.Attachment)
	}
	return nil
}

// EmailThread is the per-conversation email-channel state (§6.2). One row per
// conversation. Inbound mail binds to an OPEN conversation by original Sender +
// normalized SubjectKey — no token is injected into outbound mail.
type EmailThread struct {
	ConversationID string `gorm:"primaryKey;size:40"`
	TenantID       string `gorm:"size:40;not null;index"`
	ThreadToken    string `gorm:"size:64;not null;uniqueIndex"` // stable internal id (not used for threading)
	Sender         string `gorm:"size:320;index"`               // original sender, for sender+subject threading
	SubjectKey     string `gorm:"size:255;index"`               // normalized subject (truncated to 255), for threading
	Subject        string `gorm:"type:text"`                    // original subject, for display + outbound "Re:" (unbounded)
	LastAddress    string `gorm:"size:320"`
	LastMessageID  string `gorm:"size:40"`
}

// Upload is the ownership/validation record for a direct-to-bucket upload
// (review finding). An attachment may only reference a completed upload owned by
// the caller's tenant.
type Upload struct {
	ID              string       `gorm:"primaryKey;size:40"`
	TenantID        string       `gorm:"size:40;not null;index"`
	UploaderKind    MemberKind   `gorm:"size:16;not null"`
	ExternalUserID  *string      `gorm:"size:255"`
	InternalActorID *string      `gorm:"size:40"`
	ObjectKey       string       `gorm:"size:512;not null;uniqueIndex"`
	MimeType        string       `gorm:"size:255"`
	SizeBytes       int64        `gorm:"not null"`
	Filename        string       `gorm:"size:512"`
	Status          UploadStatus `gorm:"size:16;not null;default:'pending'"`
	CreatedAt       time.Time
}

func (u *Upload) BeforeCreate(*gorm.DB) error {
	if u.ID == "" {
		u.ID = id.New(id.Upload)
	}
	return nil
}
