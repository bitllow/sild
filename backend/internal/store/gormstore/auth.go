package gormstore

import (
	"context"
	"time"

	"github.com/bitllow/sild/backend/internal/store"
	"github.com/bitllow/sild/backend/internal/store/models"
	"gorm.io/gorm"
)

type apiKeyRepo struct{ db *gorm.DB }

func (r *apiKeyRepo) Create(ctx context.Context, k *models.APIKey) error {
	return r.db.WithContext(ctx).Create(k).Error
}

func (r *apiKeyRepo) FindByPrefix(ctx context.Context, prefix string) (*models.APIKey, error) {
	var k models.APIKey
	if err := r.db.WithContext(ctx).First(&k, "prefix = ?", prefix).Error; err != nil {
		return nil, translateErr(err)
	}
	return &k, nil
}

func (r *apiKeyRepo) ListByTenant(ctx context.Context, tenantID string) ([]models.APIKey, error) {
	var ks []models.APIKey
	err := r.db.WithContext(ctx).Where("tenant_id = ?", tenantID).Order("created_at desc").Find(&ks).Error
	return ks, err
}

func (r *apiKeyRepo) Revoke(ctx context.Context, tenantID, id string) error {
	now := time.Now()
	res := r.db.WithContext(ctx).Model(&models.APIKey{}).
		Where("tenant_id = ? AND id = ? AND revoked_at IS NULL", tenantID, id).
		Update("revoked_at", now)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return store.ErrNotFound
	}
	return nil
}

type adminRepo struct{ db *gorm.DB }

func (r *adminRepo) Create(ctx context.Context, a *models.AdminUser) error {
	return r.db.WithContext(ctx).Create(a).Error
}

func (r *adminRepo) Get(ctx context.Context, tenantID, id string) (*models.AdminUser, error) {
	var a models.AdminUser
	if err := r.db.WithContext(ctx).First(&a, "tenant_id = ? AND id = ?", tenantID, id).Error; err != nil {
		return nil, translateErr(err)
	}
	return &a, nil
}

func (r *adminRepo) FindByEmail(ctx context.Context, email string) ([]models.AdminUser, error) {
	var as []models.AdminUser
	err := r.db.WithContext(ctx).Where("email = ?", email).Find(&as).Error
	return as, err
}

func (r *adminRepo) List(ctx context.Context, tenantID string) ([]models.AdminUser, error) {
	var as []models.AdminUser
	err := r.db.WithContext(ctx).Where("tenant_id = ?", tenantID).Order("created_at").Find(&as).Error
	return as, err
}

func (r *adminRepo) SetPassword(ctx context.Context, tenantID, id, passwordHash string) error {
	res := r.db.WithContext(ctx).Model(&models.AdminUser{}).
		Where("tenant_id = ? AND id = ?", tenantID, id).
		Update("password_hash", passwordHash)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return store.ErrNotFound
	}
	return nil
}

func (r *adminRepo) SetRole(ctx context.Context, tenantID, id string, role models.PlatformRole) error {
	res := r.db.WithContext(ctx).Model(&models.AdminUser{}).
		Where("tenant_id = ? AND id = ?", tenantID, id).
		Update("platform_role", role)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return store.ErrNotFound
	}
	return nil
}

func (r *adminRepo) CreateSession(ctx context.Context, s *models.AdminSession) error {
	return r.db.WithContext(ctx).Create(s).Error
}

func (r *adminRepo) GetSession(ctx context.Context, id string) (*models.AdminSession, error) {
	var s models.AdminSession
	if err := r.db.WithContext(ctx).First(&s, "id = ?", id).Error; err != nil {
		return nil, translateErr(err)
	}
	return &s, nil
}

func (r *adminRepo) DeleteSession(ctx context.Context, id string) error {
	return r.db.WithContext(ctx).Delete(&models.AdminSession{}, "id = ?", id).Error
}

type signingKeyRepo struct{ db *gorm.DB }

func (r *signingKeyRepo) Create(ctx context.Context, k *models.SigningKey) error {
	return r.db.WithContext(ctx).Create(k).Error
}

func (r *signingKeyRepo) Active(ctx context.Context) (*models.SigningKey, error) {
	var k models.SigningKey
	if err := r.db.WithContext(ctx).Where("active = ?", true).
		Order("created_at desc").First(&k).Error; err != nil {
		return nil, translateErr(err)
	}
	return &k, nil
}

func (r *signingKeyRepo) GetByKid(ctx context.Context, kid string) (*models.SigningKey, error) {
	var k models.SigningKey
	if err := r.db.WithContext(ctx).First(&k, "kid = ?", kid).Error; err != nil {
		return nil, translateErr(err)
	}
	return &k, nil
}

func (r *signingKeyRepo) Published(ctx context.Context) ([]models.SigningKey, error) {
	var ks []models.SigningKey
	// active keys + recently retired (still verifiable) — retired_at null or future-ish.
	err := r.db.WithContext(ctx).Where("retired_at IS NULL").Order("created_at desc").Find(&ks).Error
	return ks, err
}

var (
	_ store.APIKeyRepo     = (*apiKeyRepo)(nil)
	_ store.AdminRepo      = (*adminRepo)(nil)
	_ store.SigningKeyRepo = (*signingKeyRepo)(nil)
)
