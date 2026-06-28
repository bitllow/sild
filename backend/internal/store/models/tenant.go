package models

import (
	"time"

	"github.com/bitllow/sild/backend/internal/id"
	"gorm.io/gorm"
)

// Tenant is the top-level isolation boundary. tenant_id derives from here and
// appears on every other table (§1).
type Tenant struct {
	ID   string `gorm:"primaryKey;size:40"`
	Name string `gorm:"size:255;not null"`

	// MaxAttachmentBytes overrides the default 10MB/file upload cap (§11).
	MaxAttachmentBytes int64 `gorm:"not null;default:10485760"`
	// PushIncludeBody toggles message-body inclusion in push payloads (§5.5).
	PushIncludeBody bool `gorm:"not null;default:false"`

	CreatedAt time.Time

	SearchableKeys []TenantSearchableKey `gorm:"constraint:OnDelete:CASCADE"`
}

func (t *Tenant) BeforeCreate(*gorm.DB) error {
	if t.ID == "" {
		t.ID = id.New(id.Tenant)
	}
	return nil
}

// TenantSearchableKey replaces the Postgres text[] searchable_metadata_keys with
// a portable child table: each member-metadata key indexed for trigram search +
// UI autocomplete (§3, §4.3).
type TenantSearchableKey struct {
	TenantID string `gorm:"primaryKey;size:40"`
	Key      string `gorm:"primaryKey;size:128"`
}

// TenantEmailConfig holds per-tenant email connector settings (§6.2).
type TenantEmailConfig struct {
	TenantID      string `gorm:"primaryKey;size:40"`
	InboundDomain string `gorm:"size:255"` // address/domain inbound mail arrives on
	Provider      string `gorm:"size:32"`  // sendgrid|postmark|mailgun
	SigningSecret string `gorm:"size:255"` // provider inbound signature secret
	FromName      string `gorm:"size:255"`
	FromAddress   string `gorm:"size:255"`

	AllowedDomains []TenantEmailDomain `gorm:"foreignKey:TenantID;references:TenantID;constraint:OnDelete:CASCADE"`
}

// TenantEmailDomain is the inbound recipient-domain allowlist (§6.2).
type TenantEmailDomain struct {
	TenantID string `gorm:"primaryKey;size:40"`
	Domain   string `gorm:"primaryKey;size:255"`
}
