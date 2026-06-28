package gormstore

import (
	"context"
	"time"

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

type outboxRepo struct{ db *gorm.DB }

func (r *outboxRepo) Enqueue(ctx context.Context, o *models.Outbox) error {
	if o.AvailableAt.IsZero() {
		o.AvailableAt = time.Now()
	}
	return r.db.WithContext(ctx).Create(o).Error
}

// ClaimDue returns pending events whose backoff has elapsed.
func (r *outboxRepo) ClaimDue(ctx context.Context, limit int) ([]models.Outbox, error) {
	var os []models.Outbox
	err := r.db.WithContext(ctx).
		Where("status = ? AND available_at <= ?", models.DeliveryPending, time.Now()).
		Order("available_at").Limit(limit).Find(&os).Error
	return os, err
}

func (r *outboxRepo) MarkDelivered(ctx context.Context, id string) error {
	return r.db.WithContext(ctx).Model(&models.Outbox{}).Where("id = ?", id).
		Update("status", models.DeliveryDelivered).Error
}

func (r *outboxRepo) Reschedule(ctx context.Context, id string, attempts, availableInSeconds int) error {
	return r.db.WithContext(ctx).Model(&models.Outbox{}).Where("id = ?", id).Updates(map[string]any{
		"attempts":     attempts,
		"available_at": time.Now().Add(time.Duration(availableInSeconds) * time.Second),
	}).Error
}

func (r *outboxRepo) MarkFailed(ctx context.Context, id string) error {
	return r.db.WithContext(ctx).Model(&models.Outbox{}).Where("id = ?", id).
		Update("status", models.DeliveryFailed).Error
}

type emailRepo struct{ db *gorm.DB }

func (r *emailRepo) CreateThread(ctx context.Context, t *models.EmailThread) error {
	return r.db.WithContext(ctx).Create(t).Error
}

func (r *emailRepo) FindByToken(ctx context.Context, token string) (*models.EmailThread, error) {
	var t models.EmailThread
	if err := r.db.WithContext(ctx).First(&t, "thread_token = ?", token).Error; err != nil {
		return nil, translateErr(err)
	}
	return &t, nil
}

func (r *emailRepo) Get(ctx context.Context, tenantID, convID string) (*models.EmailThread, error) {
	var t models.EmailThread
	if err := r.db.WithContext(ctx).First(&t, "tenant_id = ? AND conversation_id = ?", tenantID, convID).Error; err != nil {
		return nil, translateErr(err)
	}
	return &t, nil
}

func (r *emailRepo) Update(ctx context.Context, t *models.EmailThread) error {
	return r.db.WithContext(ctx).Save(t).Error
}

var (
	_ store.WebhookRepo = (*webhookRepo)(nil)
	_ store.OutboxRepo  = (*outboxRepo)(nil)
	_ store.EmailRepo   = (*emailRepo)(nil)
)
