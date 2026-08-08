// Package secrets seals tenant-supplied credentials so they are unreadable in
// the database. The key comes from the environment, never from a column: a
// backup or a replica leak yields ciphertext and nothing else.
package secrets

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
)

// ErrNoKey is returned by every operation on a Box built without a key.
var ErrNoKey = errors.New("no encryption key configured: set SILD_SECRETS_KEY to a base64 32-byte key (openssl rand -base64 32)")

// ErrWrongKey means the stored ciphertext was sealed by a different key than the
// one configured now.
var ErrWrongKey = errors.New("ciphertext was sealed with a different key")

// Box seals and opens secrets with one symmetric key. A Box with no key is
// usable but fails every call, so a deployment that never configured one gets a
// clear error at the write instead of a silently unencrypted column.
type Box struct {
	aead  cipher.AEAD
	keyID string
}

// New builds a Box from a base64-encoded 32-byte key. An empty key yields a Box
// that reports ErrNoKey — callers decide whether that is fatal.
func New(encodedKey string) (*Box, error) {
	if encodedKey == "" {
		return &Box{}, nil
	}
	key, err := base64.StdEncoding.DecodeString(encodedKey)
	if err != nil {
		return nil, fmt.Errorf("SILD_SECRETS_KEY is not valid base64: %w", err)
	}
	if len(key) != 32 {
		return nil, fmt.Errorf("SILD_SECRETS_KEY must decode to 32 bytes, got %d", len(key))
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &Box{aead: aead, keyID: fingerprint(key)}, nil
}

// Configured reports whether a key is present.
func (b *Box) Configured() bool { return b.aead != nil }

// KeyID names the key without revealing it, so a sealed row records what opens
// it and a later rotation can tell the generations apart.
func (b *Box) KeyID() string {
	if !b.Configured() {
		return ""
	}
	return b.keyID
}

// Seal encrypts plaintext, prefixing the nonce. The caller stores the result
// alongside KeyID.
func (b *Box) Seal(plaintext []byte) ([]byte, error) {
	if !b.Configured() {
		return nil, ErrNoKey
	}
	nonce := make([]byte, b.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	return b.aead.Seal(nonce, nonce, plaintext, nil), nil
}

// Open decrypts a sealed value. keyID is checked first so a key mismatch reports
// itself rather than surfacing as an opaque authentication failure.
func (b *Box) Open(sealed []byte, keyID string) ([]byte, error) {
	if !b.Configured() {
		return nil, ErrNoKey
	}
	if keyID != b.keyID {
		return nil, ErrWrongKey
	}
	if len(sealed) < b.aead.NonceSize() {
		return nil, errors.New("sealed value is truncated")
	}
	nonce, body := sealed[:b.aead.NonceSize()], sealed[b.aead.NonceSize():]
	out, err := b.aead.Open(nil, nonce, body, nil)
	if err != nil {
		return nil, ErrWrongKey
	}
	return out, nil
}

// fingerprint identifies a key by a truncated digest — enough to tell two keys
// apart, not enough to attack the key.
func fingerprint(key []byte) string {
	sum := sha256.Sum256(key)
	return hex.EncodeToString(sum[:4])
}
