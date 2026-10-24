package store

import (
	"context"

	"github.com/bitllow/sild/backend/internal/store/models"
)

// IdentityRepo holds embedded sign-in: the tenant's integrator config, the
// member links it resolves to, and the sessions it mints.
type IdentityRepo interface {
	GetProvider(ctx context.Context, tenantID string) (*models.TenantIdentityProvider, error)
	ProviderByEmbedKey(ctx context.Context, embedKey string) (*models.TenantIdentityProvider, error)
	UpsertProvider(ctx context.Context, p *models.TenantIdentityProvider) error
	// DeleteProvider also ends every embedded session of the tenant.
	DeleteProvider(ctx context.Context, tenantID string) error

	// AdminBySub finds the member linked to an integrator identity.
	AdminBySub(ctx context.Context, tenantID, sub string) (*models.AdminUser, error)
	// SetAdminSub links (or, with nil, unlinks) a member. Any change ends the
	// member's embedded sessions.
	SetAdminSub(ctx context.Context, tenantID, adminID string, sub *string) error

	// CreateSession also drops the member's expired sessions.
	CreateSession(ctx context.Context, s *models.EmbedSession) error
	GetSession(ctx context.Context, id string) (*models.EmbedSession, error)
	DeleteSession(ctx context.Context, id string) error
}
