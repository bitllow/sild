package domain

import (
	"context"
	"time"

	"github.com/bitllow/sild/backend/internal/auth"
	"github.com/bitllow/sild/backend/internal/store/models"
)

// CreateSession resolves an authenticated admin email to an admin_user and mints
// a server-side session (§2.4). When the email maps to admins in multiple
// tenants, the most recently created is used (single-tenant is the common case;
// multi-tenant selection is a future enhancement — review finding).
func (s *Service) CreateSession(ctx context.Context, email string) (raw string, expires time.Time, err error) {
	admins, err := s.store.Admins().FindByEmail(ctx, email)
	if err != nil {
		return "", time.Time{}, err
	}
	if len(admins) == 0 {
		return "", time.Time{}, ErrForbidden // not an admin in any tenant
	}
	return s.mintSession(ctx, &admins[0])
}

// mintSession creates a server-side session for an admin and returns the raw
// cookie value.
func (s *Service) mintSession(ctx context.Context, admin *models.AdminUser) (string, time.Time, error) {
	tok, err := auth.NewSessionToken()
	if err != nil {
		return "", time.Time{}, err
	}
	exp := s.now().Add(time.Duration(s.cfg.Auth.AdminSessionTTLHours) * time.Hour)
	if err := s.store.Admins().CreateSession(ctx, &models.AdminSession{
		ID: tok.Hash, TenantID: admin.TenantID, AdminUserID: admin.ID,
		ExpiresAt: exp, CreatedAt: s.now(),
	}); err != nil {
		return "", time.Time{}, err
	}
	return tok.Raw, exp, nil
}

// CreateSessionWithPassword authenticates an admin by email + password (§2.4
// alternative to Google OIDC) and mints a session. Checks the password against
// every admin with that email (emails may repeat across tenants).
func (s *Service) CreateSessionWithPassword(ctx context.Context, email, password string) (raw string, expires time.Time, err error) {
	admins, err := s.store.Admins().FindByEmail(ctx, email)
	if err != nil {
		return "", time.Time{}, err
	}
	for i := range admins {
		a := &admins[i]
		if a.PasswordHash != nil && auth.CheckPassword(*a.PasswordHash, password) {
			return s.mintSession(ctx, a)
		}
	}
	return "", time.Time{}, ErrForbidden // no matching credential
}

// SetAdminPassword sets/updates an admin's password (Settings → Team).
func (s *Service) SetAdminPassword(ctx context.Context, tenantID, adminID, password string) error {
	if len(password) < 8 {
		return invalid("password must be at least 8 characters")
	}
	hash, err := auth.HashPassword(password)
	if err != nil {
		return err
	}
	return mapStoreErr(s.store.Admins().SetPassword(ctx, tenantID, adminID, hash))
}

// Logout revokes the session behind a raw cookie value.
func (s *Service) Logout(ctx context.Context, raw string) error {
	return s.store.Admins().DeleteSession(ctx, auth.HashSessionToken(raw))
}

// InviteAgent adds an admin_user to the tenant (Settings → Team, §8).
func (s *Service) InviteAgent(ctx context.Context, tenantID, email string, role models.PlatformRole) (*models.AdminUser, error) {
	if email == "" {
		return nil, invalid("email is required")
	}
	if role == "" {
		role = models.PlatformAgent
	}
	a := &models.AdminUser{TenantID: tenantID, Email: email, PlatformRole: role, CreatedAt: s.now()}
	if err := s.store.Admins().Create(ctx, a); err != nil {
		return nil, err
	}
	return a, nil
}

// CreateAPIKey mints a tenant API key; the secret is returned exactly once (§4.3).
func (s *Service) CreateAPIKey(ctx context.Context, tenantID, label string) (full string, rec *models.APIKey, err error) {
	gen, err := auth.GenerateAPIKey()
	if err != nil {
		return "", nil, err
	}
	rec = &models.APIKey{
		TenantID: tenantID, Prefix: gen.Prefix, Hash: gen.Hash,
		Label: label, CreatedAt: s.now(),
	}
	if err := s.store.APIKeys().Create(ctx, rec); err != nil {
		return "", nil, err
	}
	return gen.Full, rec, nil
}

func (s *Service) ListAPIKeys(ctx context.Context, tenantID string) ([]models.APIKey, error) {
	return s.store.APIKeys().ListByTenant(ctx, tenantID)
}

func (s *Service) RevokeAPIKey(ctx context.Context, tenantID, id string) error {
	return mapStoreErr(s.store.APIKeys().Revoke(ctx, tenantID, id))
}

// CreateWebhook registers an outbound webhook with a generated signing secret (§6.1).
func (s *Service) CreateWebhook(ctx context.Context, tenantID, url string, events []string) (*models.WebhookEndpoint, error) {
	if url == "" {
		return nil, invalid("url is required")
	}
	secret, err := auth.NewSessionToken() // reuse: a 64-hex random secret
	if err != nil {
		return nil, err
	}
	ep := &models.WebhookEndpoint{TenantID: tenantID, URL: url, Secret: secret.Raw, Active: true, CreatedAt: s.now()}
	for _, e := range events {
		ep.Events = append(ep.Events, models.WebhookEvent{TenantID: tenantID, Event: e})
	}
	if err := s.store.Webhooks().Create(ctx, ep); err != nil {
		return nil, err
	}
	return ep, nil
}

func (s *Service) ListWebhooks(ctx context.Context, tenantID string) ([]models.WebhookEndpoint, error) {
	return s.store.Webhooks().List(ctx, tenantID)
}

func (s *Service) DeleteWebhook(ctx context.Context, tenantID, id string) error {
	return mapStoreErr(s.store.Webhooks().Delete(ctx, tenantID, id))
}

func (s *Service) ListDeliveries(ctx context.Context, tenantID, endpointID string) ([]models.WebhookDelivery, error) {
	return s.store.Webhooks().ListDeliveries(ctx, tenantID, endpointID)
}
