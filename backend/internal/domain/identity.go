package domain

import (
	"context"
	"errors"
	"net"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/bitllow/sild/backend/internal/auth"
	"github.com/bitllow/sild/backend/internal/secrets"
	"github.com/bitllow/sild/backend/internal/store"
	"github.com/bitllow/sild/backend/internal/store/models"
)

// EmbedSessionTTL bounds an embedded session; the host re-sends a fresh token
// well within it.
const EmbedSessionTTL = time.Hour

// Why Sild refused an embedded sign-in. API surface: the iframe shows them. A
// validator's own reasons pass through alongside these.
const (
	DenyUserNotProvisioned = "user_not_provisioned"
	DenyNoGrants           = "no_grants"
	DenyTokenExpired       = "token_expired"
)

// EmbedDenied is a token the validator vouched for, or refused, that maps to no
// usable member.
type EmbedDenied struct {
	Reason string
	Sub    string
}

func (e *EmbedDenied) Error() string {
	switch e.Reason {
	case DenyUserNotProvisioned:
		return "no team member is linked to this sign-in; ask an admin to add you"
	case DenyNoGrants:
		return "your sign-in is valid, but you have no grants in this tenant"
	}
	return "your app refused this sign-in (" + e.Reason + ")"
}

// ErrUnknownEmbedKey means no tenant publishes this embed key.
var ErrUnknownEmbedKey = errors.New("unknown embed key")

// IdentityProviderInput replaces a tenant's embedded sign-in config.
type IdentityProviderInput struct {
	ValidatorURL string `json:"validator_url"`
	// ValidatorToken nil keeps the stored one, unless the URL changes.
	ValidatorToken *string  `json:"validator_token"`
	AllowedOrigins []string `json:"allowed_origins"`
	// EmbedKey pins the public key (dev seed). Empty keeps or mints one.
	EmbedKey string `json:"-"`
}

// IdentityProviderView is the config minus its token.
type IdentityProviderView struct {
	EmbedKey          string    `json:"embed_key"`
	ValidatorURL      string    `json:"validator_url"`
	HasValidatorToken bool      `json:"has_validator_token"`
	AllowedOrigins    []string  `json:"allowed_origins"`
	UpdatedAt         time.Time `json:"updated_at"`
}

func identityView(p *models.TenantIdentityProvider) *IdentityProviderView {
	return &IdentityProviderView{
		EmbedKey: p.EmbedKey, ValidatorURL: p.ValidatorURL, HasValidatorToken: len(p.ValidatorTokenSealed) > 0,
		AllowedOrigins: nonNil(p.AllowedOrigins), UpdatedAt: p.UpdatedAt,
	}
}

func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

// GetIdentityProvider returns the tenant's embedded sign-in config.
func (s *Service) GetIdentityProvider(ctx context.Context, tenantID string) (*IdentityProviderView, error) {
	p, err := s.store.Identity().GetProvider(ctx, tenantID)
	if err != nil {
		return nil, mapStoreErr(err)
	}
	return identityView(p), nil
}

// DeleteIdentityProvider turns embedded sign-in off and ends its sessions.
func (s *Service) DeleteIdentityProvider(ctx context.Context, tenantID string) error {
	return mapStoreErr(s.store.Identity().DeleteProvider(ctx, tenantID))
}

