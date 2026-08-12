package domain

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/bitllow/sild/backend/internal/auth"
	"github.com/bitllow/sild/backend/internal/i18n"
	"github.com/bitllow/sild/backend/internal/policy"
	"github.com/bitllow/sild/backend/internal/principal"
	"github.com/bitllow/sild/backend/internal/realtime"
	"github.com/bitllow/sild/backend/internal/store"
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

// AssignRole gives a member a role they do not hold. Holding a role twice is
// refused: widening someone is an edit to the assignment they already have.
func (s *Service) AssignRole(ctx context.Context, tenantID, adminID string, role models.PlatformRole, scope models.RoleScope) error {
	return s.putAssignment(ctx, tenantID, adminID, role, scope, writeAdd)
}

// RescopeRole replaces the scope of a role the member already holds.
func (s *Service) RescopeRole(ctx context.Context, tenantID, adminID string, role models.PlatformRole, scope models.RoleScope) error {
	return s.putAssignment(ctx, tenantID, adminID, role, scope, writeRescope)
}

// SetRole leaves the member holding the role at this scope, whether or not they
// held it already — what a caller stating an end state wants (the CLI, seeds).
func (s *Service) SetRole(ctx context.Context, tenantID, adminID string, role models.PlatformRole, scope models.RoleScope) error {
	return s.putAssignment(ctx, tenantID, adminID, role, scope, writeSet)
}

// checkScope refuses a scope the role does not define, and normalises the one it
// does. Every path that stores an assignment goes through it.
func (s *Service) checkScope(ctx context.Context, tenantID string, role models.PlatformRole, scope *models.RoleScope) error {
	if err := validPlatformRole(role); err != nil {
		return err
	}
	if err := policy.ValidateScope(role, *scope); err != nil {
		return invalid(err.Error())
	}
	if role == models.PlatformTranslator {
		return s.validateTranslatorScope(ctx, tenantID, scope)
	}
	return nil
}

// validateTranslatorScope holds a grant to projects the tenant actually has and
// to language tags that mean something. A scope naming neither is a limit the
// screen shows and no decision ever matches.
func (s *Service) validateTranslatorScope(ctx context.Context, tenantID string, scope *models.RoleScope) error {
	locales := make([]string, 0, len(scope.Locales))
	for _, l := range scope.Locales {
		if l == models.ScopeAll {
			locales = append(locales, l)
			continue
		}
		n := i18n.Normalize(l)
		if !i18n.ValidLanguage(n) {
			return invalid("not a language tag: " + l)
		}
		locales = append(locales, n)
	}
	scope.Locales = locales

	named := false
	for _, p := range scope.Projects {
		if p != models.ScopeAll {
			named = true
		}
	}
	if !named {
		return nil
	}
	projects, err := s.store.Translations().ListProjects(ctx, tenantID)
	if err != nil {
		return err
	}
	have := map[string]bool{i18n.PlatformProject: true}
	for _, p := range projects {
		have[p.Slug] = true
	}
	for _, p := range scope.Projects {
		if p != models.ScopeAll && !have[p] {
			return invalid("no such project: " + p)
		}
	}
	return nil
}

// How an assignment write treats one that is already there.
type assignmentWrite int

const (
	writeAdd     assignmentWrite = iota // refuse a role the member already holds
	writeRescope                        // require the role to be held
	writeSet                            // either
)

func (s *Service) putAssignment(ctx context.Context, tenantID, adminID string, role models.PlatformRole, scope models.RoleScope, how assignmentWrite) error {
	if err := s.checkScope(ctx, tenantID, role, &scope); err != nil {
		return err
	}
	if _, err := s.store.Admins().Get(ctx, tenantID, adminID); err != nil {
		return mapStoreErr(err)
	}
	a := &models.RoleAssignment{
		TenantID: tenantID, AdminUserID: adminID, Role: role, Scope: scope,
		CreatedAt: s.now(), UpdatedAt: s.now(),
	}
	before := s.held(ctx, tenantID, adminID)
	if err := s.writeAssignment(ctx, a, how); err != nil {
		return err
	}
	s.reconcileSubscriptions(ctx, tenantID, adminID, before)
	return nil
}

// writeAssignment adds the assignment or rescopes the one there, per how.
func (s *Service) writeAssignment(ctx context.Context, a *models.RoleAssignment, how assignmentWrite) error {
	repo := s.store.RoleAssignments()
	if how == writeRescope {
		return mapStoreErr(repo.Rescope(ctx, a.TenantID, a.AdminUserID, a.Role, a.Scope))
	}
	err := repo.Create(ctx, a)
	if errors.Is(err, store.ErrDuplicate) {
		if how == writeSet {
			return mapStoreErr(repo.Rescope(ctx, a.TenantID, a.AdminUserID, a.Role, a.Scope))
		}
		return invalid("that member already holds the " + string(a.Role) + " role")
	}
	return mapStoreErr(err)
}

