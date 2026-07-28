package domain

import (
	"context"
	"strings"

	"github.com/bitllow/sild/backend/internal/store/models"
)

// defaultMaxAttachmentBytes is the per-file upload cap a new tenant starts with
// (§11). Matches the column default; set here so the value holds on every
// dialect rather than depending on the DDL.
const defaultMaxAttachmentBytes = 10 << 20

// CreateTenant registers a new tenant. Until this existed the only paths to a
// tenant row were the dev seed and the test harness (§8); sild-admin drives it.
func (s *Service) CreateTenant(ctx context.Context, name string) (*models.Tenant, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, invalid("tenant name is required")
	}
	t := &models.Tenant{Name: name, MaxAttachmentBytes: defaultMaxAttachmentBytes, CreatedAt: s.now()}
	if err := s.store.Tenants().Create(ctx, t); err != nil {
		return nil, mapStoreErr(err)
	}
	return t, nil
}

// ListTenants returns every tenant. Cross-tenant by nature — the operator CLI is
// the only caller, and there is no HTTP surface for it (no root principal exists).
func (s *Service) ListTenants(ctx context.Context) ([]models.Tenant, error) {
	return s.store.Tenants().List(ctx)
}

// FindAdminByEmail resolves an admin within one tenant, so the CLI can address an
// operator by email instead of by generated id.
func (s *Service) FindAdminByEmail(ctx context.Context, tenantID, email string) (*models.AdminUser, error) {
	admins, err := s.store.Admins().FindByEmail(ctx, email)
	if err != nil {
		return nil, mapStoreErr(err)
	}
	for i := range admins {
		if admins[i].TenantID == tenantID {
			return &admins[i], nil
		}
	}
	return nil, ErrNotFound
}
