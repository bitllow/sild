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
	// PushIncludeSender toggles naming the sender. Separate from the body so a
	// tenant can show who wrote without showing what.
	PushIncludeSender bool `gorm:"not null;default:false"`
	// PushSenderSource picks whose name a support nudge carries: the brand, or
	// the agent who replied. Peer nudges always name the member.
	PushSenderSource PushSenderSource `gorm:"size:16;not null;default:'brand'"`

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

	// InboundToken is the local part of this tenant's forwarding address
	// (<token>@<config inbound domain>). Orgs forward their support mailbox to it
	// and the sild-mail daemon resolves the tenant by this token (§6.2).
	InboundToken string `gorm:"size:64;index"`
	// Verified flips true the first time an email is ingested through the
	// forwarding address — proof the org's forwarding is wired correctly.
	Verified bool `gorm:"not null;default:false"`
	// AutoReply sends an acknowledgement when inbound mail opens a conversation.
	AutoReply bool `gorm:"not null;default:false"`
	// SpamFilter drops autoresponder/bounce mail (out-of-office, mailer-daemon).
	// The DB default keeps AutoMigrate safe when adding the column to existing
	// non-empty tables, and defaults the filter on. SetEmailConfig writes the
	// booleans explicitly so a disabled filter still persists (GORM otherwise
	// omits a false value that matches a non-zero default).
	SpamFilter bool `gorm:"not null;default:true"`

	AllowedDomains []TenantEmailDomain `gorm:"foreignKey:TenantID;references:TenantID;constraint:OnDelete:CASCADE"`
}

// TenantPushConfig is a tenant's credential for their own push project (§5.5) —
// only its owner can address that app. Sealed with a key from the environment;
// CredentialKeyID names which one, so a rotation needs no migration.
type TenantPushConfig struct {
	TenantID string `gorm:"primaryKey;size:40"`
	// ProjectID and ClientEmail are the non-secret identity of the credential,
	// shown back to the tenant so they can confirm which project is wired up.
	ProjectID   string `gorm:"size:255;not null"`
	ClientEmail string `gorm:"size:255;not null"`

	// No explicit column type: a named one is passed through verbatim, and `blob`
	// does not exist on postgres.
	CredentialSealed []byte `gorm:"not null"`
	CredentialKeyID  string `gorm:"size:32;not null"`

	// Verified flips true the first time a nudge is delivered — proof the whole
	// path works, not just that the credential parsed.
	Verified  bool `gorm:"not null;default:false"`
	CreatedAt time.Time
	UpdatedAt time.Time
}

// TenantEmailDomain is the inbound recipient-domain allowlist (§6.2).
type TenantEmailDomain struct {
	TenantID string `gorm:"primaryKey;size:40"`
	Domain   string `gorm:"primaryKey;size:255"`
}
