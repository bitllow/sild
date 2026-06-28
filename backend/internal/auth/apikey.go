// Package auth holds credential primitives: API keys, user JWTs (JWS/ES256),
// JWKS, and the admin OIDC verifier. None of it imports gin — HTTP wiring lives
// in internal/middleware.
package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"strings"
)

// API key layout: sild_live_<prefix>_<secret>
//   prefix — public, indexed → O(1) lookup (no scan)
//   secret — high-entropy; only its SHA-256 is stored (§2.1)
//
// SHA-256 (not argon2id) is correct here: the secret is high-entropy, so a fast
// hash is safe and lets us authenticate every server→server request cheaply.
const (
	apiKeyEnvPrefix = "sild_live_"
	prefixBytes     = 6  // 12 hex chars
	secretBytes     = 24 // 32 base64url chars
)

// ErrMalformedKey is returned when a key string doesn't parse.
var ErrMalformedKey = errors.New("malformed api key")

// GeneratedKey is the result of minting an API key.
type GeneratedKey struct {
	Full   string // shown to the caller exactly once
	Prefix string // stored for lookup
	Hash   string // stored (sha256 hex of the secret)
}

// GenerateAPIKey mints a new key.
func GenerateAPIKey() (GeneratedKey, error) {
	pb := make([]byte, prefixBytes)
	sb := make([]byte, secretBytes)
	if _, err := rand.Read(pb); err != nil {
		return GeneratedKey{}, err
	}
	if _, err := rand.Read(sb); err != nil {
		return GeneratedKey{}, err
	}
	prefix := hex.EncodeToString(pb)
	secret := base64.RawURLEncoding.EncodeToString(sb)
	return GeneratedKey{
		Full:   apiKeyEnvPrefix + prefix + "_" + secret,
		Prefix: prefix,
		Hash:   hashSecret(secret),
	}, nil
}

// ParseAPIKey splits a key string into (prefix, secret).
func ParseAPIKey(full string) (prefix, secret string, err error) {
	rest, ok := strings.CutPrefix(full, apiKeyEnvPrefix)
	if !ok {
		return "", "", ErrMalformedKey
	}
	prefix, secret, ok = strings.Cut(rest, "_")
	if !ok || prefix == "" || secret == "" {
		return "", "", ErrMalformedKey
	}
	return prefix, secret, nil
}

// VerifySecret reports whether secret matches the stored hash (constant-time).
func VerifySecret(secret, storedHash string) bool {
	return subtle.ConstantTimeCompare([]byte(hashSecret(secret)), []byte(storedHash)) == 1
}

func hashSecret(secret string) string {
	sum := sha256.Sum256([]byte(secret))
	return hex.EncodeToString(sum[:])
}
