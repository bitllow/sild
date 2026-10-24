package models

import (
	"time"

	"gorm.io/datatypes"
	"gorm.io/gorm"

	"github.com/bitllow/sild/backend/internal/id"
)

// RequesterAssertion records each assertion Sild signed for a data adapter:
// which service account asked which source, for whom, in which conversation.
type RequesterAssertion struct {
	ID             string `gorm:"primaryKey;size:40"`
	TenantID       string `gorm:"size:40;not null;index:idx_requester_assertion_conv,priority:1"`
	ConversationID string `gorm:"size:40;not null;index:idx_requester_assertion_conv,priority:2"`
	ActorID        string `gorm:"size:40;not null"`
	RequesterSub   string `gorm:"size:255;not null"`
	RequestID      string `gorm:"size:64;not null"`
	Source         string `gorm:"size:128;not null"`
	Params         datatypes.JSON
	CreatedAt      time.Time
}

func (a *RequesterAssertion) BeforeCreate(*gorm.DB) error {
	if a.ID == "" {
		a.ID = id.New(id.Assertion)
	}
	return nil
}
