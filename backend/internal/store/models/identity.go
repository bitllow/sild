package models

import "time"

// TenantIdentityProvider is how a tenant's staff sign in to the embedded inbox
// with the token their own app already gave them: Sild asks the tenant's
// validator who the token belongs to.
type TenantIdentityProvider struct {
	TenantID string `gorm:"primaryKey;size:40"`
	// EmbedKey is public: the iframe URL carries it. Never the tenant id.
	EmbedKey     string `gorm:"size:64;not null;uniqueIndex"`
	ValidatorURL string `gorm:"size:2048"`
	// ValidatorTokenSealed is the bearer Sild presents to the validator.
	ValidatorTokenSealed []byte
	ValidatorTokenKeyID  string   `gorm:"size:32"`
	AllowedOrigins       []string `gorm:"type:text;serializer:json"`

	CreatedAt time.Time
	UpdatedAt time.Time
}

// EmbedSession is a short-lived bearer session minted from an integrator token.
// ExternalSub pins it to the link it was minted through: relinking the member
// ends it.
type EmbedSession struct {
	ID          string `gorm:"primaryKey;size:64"` // sha256 of the bearer
	TenantID    string `gorm:"size:40;not null;index"`
	AdminUserID string `gorm:"size:40;not null;index"`
	ExternalSub string `gorm:"size:255;not null"`
	ExpiresAt   time.Time
	CreatedAt   time.Time
}
