package auth

import (
	"crypto/rand"
	"encoding/hex"
)

// EmbedSessionPrefix marks an embedded-inbox session bearer. It is opaque, so
// the user-JWT verifier can never mistake it for one of its own tokens.
const EmbedSessionPrefix = "sild_emb_"

// NewEmbedSessionToken mints an embedded session bearer; only its hash is stored.
func NewEmbedSessionToken() (SessionToken, error) { return newSessionToken(EmbedSessionPrefix) }

// NewEmbedKey mints the public key an iframe URL names its tenant by.
func NewEmbedKey() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return "emb_" + hex.EncodeToString(b), nil
}
