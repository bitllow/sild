package models

import (
	"time"

	"github.com/bitllow/sild/backend/internal/id"
	"gorm.io/gorm"
)

// APIKey is a server↔server credential (§2.1). The secret is SHA-256 hashed and
// never retrievable. Prefix is a public, indexed lookup component so verification
// is O(1) (no scan): key string is sild_live_<prefix>_<secret>.
type APIKey struct {
	ID        string     `gorm:"primaryKey;size:40"`
	TenantID  string     `gorm:"size:40;not null;index:idx_apikey_tenant"`
	Prefix    string     `gorm:"size:24;not null;uniqueIndex"` // lookup key
	Hash      string     `gorm:"size:128;not null"`            // sha256 hex of the secret part
	Label     string     `gorm:"size:255"`
	RevokedAt *time.Time
	CreatedAt time.Time
}

func (k *APIKey) BeforeCreate(*gorm.DB) error {
	if k.ID == "" {
		k.ID = id.New(id.APIKey)
	}
	return nil
}

// Active reports whether the key may still authenticate.
func (k *APIKey) Active() bool { return k.RevokedAt == nil }

// AdminUser is an inbox operator (§2.4, §7). Separate identity space from chat
// end-users; authenticated via Google OIDC.
type AdminUser struct {
	ID           string       `gorm:"primaryKey;size:40"`
	TenantID     string       `gorm:"size:40;not null;uniqueIndex:idx_admin_tenant_email"`
	Email        string       `gorm:"size:320;not null;uniqueIndex:idx_admin_tenant_email"`
	PlatformRole PlatformRole `gorm:"size:16;not null"`
	// PasswordHash is set when the admin uses email/password login (§2.4
	// alternative to Google OIDC); nil for OIDC-only admins.
	PasswordHash *string `gorm:"size:255"`
	CreatedAt    time.Time
}

func (a *AdminUser) BeforeCreate(*gorm.DB) error {
	if a.ID == "" {
		a.ID = id.New(id.AdminUser)
	}
	return nil
}

// AdminSession is a server-side admin cookie session (revocable).
type AdminSession struct {
	ID          string `gorm:"primaryKey;size:64"` // opaque cookie value (hashed at rest)
	TenantID    string `gorm:"size:40;not null;index"`
	AdminUserID string `gorm:"size:40;not null;index"`
	ExpiresAt   time.Time
	CreatedAt   time.Time
}

// SigningKey is a platform-level JWT signing keypair (§2.2, §2.5). Keys are
// global (the platform issues all user JWTs); rotation publishes via JWKS.
type SigningKey struct {
	Kid        string `gorm:"primaryKey;size:64"`
	Algorithm  string `gorm:"size:16;not null"` // ES256 | EdDSA
	PrivatePEM string `gorm:"type:text;not null"`
	PublicPEM  string `gorm:"type:text;not null"`
	Active     bool   `gorm:"not null;default:true;index"` // the current signing key
	CreatedAt  time.Time
	RetiredAt  *time.Time
}
