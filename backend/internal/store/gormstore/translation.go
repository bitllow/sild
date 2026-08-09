package gormstore

import (
	"context"

	"github.com/bitllow/sild/backend/internal/store"
	"github.com/bitllow/sild/backend/internal/store/models"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type translationRepo struct{ db *gorm.DB }

func (r *translationRepo) GetProject(ctx context.Context, tenantID, project string) (*models.TranslationProject, error) {
	var p models.TranslationProject
	err := r.db.WithContext(ctx).
		Where("tenant_id = ? AND slug = ?", tenantID, project).
		First(&p).Error
	if err != nil {
		return nil, translateErr(err)
	}
	return &p, nil
}

func (r *translationRepo) SaveProject(ctx context.Context, p *models.TranslationProject, locales []string) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.OnConflict{UpdateAll: true}).Create(p).Error; err != nil {
			return err
		}
		if err := tx.Where("tenant_id = ? AND project = ?", p.TenantID, p.Slug).
			Delete(&models.TranslationProjectLocale{}).Error; err != nil {
			return err
		}
		if len(locales) == 0 {
			return nil
		}
		rows := make([]models.TranslationProjectLocale, 0, len(locales))
		for _, l := range locales {
			rows = append(rows, models.TranslationProjectLocale{TenantID: p.TenantID, Project: p.Slug, Locale: l})
		}
		return tx.Create(&rows).Error
	})
}

func (r *translationRepo) ProjectLocales(ctx context.Context, tenantID, project string) ([]string, error) {
	var rows []models.TranslationProjectLocale
	err := r.db.WithContext(ctx).
		Where("tenant_id = ? AND project = ?", tenantID, project).
		Order("locale asc").Find(&rows).Error
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(rows))
	for _, l := range rows {
		out = append(out, l.Locale)
	}
	return out, nil
}

func (r *translationRepo) Overrides(ctx context.Context, tenantID, project string) ([]models.TranslationOverride, error) {
	var os []models.TranslationOverride
	err := r.db.WithContext(ctx).
		Where("tenant_id = ? AND project = ?", tenantID, project).
		Find(&os).Error
	return os, err
}

func (r *translationRepo) LocaleOverrides(ctx context.Context, tenantID, project, locale string) ([]models.TranslationOverride, error) {
	var os []models.TranslationOverride
	err := r.db.WithContext(ctx).
		Where("tenant_id = ? AND project = ? AND locale = ?", tenantID, project, locale).
		Find(&os).Error
	return os, err
}

func (r *translationRepo) PutOverride(ctx context.Context, o *models.TranslationOverride) error {
	return r.db.WithContext(ctx).
		Clauses(clause.OnConflict{UpdateAll: true}).
		Create(o).Error
}

func (r *translationRepo) DeleteOverride(ctx context.Context, tenantID, project, locale, key string) error {
	return r.db.WithContext(ctx).
		Where("tenant_id = ? AND project = ? AND locale = ? AND string_key = ?", tenantID, project, locale, key).
		Delete(&models.TranslationOverride{}).Error
}

func (r *translationRepo) LatestRelease(ctx context.Context, tenantID, project string) (*models.TranslationRelease, error) {
	var rel models.TranslationRelease
	err := r.db.WithContext(ctx).
		Where("tenant_id = ? AND project = ?", tenantID, project).
		Order("version desc").First(&rel).Error
	if err != nil {
		return nil, translateErr(err)
	}
	return &rel, nil
}

func (r *translationRepo) ListReleases(ctx context.Context, tenantID, project string) ([]models.TranslationRelease, error) {
	var rs []models.TranslationRelease
	err := r.db.WithContext(ctx).
		Where("tenant_id = ? AND project = ?", tenantID, project).
		Order("version desc").Find(&rs).Error
	return rs, err
}

func (r *translationRepo) CreateRelease(ctx context.Context, rel *models.TranslationRelease, bundles []models.TranslationBundle) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(rel).Error; err != nil {
			return err
		}
		if len(bundles) == 0 {
			return nil
		}
		return tx.Create(&bundles).Error
	})
}

func (r *translationRepo) Bundle(ctx context.Context, tenantID, project, locale string, version int) (*models.TranslationBundle, error) {
	var b models.TranslationBundle
	err := r.db.WithContext(ctx).
		Where("tenant_id = ? AND project = ? AND locale = ? AND version = ?", tenantID, project, locale, version).
		First(&b).Error
	if err != nil {
		return nil, translateErr(err)
	}
	return &b, nil
}

func (r *translationRepo) ReleaseBundles(ctx context.Context, tenantID, project string, version int) ([]models.TranslationBundle, error) {
	var bs []models.TranslationBundle
	err := r.db.WithContext(ctx).
		Where("tenant_id = ? AND project = ? AND version = ?", tenantID, project, version).
		Order("locale asc").Find(&bs).Error
	return bs, err
}

func (r *translationRepo) ReleaseLocales(ctx context.Context, tenantID, project string, version int) ([]string, error) {
	var locales []string
	err := r.db.WithContext(ctx).Model(&models.TranslationBundle{}).
		Where("tenant_id = ? AND project = ? AND version = ?", tenantID, project, version).
		Order("locale asc").Pluck("locale", &locales).Error
	return locales, err
}

func (r *translationRepo) Scopes(ctx context.Context, tenantID, adminUserID string) ([]models.TranslatorScope, error) {
	var ss []models.TranslatorScope
	err := r.db.WithContext(ctx).
		Where("tenant_id = ? AND admin_user_id = ?", tenantID, adminUserID).
		Find(&ss).Error
	return ss, err
}

func (r *translationRepo) AllScopes(ctx context.Context, tenantID string) ([]models.TranslatorScope, error) {
	var ss []models.TranslatorScope
	err := r.db.WithContext(ctx).Where("tenant_id = ?", tenantID).Find(&ss).Error
	return ss, err
}

func (r *translationRepo) SetScopes(ctx context.Context, tenantID, adminUserID string, scopes []models.TranslatorScope) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("tenant_id = ? AND admin_user_id = ?", tenantID, adminUserID).
			Delete(&models.TranslatorScope{}).Error; err != nil {
			return err
		}
		if len(scopes) == 0 {
			return nil
		}
		return tx.Create(&scopes).Error
	})
}

var _ store.TranslationRepo = (*translationRepo)(nil)
