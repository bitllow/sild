package gormstore

import (
	"context"

	"github.com/bitllow/sild/backend/internal/store"
	"github.com/bitllow/sild/backend/internal/store/models"
	"gorm.io/gorm"
)

type webhookRepo struct{ db *gorm.DB }

func (r *webhookRepo) Create(ctx context.Context, e *models.WebhookEndpoint) error {
	return r.db.WithContext(ctx).Create(e).Error
}

func (r *webhookRepo) List(ctx context.Context, tenantID string) ([]models.WebhookEndpoint, error) {
	var es []models.WebhookEndpoint
	err := r.db.WithContext(ctx).Preload("Events").
		Where("tenant_id = ?", tenantID).Order("created_at desc").Find(&es).Error
	return es, err
}

func (r *webhookRepo) SetActive(ctx context.Context, tenantID, id string, active bool) error {
	res := r.db.WithContext(ctx).Model(&models.WebhookEndpoint{}).
		Where("tenant_id = ? AND id = ?", tenantID, id).Update("active", active)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return store.ErrNotFound
	}
	return nil
}

func (r *webhookRepo) Delete(ctx context.Context, tenantID, id string) error {
	res := r.db.WithContext(ctx).Where("tenant_id = ? AND id = ?", tenantID, id).Delete(&models.WebhookEndpoint{})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return store.ErrNotFound
	}
	return nil
}

func (r *webhookRepo) ListForEvent(ctx context.Context, tenantID, event string) ([]models.WebhookEndpoint, error) {
	var es []models.WebhookEndpoint
	err := r.db.WithContext(ctx).Preload("Events").
		Joins("JOIN webhook_events ev ON ev.endpoint_id = webhook_endpoints.id").
		Where("webhook_endpoints.tenant_id = ? AND webhook_endpoints.active = ? AND ev.event = ?",
			tenantID, true, event).
		Find(&es).Error
	return es, err
}

func (r *webhookRepo) LogDelivery(ctx context.Context, d *models.WebhookDelivery) error {
	return r.db.WithContext(ctx).Create(d).Error
}

func (r *webhookRepo) ListDeliveries(ctx context.Context, tenantID, endpointID string) ([]models.WebhookDelivery, error) {
	var ds []models.WebhookDelivery
	err := r.db.WithContext(ctx).
		Where("tenant_id = ? AND endpoint_id = ?", tenantID, endpointID).
		Order("created_at desc").Limit(200).Find(&ds).Error
	return ds, err
}

var _ store.WebhookRepo = (*webhookRepo)(nil)
