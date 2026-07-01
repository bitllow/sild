package gormstore

import (
	"context"

	"github.com/bitllow/sild/backend/internal/store"
	"github.com/bitllow/sild/backend/internal/store/models"
	"gorm.io/gorm"
)

type brandRepo struct{ db *gorm.DB }

func (r *brandRepo) List(ctx context.Context, tenantID string) ([]models.Brand, error) {
	var bs []models.Brand
	err := r.db.WithContext(ctx).
		Where("tenant_id = ?", tenantID).
		Order("position asc, created_at asc").
		Find(&bs).Error
	return bs, err
}

func (r *brandRepo) Active(ctx context.Context, tenantID string) (*models.Brand, error) {
	var b models.Brand
	err := r.db.WithContext(ctx).
		Where("tenant_id = ? AND active = ?", tenantID, true).
		Order("position asc, created_at asc").
		First(&b).Error
	if err != nil {
		return nil, translateErr(err)
	}
	return &b, nil
}

// Replace swaps the tenant's whole brand set in one transaction: the Appearance
// UI stages edits across every brand and saves them together, so a delete-all +
// insert keeps the persisted set an exact mirror of what the admin saved (removed
// brands disappear, positions and the single active flag stick).
func (r *brandRepo) Replace(ctx context.Context, tenantID string, brands []models.Brand) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("tenant_id = ?", tenantID).Delete(&models.Brand{}).Error; err != nil {
			return err
		}
		if len(brands) == 0 {
			return nil
		}
		for i := range brands {
			brands[i].TenantID = tenantID
		}
		// Select("*") forces every column (incl. the false `active` values, which
		// carry a DB default GORM would otherwise omit) into the INSERT, so the
		// domain layer's "exactly one active" invariant persists verbatim.
		return tx.Select("*").Create(&brands).Error
	})
}

var _ store.BrandRepo = (*brandRepo)(nil)
