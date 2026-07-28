package domain

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/bitllow/sild/backend/internal/auth"
	"github.com/bitllow/sild/backend/internal/realtime"
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

const minPasswordLen = 8

// ValidatePassword applies the password rule. Exported for callers that must
// check before they start writing, so the rule and its wording have one home.
func ValidatePassword(password string) error {
	if len(password) < minPasswordLen {
		return invalid(fmt.Sprintf("password must be at least %d characters", minPasswordLen))
	}
	return nil
}

// SetAdminPassword sets/updates an admin's password (Settings → Team).
func (s *Service) SetAdminPassword(ctx context.Context, tenantID, adminID, password string) error {
	if err := ValidatePassword(password); err != nil {
		return err
	}
	hash, err := auth.HashPassword(password)
	if err != nil {
		return err
	}
	return mapStoreErr(s.store.Admins().SetPassword(ctx, tenantID, adminID, hash))
}

// SetAdminRole updates an admin's platform role (Settings → Team, §7). Caller rules
// live in api.guardOwnerMutation; the last-owner invariant below holds for all paths.
func (s *Service) SetAdminRole(ctx context.Context, tenantID, adminID string, role models.PlatformRole) error {
	if err := validPlatformRole(role); err != nil {
		return err
	}
	// A tenant must keep an owner: only owners can grant peer access or appoint
	// another owner, so demoting the last one locks the tenant out irreversibly.
	if role != models.PlatformOwner {
		current, err := s.store.Admins().Get(ctx, tenantID, adminID)
		if err != nil {
			return mapStoreErr(err)
		}
		if current.PlatformRole == models.PlatformOwner && !s.hasOtherOwner(ctx, tenantID, adminID) {
			return invalid("the tenant must keep at least one owner")
		}
	}
	return mapStoreErr(s.store.Admins().SetRole(ctx, tenantID, adminID, role))
}

// validPlatformRole rejects a role outside the §7 set: one no permission check
// matches would store an operator who can do nothing.
func validPlatformRole(role models.PlatformRole) error {
	switch role {
	case models.PlatformOwner, models.PlatformAdmin, models.PlatformAgent:
		return nil
	}
	return invalid("invalid platform role")
}

// hasOtherOwner reports whether another owner exists. Rosters are small, so this
// lists rather than adding a counting query.
func (s *Service) hasOtherOwner(ctx context.Context, tenantID, exceptID string) bool {
	admins, err := s.store.Admins().List(ctx, tenantID)
	if err != nil {
		return false // can't prove another owner exists — refuse the demotion
	}
	for i := range admins {
		if admins[i].ID != exceptID && admins[i].PlatformRole == models.PlatformOwner {
			return true
		}
	}
	return false
}

// SetPeerAccess toggles an operator's access to peer conversations (Settings →
// Team). Per-user, independent of platform role.
func (s *Service) SetPeerAccess(ctx context.Context, tenantID, adminID string, peerAccess bool) error {
	if err := s.store.Admins().SetPeerAccess(ctx, tenantID, adminID, peerAccess); err != nil {
		return mapStoreErr(err)
	}
	// Reconcile the operator's LIVE realtime subscriptions so the change takes
	// effect at once: a revoke stops peer message delivery to an already-open
	// connection immediately, and a grant starts it — otherwise the peer channel
	// set is only re-derived at reconnect (agentSubscriptions).
	s.reconcilePeerSubscriptions(ctx, tenantID, adminID, peerAccess)
	return nil
}

// reconcilePeerSubscriptions adds or removes the operator's server-side
// subscription to the tenant peer channel, matching a peer_access change on a
// still-connected inbox socket. One channel carries the whole peer surface, so
// this is a single broker call regardless of how many peer conversations exist.
// Best-effort (the realtime layer may not support live subscription changes —
// tests/workers — and reconnect re-derives anyway).
func (s *Service) reconcilePeerSubscriptions(ctx context.Context, tenantID, adminID string, grant bool) {
	sub, ok := s.pub.(realtime.Subscriber)
	if !ok {
		return
	}
	ch := realtime.PeerChannel(tenantID)
	if grant {
		_ = sub.Subscribe(adminID, ch)
	} else {
		_ = sub.Unsubscribe(adminID, ch)
	}
}

// GetAdmin loads a single admin user (Settings → Team, /admin/me).
func (s *Service) GetAdmin(ctx context.Context, tenantID, adminID string) (*models.AdminUser, error) {
	a, err := s.store.Admins().Get(ctx, tenantID, adminID)
	return a, mapStoreErr(err)
}

// Logout revokes the session behind a raw cookie value.
func (s *Service) Logout(ctx context.Context, raw string) error {
	return s.store.Admins().DeleteSession(ctx, auth.HashSessionToken(raw))
}

// InviteAgent adds an admin_user to the tenant (Settings → Team, §8). first/last
// are optional display-name parts; the first name is what end-users see as the
// agent's reply name on the messenger surfaces.
func (s *Service) InviteAgent(ctx context.Context, tenantID, email, first, last string, role models.PlatformRole) (*models.AdminUser, error) {
	if email == "" {
		return nil, invalid("email is required")
	}
	if role == "" {
		role = models.PlatformAgent
	}
	if err := validPlatformRole(role); err != nil {
		return nil, err
	}
	a := &models.AdminUser{TenantID: tenantID, Email: email, FirstName: first, LastName: last, PlatformRole: role, CreatedAt: s.now()}
	if err := s.store.Admins().Create(ctx, a); err != nil {
		return nil, err
	}
	return a, nil
}

// AgentDisplayName resolves an agent actor id to a short display name for
// end-user–facing surfaces (the widget shows this instead of "Support"). Falls
// back to the email local-part, then "" when the actor can't be resolved.
func (s *Service) AgentDisplayName(ctx context.Context, tenantID, actorID string) string {
	a, err := s.store.Admins().Get(ctx, tenantID, actorID)
	if err != nil || a == nil {
		return ""
	}
	if a.FirstName != "" {
		return a.FirstName
	}
	if i := strings.IndexByte(a.Email, '@'); i > 0 {
		return a.Email[:i]
	}
	return a.Email
}

// ListAdmins returns the tenant's admin users (Settings → Team, §8).
func (s *Service) ListAdmins(ctx context.Context, tenantID string) ([]models.AdminUser, error) {
	return s.store.Admins().List(ctx, tenantID)
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

// SetWebhookActive enables/disables delivery for an endpoint (§6.1).
func (s *Service) SetWebhookActive(ctx context.Context, tenantID, id string, active bool) error {
	return mapStoreErr(s.store.Webhooks().SetActive(ctx, tenantID, id, active))
}

func (s *Service) ListDeliveries(ctx context.Context, tenantID, endpointID string) ([]models.WebhookDelivery, error) {
	return s.store.Webhooks().ListDeliveries(ctx, tenantID, endpointID)
}
