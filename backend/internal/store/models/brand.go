package models

import (
	"time"

	"github.com/bitllow/sild/backend/internal/id"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

// Brand is one named messenger look for a tenant (Settings → Appearance). A
// tenant may keep several brands and mark exactly one Active; the active brand's
// Config is what ships to the web drop-in widget AND the native SDK messengers
// (same brand config drives both surfaces).
//
// Config is stored as an opaque JSON blob (the design's per-brand config object:
// brand color, theme, font, radius, launcher, welcome copy, toggles, plus
// logo/icon data URLs). Keeping it as JSON lets the config shape evolve without a
// migration; the domain layer owns validation/normalization.
type Brand struct {
	ID       string `gorm:"primaryKey;size:40"`
	TenantID string `gorm:"size:40;not null;index"`
	Name     string `gorm:"size:255;not null"`
	// Position preserves the admin-facing ordering of the brand switcher.
	Position int            `gorm:"not null;default:0"`
	Active   bool           `gorm:"not null;default:false"`
	Config   datatypes.JSON `gorm:"type:json;not null"`

	CreatedAt time.Time
	UpdatedAt time.Time
}

func (b *Brand) BeforeCreate(*gorm.DB) error {
	if b.ID == "" {
		b.ID = id.New(id.Brand)
	}
	return nil
}
