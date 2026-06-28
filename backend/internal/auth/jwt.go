package auth

import (
	"context"
	"crypto/ecdsa"
	"errors"
	"sync"
	"time"

	"github.com/bitllow/sild/backend/internal/config"
	"github.com/bitllow/sild/backend/internal/id"
	"github.com/bitllow/sild/backend/internal/store"
	"github.com/bitllow/sild/backend/internal/store/models"
	"github.com/golang-jwt/jwt/v5"
)

// ErrInvalidToken is returned when a JWT fails verification or carries the wrong
// claims (§2.2: typ must be "user", tid must be present).
var ErrInvalidToken = errors.New("invalid token")

// Claims is the user-JWT claim set (§2.2). Authorization is NOT encoded here —
// it is resolved live from membership on each request (§2.2).
type Claims struct {
	Tid string `json:"tid"`
	Typ string `json:"typ"`
	jwt.RegisteredClaims
}

// KeyManager mints and verifies user JWTs and serves JWKS, backed by signing
// keys in the store. Public keys are cached by kid (immutable once created).
type KeyManager struct {
	keys store.SigningKeyRepo
	cfg  config.Auth

	mu     sync.RWMutex
	pubCar map[string]*ecdsa.PublicKey
}

// NewKeyManager constructs a KeyManager. dig provides it.
func NewKeyManager(st store.Store, cfg *config.Config) *KeyManager {
	return &KeyManager{keys: st.SigningKeys(), cfg: cfg.Auth, pubCar: map[string]*ecdsa.PublicKey{}}
}

// EnsureActiveKey bootstraps a signing key if none exists (rotation-friendly).
func (m *KeyManager) EnsureActiveKey(ctx context.Context) error {
	if _, err := m.keys.Active(ctx); err == nil {
		return nil
	} else if !errors.Is(err, store.ErrNotFound) {
		return err
	}
	priv, pub, err := GenerateES256()
	if err != nil {
		return err
	}
	return m.keys.Create(ctx, &models.SigningKey{
		Kid:        id.New("sk"),
		Algorithm:  "ES256",
		PrivatePEM: priv,
		PublicPEM:  pub,
		Active:     true,
		CreatedAt:  time.Now(),
	})
}

// Mint issues a user JWT for sub within tenant tid (§2.3).
func (m *KeyManager) Mint(ctx context.Context, sub, tid string, ttl time.Duration) (string, time.Time, error) {
	return m.mint(ctx, sub, tid, "user", ttl)
}

// MintAgent issues an agent JWT for an admin actor (typ="agent"), used only to
// authenticate the inbox's egress-only realtime connection (§5). It carries no
// authorization — the realtime node resolves an agent's channel set live from
// the assignment queue on connect.
func (m *KeyManager) MintAgent(ctx context.Context, adminID, tid string, ttl time.Duration) (string, time.Time, error) {
	return m.mint(ctx, adminID, tid, "agent", ttl)
}

func (m *KeyManager) mint(ctx context.Context, sub, tid, typ string, ttl time.Duration) (string, time.Time, error) {
	sk, err := m.keys.Active(ctx)
	if err != nil {
		return "", time.Time{}, err
	}
	priv, err := parseECPrivate(sk.PrivatePEM)
	if err != nil {
		return "", time.Time{}, err
	}
	now := time.Now()
	exp := now.Add(ttl)
	claims := Claims{
		Tid: tid,
		Typ: typ,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    m.cfg.Issuer,
			Subject:   sub,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(exp),
		},
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodES256, claims)
	tok.Header["kid"] = sk.Kid
	signed, err := tok.SignedString(priv)
	return signed, exp, err
}

// Verify parses and validates a user JWT, returning its claims (§2.2).
func (m *KeyManager) Verify(ctx context.Context, token string) (*Claims, error) {
	claims := &Claims{}
	_, err := jwt.ParseWithClaims(token, claims, func(t *jwt.Token) (any, error) {
		kid, _ := t.Header["kid"].(string)
		return m.publicKey(ctx, kid)
	},
		jwt.WithValidMethods([]string{"ES256"}),
		jwt.WithIssuer(m.cfg.Issuer),
		jwt.WithExpirationRequired(),
	)
	if err != nil {
		return nil, err
	}
	if claims.Typ != "user" || claims.Tid == "" || claims.Subject == "" {
		return nil, ErrInvalidToken
	}
	return claims, nil
}

// VerifyRealtime validates a token used to open the egress-only realtime
// connection (§5). It accepts both user and agent tokens; the node branches on
// claims.Typ to compute the channel set. REST routes keep using Verify (strict
// user-only).
func (m *KeyManager) VerifyRealtime(ctx context.Context, token string) (*Claims, error) {
	claims := &Claims{}
	_, err := jwt.ParseWithClaims(token, claims, func(t *jwt.Token) (any, error) {
		kid, _ := t.Header["kid"].(string)
		return m.publicKey(ctx, kid)
	},
		jwt.WithValidMethods([]string{"ES256"}),
		jwt.WithIssuer(m.cfg.Issuer),
		jwt.WithExpirationRequired(),
	)
	if err != nil {
		return nil, err
	}
	if (claims.Typ != "user" && claims.Typ != "agent") || claims.Tid == "" || claims.Subject == "" {
		return nil, ErrInvalidToken
	}
	return claims, nil
}

func (m *KeyManager) publicKey(ctx context.Context, kid string) (*ecdsa.PublicKey, error) {
	if kid == "" {
		return nil, ErrInvalidToken
	}
	m.mu.RLock()
	if pk, ok := m.pubCar[kid]; ok {
		m.mu.RUnlock()
		return pk, nil
	}
	m.mu.RUnlock()

	sk, err := m.keys.GetByKid(ctx, kid)
	if err != nil {
		return nil, ErrInvalidToken
	}
	pk, err := parseECPublic(sk.PublicPEM)
	if err != nil {
		return nil, err
	}
	m.mu.Lock()
	m.pubCar[kid] = pk
	m.mu.Unlock()
	return pk, nil
}
