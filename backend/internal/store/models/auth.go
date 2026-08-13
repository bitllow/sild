package models

import (
	"strings"
	"time"

	"github.com/bitllow/sild/backend/internal/id"
	"gorm.io/gorm"
)

// APIKey is a server↔server credential (§2.1). The secret is SHA-256 hashed and
// never retrievable. Prefix is a public, indexed lookup component so verification
// is O(1) (no scan): key string is sild_live_<prefix>_<secret>.
type APIKey struct {
	ID       string `gorm:"primaryKey;size:40"`
	TenantID string `gorm:"size:40;not null;index:idx_apikey_tenant"`
	Prefix   string `gorm:"size:24;not null;uniqueIndex"` // lookup key
	Hash     string `gorm:"size:128;not null"`            // sha256 hex of the secret part
	Label    string `gorm:"size:255"`
	// Scope limits what this key may reach, in the same document a role assignment
	// carries. Empty is a tenant-wide key — what every key minted before scopes
	// existed is, and still means.
	Scope     RoleScope `gorm:"serializer:json"`
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
	ID       string `gorm:"primaryKey;size:40"`
	TenantID string `gorm:"size:40;not null;uniqueIndex:idx_admin_tenant_email"`
	Email    string `gorm:"size:320;not null;uniqueIndex:idx_admin_tenant_email"`
	// FirstName/LastName are the operator's display name (Settings → Team). The
	// first name is surfaced to end-users on the messenger surfaces (web widget +
	// SDK) as the agent's reply name, in place of the generic "Support".
	FirstName string `gorm:"size:120"`
	LastName  string `gorm:"size:120"`
	// PasswordHash is set when the admin uses email/password login (§2.4
	// alternative to Google OIDC); nil for OIDC-only admins.
	PasswordHash *string `gorm:"size:255"`
	// Locale is the language this operator reads. Empty means unknown.
	Locale    string `gorm:"size:16"`
	CreatedAt time.Time
}

func (a *AdminUser) BeforeCreate(*gorm.DB) error {
	if a.ID == "" {
		a.ID = id.New(id.AdminUser)
	}
	return nil
}

// DisplayName is what end-user surfaces call this operator. One rule, so a
// notification and the widget never name the same agent differently.
func (a *AdminUser) DisplayName() string {
	if a.FirstName != "" {
		return a.FirstName
	}
	if i := strings.IndexByte(a.Email, '@'); i > 0 {
		return a.Email[:i]
	}
	return a.Email
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