// SetIdentityProvider validates and stores the tenant's embedded sign-in config.
func (s *Service) SetIdentityProvider(ctx context.Context, tenantID string, in IdentityProviderInput) (*IdentityProviderView, error) {
	prev, err := s.store.Identity().GetProvider(ctx, tenantID)
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		return nil, err
	}
	p := &models.TenantIdentityProvider{TenantID: tenantID}
	if p.ValidatorURL, err = validatorURL(in.ValidatorURL, s.cfg.Env); err != nil {
		return nil, invalid("validator_url: " + err.Error())
	}
	switch {
	case in.ValidatorToken != nil:
		tok := *in.ValidatorToken
		if tok == "" || len(tok) > 4096 || strings.ContainsFunc(tok, func(r rune) bool { return r <= ' ' || r == 0x7f }) {
			return nil, invalid("validator_token must be 1–4096 visible characters")
		}
		if !s.secrets.Configured() {
			return nil, invalid(secrets.ErrNoKey.Error())
		}
		if p.ValidatorTokenSealed, err = s.secrets.Seal([]byte(tok)); err != nil {
			return nil, err
		}
		p.ValidatorTokenKeyID = s.secrets.KeyID()
	// A stored token goes only to the URL it was given for.
	case prev != nil && prev.ValidatorURL == p.ValidatorURL && len(prev.ValidatorTokenSealed) > 0:
		p.ValidatorTokenSealed, p.ValidatorTokenKeyID = prev.ValidatorTokenSealed, prev.ValidatorTokenKeyID
	default:
		return nil, invalid("validator_token is required for a new validator_url")
	}
	if p.AllowedOrigins, err = normalizeOrigins(in.AllowedOrigins, s.cfg.Env == "production"); err != nil {
		return nil, err
	}
	switch {
	case in.EmbedKey != "":
		p.EmbedKey = in.EmbedKey
	case prev != nil:
		p.EmbedKey, p.CreatedAt = prev.EmbedKey, prev.CreatedAt
	default:
		if p.EmbedKey, err = auth.NewEmbedKey(); err != nil {
			return nil, err
		}
	}
	if err := s.store.Identity().UpsertProvider(ctx, p); err != nil {
		return nil, err
	}
	return identityView(p), nil
}

// validatorURL accepts an absolute https URL; plain http only to loopback in
// development and test, for a validator on the developer's machine.
func validatorURL(raw, env string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Host == "" || u.User != nil || u.Fragment != "" || (u.Scheme != "https" && u.Scheme != "http") {
		return "", errors.New("must be an absolute http(s) URL")
	}
	if len(u.String()) > 2048 {
		return "", errors.New("must be at most 2048 characters")
	}
	if u.Scheme == "http" && !((env == "development" || env == "test") && isLoopback(u.Hostname())) {
		return "", errors.New("must use https")
	}
	return u.String(), nil
}

