package models

import (
	"time"

	"gorm.io/datatypes"
)

// ConversationArchive is the tombstone left after a conversation's hot rows are
// moved to a sink (§12). The MembersSnapshot preserves membership for archived
// read authorization after conversation_members is deleted (review finding).
type ConversationArchive struct {
	ConversationID string         `gorm:"primaryKey;size:40"`
	TenantID       string         `gorm:"size:40;not null;index"`
	Sink           ArchiveSink    `gorm:"size:16;not null"`
	SinkRef        string         `gorm:"size:1024;not null"` // BigQuery coord / bucket object key
	MessageCount   int            `gorm:"not null"`
	MembersSnapshot datatypes.JSON `gorm:"type:json"` // [{member_kind, external_user_id, internal_actor_id, conv_role}]
	ArchivedAt     time.Time
}
