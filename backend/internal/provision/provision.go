// Package provision creates the three things a new customer needs — a tenant, an
// owner who can log in, and an API key — through domain.Service so the §1
// invariants and password hashing hold. sild-admin drives it from a shell;
// sild-standalone drives the same code from bootstrap env on platforms that have
// no shell.
package provision

import (
	"context"
	"fmt"
	"strings"

	"github.com/bitllow/sild/backend/internal/domain"
	"github.com/bitllow/sild/backend/internal/store"
	"github.com/bitllow/sild/backend/internal/store/models"
)

// LeaseBootstrap serializes the empty-database bootstrap. Replicas starting
// together would otherwise each see an empty tenant table and create a tenant.
const LeaseBootstrap = "tenant-bootstrap"

// TenantSpec describes the tenant to create. AdminPassword may be empty, which
// leaves the owner without a password — an operator sets one later, or signs in
// through Google OIDC (§2.4).
type TenantSpec struct {
	Name          string
	AdminEmail    string
	AdminName     string // "Eva Marleen"; split into first/last
	AdminPassword string
	APIKeyLabel   string
}

// Result is what the operator needs to write down. APIKey is returned exactly
// once — it is stored hashed (§4.3).
type Result struct {
	TenantID          string
	AdminID           string
	APIKey            string
	ForwardingAddress string
}

// Tenant creates the tenant, its owner, and one API key.
func Tenant(ctx context.Context, svc *domain.Service, spec TenantSpec) (*Result, error) {
	// Validate everything before the first write. These steps are separate
	// transactions, so a spec rejected halfway leaves a tenant with no owner and
	// no key — and Bootstrap would then skip forever, because a tenant exists.
	if err := spec.validate(); err != nil {
		return nil, err
	}
	t, err := svc.CreateTenant(ctx, spec.Name)
	if err != nil {
		return nil, err
	}
	first, last := splitName(spec.AdminName)
	admin, err := svc.InviteAgent(ctx, t.ID, spec.AdminEmail, first, last, models.PlatformOwner)
	if err != nil {
		return nil, fmt.Errorf("create owner: %w", err)
	}
	if spec.AdminPassword != "" {
		if err := svc.SetAdminPassword(ctx, t.ID, admin.ID, spec.AdminPassword); err != nil {
			return nil, fmt.Errorf("set owner password: %w", err)
		}
	}
	// The owner is the operator who configures the product; peer conversations are
	// part of it, and only an owner can grant the access to anyone else.
	if err := svc.SetPeerAccess(ctx, t.ID, admin.ID, true); err != nil {
		return nil, fmt.Errorf("grant peer access: %w", err)
	}
	label := spec.APIKeyLabel
	if label == "" {
		label = "default"
	}
	key, _, err := svc.CreateAPIKey(ctx, t.ID, label)
	if err != nil {
		return nil, fmt.Errorf("create api key: %w", err)
	}
	res := &Result{TenantID: t.ID, AdminID: admin.ID, APIKey: key}
	if ch, err := svc.GetEmailChannel(ctx, t.ID); err == nil {
		res.ForwardingAddress = ch.ForwardingAddress
	}
	return res, nil
}

// Bootstrap creates spec's tenant only when no tenant exists yet, so restarts and
// extra replicas are no-ops. done reports whether this call created it.
//
// The empty-table condition is deliberately coarse: it makes bootstrap a
// first-run convenience for platforms with no shell, not a management API. Adding
// a second tenant is sild-admin's job.
func Bootstrap(ctx context.Context, svc *domain.Service, st store.Store, spec TenantSpec) (res *Result, done bool, err error) {
	if strings.TrimSpace(spec.Name) == "" {
		return nil, false, nil // not configured
	}
	// Cheap check first, so the common case — every restart after the first — costs
	// one SELECT instead of a lease round-trip.
	if ids, err := st.Tenants().AllIDs(ctx); err != nil || len(ids) > 0 {
		return nil, false, err
	}
	err = store.RunExclusive(ctx, st.Leases(), LeaseBootstrap, func(ctx context.Context) error {
		// Re-check under the lease: another replica may have bootstrapped between
		// our start and our turn.
		ids, err := st.Tenants().AllIDs(ctx)
		if err != nil || len(ids) > 0 {
			return err
		}
		res, err = Tenant(ctx, svc, spec)
		done = err == nil
		return err
	})
	if err != nil {
		return nil, false, err
	}
	return res, done, nil
}

// validate rejects a spec that would fail partway through provisioning. It
// duplicates the domain's own rules deliberately: the domain enforces them per
// call, and this needs them all to hold before the first call.
func (s TenantSpec) validate() error {
	if strings.TrimSpace(s.Name) == "" {
		return fmt.Errorf("tenant name is required")
	}
	if strings.TrimSpace(s.AdminEmail) == "" {
		return fmt.Errorf("admin email is required")
	}
	if s.AdminPassword != "" && len(s.AdminPassword) < domain.MinPasswordLen {
		return fmt.Errorf("admin password must be at least %d characters", domain.MinPasswordLen)
	}
	return nil
}

func splitName(full string) (first, last string) {
	first, last, _ = strings.Cut(strings.TrimSpace(full), " ")
	return first, strings.TrimSpace(last)
}
