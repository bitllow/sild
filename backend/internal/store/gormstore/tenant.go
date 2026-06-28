package gormstore

import (
	"context"

	"github.com/bitllow/sild/backend/internal/store"
	"github.com/bitllow/sild/backend/internal/store/models"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type tenantRepo struct{ db *gorm.DB }

func (r *tenantRepo) Create(ctx context.Context, t *models.Tenant) error {
	return r.db.WithContext(ctx).Create(t).Error
}

func (r *tenantRepo) Get(ctx context.Context, id string) (*models.Tenant, error) {
	var t models.Tenant
	if err := r.db.WithContext(ctx).First(&t, "id = ?", id).Error; err != nil {
		return nil, translateErr(err)
	}
	return &t, nil
}

func (r *tenantRepo) AllIDs(ctx context.Context) ([]string, error) {
	var ids []string
	err := r.db.WithContext(ctx).Model(&models.Tenant{}).Pluck("id", &ids).Error
	return ids, err
}

func (r *tenantRepo) SearchableKeys(ctx context.Context, tenantID string) ([]string, error) {
	var keys []string
	err := r.db.WithContext(ctx).Model(&models.TenantSearchableKey{}).
		Where("tenant_id = ?", tenantID).Pluck("key", &keys).Error
	return keys, err
}

func (r *tenantRepo) SetSearchableKeys(ctx context.Context, tenantID string, keys []string) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("tenant_id = ?", tenantID).Delete(&models.TenantSearchableKey{}).Error; err != nil {
			return err
		}
		if len(keys) == 0 {
			return nil
		}
		rows := make([]models.TenantSearchableKey, 0, len(keys))
		for _, k := range keys {
			rows = append(rows, models.TenantSearchableKey{TenantID: tenantID, Key: k})
		}
		return tx.Create(&rows).Error
	})
}

func (r *tenantRepo) GetEmailConfig(ctx context.Context, tenantID string) (*models.TenantEmailConfig, error) {
	var c models.TenantEmailConfig
	if err := r.db.WithContext(ctx).Preload("AllowedDomains").
		First(&c, "tenant_id = ?", tenantID).Error; err != nil {
		return nil, translateErr(err)
	}
	return &c, nil
}

func (r *tenantRepo) SetEmailConfig(ctx context.Context, cfg *models.TenantEmailConfig) error {
	return r.db.WithContext(ctx).Clauses(clause.OnConflict{UpdateAll: true}).Create(cfg).Error
}

func (r *tenantRepo) FindByInboundDomain(ctx context.Context, domain string) (*models.TenantEmailConfig, error) {
	var c models.TenantEmailConfig
	err := r.db.WithContext(ctx).Preload("AllowedDomains").
		First(&c, "inbound_domain = ?", domain).Error
	if err != nil {
		return nil, translateErr(err)
	}
	return &c, nil
}

var _ store.TenantRepo = (*tenantRepo)(nil)
