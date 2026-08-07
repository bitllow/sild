// Package provision creates the three things a new customer needs — a tenant, an
// owner who can log in, and an API key — through domain.Service so the §1
// invariants and password hashing hold. sild-admin drives it from a shell;
// sild-standalone drives it from bootstrap env where there is no shell.
package provision

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/bitllow/sild/backend/internal/domain"
	"github.com/bitllow/sild/backend/internal/store"
	"github.com/bitllow/sild/backend/internal/store/models"
)

// leaseBootstrap serializes the empty-database bootstrap, so replicas starting
// together do not each create a tenant. The TTL bounds one holder.
const (
	leaseBootstrap = "tenant-bootstrap"
	leaseTTL       = 2 * time.Minute
)

// TenantSpec describes the tenant to create. An empty AdminPassword leaves the
// owner without one — set it later, or sign in through Google OIDC (§2.4).
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
	// Validate before the first write: these steps are separate transactions, so a
	// spec rejected halfway leaves a tenant with no owner and no key, which
	// Bootstrap then reads as "already done".
	if err := spec.validate(); err != nil {
		return nil, err
	}
	t, err := svc.CreateTenant(ctx, spec.Name)
	if err != nil {
		return nil, err
	}
	first, last := SplitName(spec.AdminName)
	admin, err := svc.InviteAgent(ctx, t.ID, spec.AdminEmail, first, last, models.PlatformOwner)
	if err != nil {
		return nil, fmt.Errorf("create owner: %w", err)
	}
	if spec.AdminPassword != "" {
		if err := svc.SetAdminPassword(ctx, t.ID, admin.ID, spec.AdminPassword); err != nil {
			return nil, fmt.Errorf("set owner password: %w", err)
		}
	}
	// Only an owner can grant peer access to anyone else, so the first one gets it.
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

// Bootstrap creates spec's tenant only while no tenant exists, so restarts and
// extra replicas are no-ops. A nil Result means this call did not create it.
func Bootstrap(ctx context.Context, svc *domain.Service, st store.Store, spec TenantSpec) (*Result, error) {
	if strings.TrimSpace(spec.Name) == "" {
		return nil, nil // not configured
	}
	empty, err := noTenants(ctx, st)
	if err != nil || !empty {
		return nil, err
	}
	var res *Result
	// Skip rather than wait when another replica holds the lease: this runs before
	// the listener binds, and blocking here would fail the startup probe.
	_, err = store.RunLeased(ctx, st.Leases(), leaseBootstrap, leaseTTL, func(ctx context.Context) error {
		empty, err := noTenants(ctx, st) // another replica may have won the race
		if err != nil || !empty {
			return err
		}
		res, err = Tenant(ctx, svc, spec)
		return err
	})
	if err != nil {
		return nil, err
	}
	return res, nil
}

func noTenants(ctx context.Context, st store.Store) (bool, error) {
	exists, err := st.Tenants().Exists(ctx)
	return !exists, err
}

func (s TenantSpec) validate() error {
	if strings.TrimSpace(s.AdminEmail) == "" {
		return fmt.Errorf("admin email is required")
	}
	if s.AdminPassword == "" {
		return nil
	}
	return domain.ValidatePassword(s.AdminPassword)
}

// SplitName splits a display name into first and last parts.
func SplitName(full string) (first, last string) {
	first, last, _ = strings.Cut(strings.TrimSpace(full), " ")
	return first, strings.TrimSpace(last)
}
