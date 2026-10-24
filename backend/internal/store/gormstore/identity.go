package gormstore

import (
	"context"
	"time"

	"github.com/bitllow/sild/backend/internal/store"
	"github.com/bitllow/sild/backend/internal/store/models"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type identityRepo struct{ db *gorm.DB }

func (r *identityRepo) GetProvider(ctx context.Context, tenantID string) (*models.TenantIdentityProvider, error) {
	var p models.TenantIdentityProvider
	if err := r.db.WithContext(ctx).First(&p, "tenant_id = ?", tenantID).Error; err != nil {
		return nil, translateErr(err)
	}
	return &p, nil
}

func (r *identityRepo) ProviderByEmbedKey(ctx context.Context, embedKey string) (*models.TenantIdentityProvider, error) {
	var p models.TenantIdentityProvider
	if err := r.db.WithContext(ctx).First(&p, "embed_key = ?", embedKey).Error; err != nil {
		return nil, translateErr(err)
	}
	return &p, nil
}

func (r *identityRepo) UpsertProvider(ctx context.Context, p *models.TenantIdentityProvider) error {
	now := time.Now()
	if p.CreatedAt.IsZero() {
		p.CreatedAt = now
	}
	p.UpdatedAt = now
	return r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "tenant_id"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"embed_key", "validator_url", "validator_token_sealed", "validator_token_key_id",
			"allowed_origins", "embedded_only", "updated_at",
		}),
	}).Create(p).Error
}

func (r *identityRepo) DeleteProvider(ctx context.Context, tenantID string) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("tenant_id = ?", tenantID).Delete(&models.EmbedSession{}).Error; err != nil {
			return err
		}
		res := tx.Where("tenant_id = ?", tenantID).Delete(&models.TenantIdentityProvider{})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return store.ErrNotFound
		}
		return nil
	})
}

func (r *identityRepo) AdminBySub(ctx context.Context, tenantID, sub string) (*models.AdminUser, error) {
	var a models.AdminUser
	if err := r.db.WithContext(ctx).First(&a, "tenant_id = ? AND external_sub = ?", tenantID, sub).Error; err != nil {
		return nil, translateErr(err)
	}
	return &a, nil
}

func (r *identityRepo) SetAdminSub(ctx context.Context, tenantID, adminID string, sub *string) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		res := tx.Model(&models.AdminUser{}).
			Where("tenant_id = ? AND id = ?", tenantID, adminID).
			Update("external_sub", sub)
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return store.ErrNotFound
		}
		return tx.Where("tenant_id = ? AND admin_user_id = ?", tenantID, adminID).
			Delete(&models.EmbedSession{}).Error
	})
}

func (r *identityRepo) CreateSession(ctx context.Context, s *models.EmbedSession) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("tenant_id = ? AND admin_user_id = ? AND expires_at < ?", s.TenantID, s.AdminUserID, time.Now()).
			Delete(&models.EmbedSession{}).Error; err != nil {
			return err
		}
		return tx.Create(s).Error
	})
}

func (r *identityRepo) GetSession(ctx context.Context, id string) (*models.EmbedSession, error) {
	var s models.EmbedSession
	if err := r.db.WithContext(ctx).First(&s, "id = ?", id).Error; err != nil {
		return nil, translateErr(err)
	}
	return &s, nil
}

func (r *identityRepo) DeleteSession(ctx context.Context, id string) error {
	return r.db.WithContext(ctx).Delete(&models.EmbedSession{}, "id = ?", id).Error
}

var _ store.IdentityRepo = (*identityRepo)(nil)