// held is the principal a member's assignments make, for the decisions that
// follow a write to them. nil when the rows cannot be read, so a caller does not
// mistake a failed read for an empty grant.
func (s *Service) held(ctx context.Context, tenantID, adminID string) *principal.Principal {
	rows, err := s.store.RoleAssignments().ListByAdmin(ctx, tenantID, adminID)
	if err != nil {
		return nil
	}
	return principal.ForAdmin(&models.AdminUser{ID: adminID, TenantID: tenantID}, rows)
}

// RemoveRole takes a role away. A tenant's last owner cannot be removed: only an
// owner appoints another, so losing the last one locks the tenant out for good.
func (s *Service) RemoveRole(ctx context.Context, tenantID, adminID string, role models.PlatformRole) error {
	before := s.held(ctx, tenantID, adminID)
	if err := s.store.RoleAssignments().Delete(ctx, tenantID, adminID, role); err != nil {
		if errors.Is(err, store.ErrLastOwner) {
			return invalid("the tenant must keep at least one owner")
		}
		return mapStoreErr(err)
	}
	s.reconcileSubscriptions(ctx, tenantID, adminID, before)
	return nil
}

// reconcileSubscriptions matches an open socket to what the member now holds, in
// both directions: a grant reaches a connection that is already up, and a revoke
// stops delivery to it, rather than either waiting for a reconnect. before is what
// they held prior to the write, so a write that moves nothing publishes nothing.
func (s *Service) reconcileSubscriptions(ctx context.Context, tenantID, adminID string, before *principal.Principal) {
	sub, ok := s.pub.(realtime.Subscriber)
	if !ok || before == nil {
		return
	}
	after := s.held(ctx, tenantID, adminID)
	if after == nil {
		return
	}
	if now := after.PeerAccess(); now != before.PeerAccess() {
		moveSubscription(sub, adminID, realtime.PeerChannel(tenantID), now)
	}
	// The tenant support channel follows whether any role reaches a conversation.
	was := !policy.Scope(before, policy.ConversationsList).DenyAll()
	if now := !policy.Scope(after, policy.ConversationsList).DenyAll(); now != was {
		moveSubscription(sub, adminID, realtime.AgentsChannel(tenantID), now)
	}
}

func moveSubscription(sub realtime.Subscriber, adminID, channel string, on bool) {
	if on {
		_ = sub.Subscribe(adminID, channel)
		return
	}
	_ = sub.Unsubscribe(adminID, channel)
}

// RoleAssignments lists what one member holds.
func (s *Service) RoleAssignments(ctx context.Context, tenantID, adminID string) ([]models.RoleAssignment, error) {
	as, err := s.store.RoleAssignments().ListByAdmin(ctx, tenantID, adminID)
	return as, mapStoreErr(err)
}

// TenantRoleAssignments lists every assignment in the tenant, for the Team screen.
func (s *Service) TenantRoleAssignments(ctx context.Context, tenantID string) ([]models.RoleAssignment, error) {
	as, err := s.store.RoleAssignments().ListByTenant(ctx, tenantID)
	return as, mapStoreErr(err)
}

// validPlatformRole rejects a role outside the §7 set: one no permission check
// matches would store an operator who can do nothing.
func validPlatformRole(role models.PlatformRole) error {
	if role.Valid() {
		return nil
	}
	return invalid("invalid platform role")
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
func (s *Service) InviteAgent(ctx context.Context, tenantID, email, first, last string, role models.PlatformRole, scope models.RoleScope) (*models.AdminUser, error) {
	if email == "" {
		return nil, invalid("email is required")
	}
	if role == "" {
		role = models.PlatformAgent
	}
	if err := s.checkScope(ctx, tenantID, role, &scope); err != nil {
		return nil, err
	}
	a := &models.AdminUser{TenantID: tenantID, Email: email, FirstName: first, LastName: last, CreatedAt: s.now()}
	// One transaction: a member stored without their assignment holds nothing, and
	// the invite that was supposed to give them a role has already reported failure.
	err := s.store.Tx(ctx, func(tx store.Store) error {
		if err := tx.Admins().Create(ctx, a); err != nil {
			return err
		}
		return tx.RoleAssignments().Create(ctx, &models.RoleAssignment{
			TenantID: tenantID, AdminUserID: a.ID, Role: role, Scope: scope,
			CreatedAt: s.now(), UpdatedAt: s.now(),
		})
	})
	if err != nil {
		return nil, mapStoreErr(err)
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
	return a.DisplayName()
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
