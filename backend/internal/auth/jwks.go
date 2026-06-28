package auth

import (
	"context"
	"encoding/base64"
)

// JWK is a single JSON Web Key (EC P-256 public).
type JWK struct {
	Kty string `json:"kty"`
	Crv string `json:"crv"`
	X   string `json:"x"`
	Y   string `json:"y"`
	Kid string `json:"kid"`
	Use string `json:"use"`
	Alg string `json:"alg"`
}

// JWKS is the JWK set served at /.well-known/jwks.json (§2.5).
type JWKS struct {
	Keys []JWK `json:"keys"`
}

// JWKS returns the public keys clients use to verify user JWTs.
func (m *KeyManager) JWKS(ctx context.Context) (JWKS, error) {
	keys, err := m.keys.Published(ctx)
	if err != nil {
		return JWKS{}, err
	}
	set := JWKS{Keys: make([]JWK, 0, len(keys))}
	for _, k := range keys {
		pub, err := parseECPublic(k.PublicPEM)
		if err != nil {
			continue
		}
		set.Keys = append(set.Keys, JWK{
			Kty: "EC",
			Crv: "P-256",
			X:   base64.RawURLEncoding.EncodeToString(pub.X.FillBytes(make([]byte, 32))),
			Y:   base64.RawURLEncoding.EncodeToString(pub.Y.FillBytes(make([]byte, 32))),
			Kid: k.Kid,
			Use: "sig",
			Alg: "ES256",
		})
	}
	return set, nil
}
