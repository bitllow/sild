package store

import (
	"context"

	"github.com/bitllow/sild/backend/internal/store/models"
)

// TranslationRepo persists a tenant's overrides and the releases cut from them.
// The platform project's keys are not here — they ship in the binary.
type TranslationRepo interface {
	GetProject(ctx context.Context, tenantID, project string) (*models.TranslationProject, error)
	// SaveProject writes the settings and replaces the enabled locale set whole.
	SaveProject(ctx context.Context, p *models.TranslationProject, locales []string) error
	ProjectLocales(ctx context.Context, tenantID, project string) ([]string, error)

	// Overrides returns every draft override for a project, or just one locale's.
	Overrides(ctx context.Context, tenantID, project string) ([]models.TranslationOverride, error)
	LocaleOverrides(ctx context.Context, tenantID, project, locale string) ([]models.TranslationOverride, error)
	PutOverride(ctx context.Context, o *models.TranslationOverride) error
	DeleteOverride(ctx context.Context, tenantID, project, locale, key string) error

	LatestRelease(ctx context.Context, tenantID, project string) (*models.TranslationRelease, error)
	ListReleases(ctx context.Context, tenantID, project string) ([]models.TranslationRelease, error)
	// CreateRelease commits the release and its bundles together, so a version is
	// never visible without the content it names.
	CreateRelease(ctx context.Context, r *models.TranslationRelease, bundles []models.TranslationBundle) error
	Bundle(ctx context.Context, tenantID, project, locale string, version int) (*models.TranslationBundle, error)
	ReleaseBundles(ctx context.Context, tenantID, project string, version int) ([]models.TranslationBundle, error)
	// ReleaseLocales names a version's locales without reading their strings —
	// the manifest reports versions, and every client polls it.
	ReleaseLocales(ctx context.Context, tenantID, project string, version int) ([]string, error)

	Scopes(ctx context.Context, tenantID, adminUserID string) ([]models.TranslatorScope, error)
	AllScopes(ctx context.Context, tenantID string) ([]models.TranslatorScope, error)
	// SetScopes replaces one translator's scope rows whole.
	SetScopes(ctx context.Context, tenantID, adminUserID string, scopes []models.TranslatorScope) error
}