func isLoopback(host string) bool {
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// normalizeOrigins reduces each entry to scheme://host[:port], the exact form
// postMessage and frame-ancestors compare against.
func normalizeOrigins(in []string, production bool) ([]string, error) {
	out := []string{}
	for _, raw := range in {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		u, err := url.Parse(raw)
		if err != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") ||
			(u.Path != "" && u.Path != "/") || u.RawQuery != "" || u.Fragment != "" || u.User != nil ||
			strings.Contains(u.Host, "*") {
			return nil, invalid("allowed origin " + raw + " must be scheme://host[:port]")
		}
		if production && u.Scheme != "https" {
			return nil, invalid("allowed origin " + raw + " must use https")
		}
		origin := strings.ToLower(u.Scheme + "://" + u.Host)
		if !slices.Contains(out, origin) {
			out = append(out, origin)
		}
	}
	if len(out) == 0 {
		return nil, invalid("at least one allowed origin is required")
	}
	return out, nil
}

// EmbedOrigins is what the iframe may learn about a tenant before sign-in: the
// origins allowed to frame it and to hand it a token. Nothing else.
func (s *Service) EmbedOrigins(ctx context.Context, embedKey string) ([]string, error) {
	p, err := s.store.Identity().ProviderByEmbedKey(ctx, embedKey)
	if err != nil {
		return nil, mapStoreErr(err)
	}
	return nonNil(p.AllowedOrigins), nil
}

// SetExternalSub links a member to an integrator identity; empty unlinks.
func (s *Service) SetExternalSub(ctx context.Context, tenantID, adminID, sub string) error {
	sub = strings.TrimSpace(sub)
	if len(sub) > 255 {
		return invalid("external_sub is at most 255 characters")
	}
	admin, err := s.store.Admins().Get(ctx, tenantID, adminID)
	if err != nil {
		return mapStoreErr(err)
	}
	if admin.Kind == models.AdminService {
		return invalid("a service account cannot sign in")
	}
	if sub == "" {
		return mapStoreErr(s.store.Identity().SetAdminSub(ctx, tenantID, adminID, nil))
	}
	if holder, err := s.store.Identity().AdminBySub(ctx, tenantID, sub); err == nil && holder.ID != adminID {
		return conflict("external_sub_taken", "another member is already linked to this identity")
	} else if err != nil && !errors.Is(err, store.ErrNotFound) {
		return err
	}
	return mapStoreErr(s.store.Identity().SetAdminSub(ctx, tenantID, adminID, &sub))
}

// EmbedSession is a minted embedded sign-in.
type EmbedSession struct {
	Token       string
	ExpiresAt   time.Time
	ExternalSub string
	Admin       *models.AdminUser
}

// ExchangeEmbedToken trades a token from the tenant's own app for an embedded
// session. A token the validator refuses, or that maps to no usable member, is
// *EmbedDenied; a validator that gives no usable answer is *auth.ValidatorError.
func (s *Service) ExchangeEmbedToken(ctx context.Context, embedKey, token string) (*EmbedSession, error) {
	p, err := s.store.Identity().ProviderByEmbedKey(ctx, embedKey)
	if errors.Is(err, store.ErrNotFound) {
		return nil, ErrUnknownEmbedKey
	}
	if err != nil {
		return nil, err
	}
	apiToken, err := s.secrets.Open(p.ValidatorTokenSealed, p.ValidatorTokenKeyID)
	if err != nil {
		return nil, err
	}
	v, err := s.validator.Validate(ctx, p.ValidatorURL, string(apiToken), token)
	if err != nil {
		return nil, err
	}
	if !v.Active {
		return nil, &EmbedDenied{Reason: v.Reason, Sub: v.Sub}
	}
	// Never outlive the token it was minted from: a sign-out in the host app
	// then reaches Sild within the token's own lifetime.
	now := s.now()
	exp := now.Add(EmbedSessionTTL)
	if !v.Expiry.IsZero() && v.Expiry.Before(exp) {
		exp = v.Expiry
	}
	if !exp.After(now) {
		return nil, &EmbedDenied{Reason: DenyTokenExpired, Sub: v.Sub}
	}
	admin, err := s.store.Identity().AdminBySub(ctx, p.TenantID, v.Sub)
	if errors.Is(err, store.ErrNotFound) {
		return nil, &EmbedDenied{Reason: DenyUserNotProvisioned, Sub: v.Sub}
	}
	if err != nil {
		return nil, err
	}
	roles, err := s.store.RoleAssignments().ListByAdmin(ctx, p.TenantID, admin.ID)
	if err != nil {
		return nil, err
	}
	if len(roles) == 0 {
		return nil, &EmbedDenied{Reason: DenyNoGrants, Sub: v.Sub}
	}

	tok, err := auth.NewEmbedSessionToken()
	if err != nil {
		return nil, err
	}
	if err := s.store.Identity().CreateSession(ctx, &models.EmbedSession{
		ID: tok.Hash, TenantID: p.TenantID, AdminUserID: admin.ID, ExternalSub: v.Sub,
		ExpiresAt: exp, CreatedAt: now,
	}); err != nil {
		return nil, err
	}
	return &EmbedSession{Token: tok.Raw, ExpiresAt: exp, ExternalSub: v.Sub, Admin: admin}, nil
}

// EndEmbedSession revokes an embedded session bearer.
func (s *Service) EndEmbedSession(ctx context.Context, raw string) error {
	return s.store.Identity().DeleteSession(ctx, auth.HashSessionToken(raw))
}
