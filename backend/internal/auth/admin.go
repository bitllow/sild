package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"

	"github.com/bitllow/sild/backend/internal/config"
)

// AdminAuthenticator abstracts the admin identity provider (§2.4). Google OIDC
// is the production impl; a dev stub keeps the inbox usable without Google.
type AdminAuthenticator interface {
	// LoginURL returns the provider redirect URL for the given opaque state.
	LoginURL(state string) string
	// Resolve exchanges the callback code for the verified admin email.
	Resolve(ctx context.Context, code string) (email string, err error)
	// IsStub reports whether this is the dev (non-OIDC) authenticator.
	IsStub() bool
}

// NewAdminAuthenticator picks Google OIDC when configured, else the dev stub
// (non-production only). dig provides the result.
func NewAdminAuthenticator(cfg *config.Config) AdminAuthenticator {
	if cfg.Auth.GoogleClientID != "" && cfg.Auth.GoogleClientSecret != "" {
		return newGoogleAuthenticator(cfg.Auth)
	}
	return devAuthenticator{}
}

// devAuthenticator trusts the supplied code as the email. Dev/test only — the
// handler must refuse to mount it in production.
type devAuthenticator struct{}

func (devAuthenticator) LoginURL(state string) string { return "/v1/admin/auth/google/dev?state=" + state }
func (devAuthenticator) Resolve(_ context.Context, code string) (string, error) {
	if code == "" {
		return "", ErrInvalidToken
	}
	return code, nil // code IS the email in dev
}
func (devAuthenticator) IsStub() bool { return true }

// SessionToken is an opaque admin session credential: the raw value goes in the
// cookie, only its hash is stored (so a DB leak can't mint cookies).
type SessionToken struct {
	Raw  string // set as the cookie value
	Hash string // stored as AdminSession.ID
}

// NewSessionToken mints a random admin session token.
func NewSessionToken() (SessionToken, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return SessionToken{}, err
	}
	raw := hex.EncodeToString(b)
	return SessionToken{Raw: raw, Hash: HashSessionToken(raw)}, nil
}

// HashSessionToken hashes a raw cookie value for DB lookup.
func HashSessionToken(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}
