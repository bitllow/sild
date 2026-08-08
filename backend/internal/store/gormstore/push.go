package gormstore

import (
	"context"
	"errors"
	"time"

	"github.com/bitllow/sild/backend/internal/store"
	"github.com/bitllow/sild/backend/internal/store/models"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type pushConfigRepo struct{ db *gorm.DB }

func (r *pushConfigRepo) Get(ctx context.Context, tenantID string) (*models.TenantPushConfig, error) {
	var c models.TenantPushConfig
	err := r.db.WithContext(ctx).Where("tenant_id = ?", tenantID).First(&c).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, store.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &c, nil
}

// Upsert replaces the whole credential. Verified is reset by the caller when the
// credential changes — a new project has not proved anything yet.
func (r *pushConfigRepo) Upsert(ctx context.Context, c *models.TenantPushConfig) error {
	c.UpdatedAt = time.Now()
	return r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "tenant_id"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"project_id", "client_email", "credential_sealed", "credential_key_id", "verified", "updated_at",
		}),
	}).Create(c).Error
}

func (r *pushConfigRepo) Delete(ctx context.Context, tenantID string) error {
	return r.db.WithContext(ctx).Where("tenant_id = ?", tenantID).
		Delete(&models.TenantPushConfig{}).Error
}

func (r *pushConfigRepo) MarkVerified(ctx context.Context, tenantID string) error {
	return r.db.WithContext(ctx).Model(&models.TenantPushConfig{}).
		Where("tenant_id = ? AND verified = ?", tenantID, false).
		Updates(map[string]any{"verified": true, "updated_at": time.Now()}).Error
}

type pushOptOutRepo struct{ db *gorm.DB }

func (r *pushOptOutRepo) Set(ctx context.Context, tenantID, externalUserID string) error {
	return r.db.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).
		Create(&models.PushOptOut{TenantID: tenantID, ExternalUserID: externalUserID, CreatedAt: time.Now()}).Error
}

func (r *pushOptOutRepo) Clear(ctx context.Context, tenantID, externalUserID string) error {
	return r.db.WithContext(ctx).
		Where("tenant_id = ? AND external_user_id = ?", tenantID, externalUserID).
		Delete(&models.PushOptOut{}).Error
}

func (r *pushOptOutRepo) OptedOut(ctx context.Context, tenantID string, externalUserIDs []string) (map[string]bool, error) {
	out := map[string]bool{}
	if len(externalUserIDs) == 0 {
		return out, nil
	}
	var ids []string
	err := r.db.WithContext(ctx).Model(&models.PushOptOut{}).
		Where("tenant_id = ? AND external_user_id IN ?", tenantID, externalUserIDs).
		Pluck("external_user_id", &ids).Error
	if err != nil {
		return nil, err
	}
	for _, id := range ids {
		out[id] = true
	}
	return out, nil
}

type pushOutboxRepo struct{ db *gorm.DB }

// Enqueue is idempotent on message_id: a send retried into the same message must
// not queue a second nudge.
func (r *pushOutboxRepo) Enqueue(ctx context.Context, p *models.PushOutbox) error {
	if p.AvailableAt.IsZero() {
		p.AvailableAt = time.Now()
	}
	return r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "message_id"}}, DoNothing: true,
	}).Create(p).Error
}

func (r *pushOutboxRepo) ClaimDue(ctx context.Context, limit int) ([]models.PushOutbox, string, error) {
	return claimDue[models.PushOutbox](ctx, r.db, limit)
}

func (r *pushOutboxRepo) RenewClaim(ctx context.Context, id, claimToken string) (bool, error) {
	return renewClaim[models.PushOutbox](ctx, r.db, id, claimToken)
}

func (r *pushOutboxRepo) MarkDelivered(ctx context.Context, id string) error {
	return settle[models.PushOutbox](ctx, r.db, id, models.DeliveryDelivered)
}

func (r *pushOutboxRepo) MarkFailed(ctx context.Context, id string) error {
	return settle[models.PushOutbox](ctx, r.db, id, models.DeliveryFailed)
}

func (r *pushOutboxRepo) Reschedule(ctx context.Context, id string, attempts, availableInSeconds int) error {
	return reschedule[models.PushOutbox](ctx, r.db, id, attempts, availableInSeconds)
}

var (
	_ store.PushConfigRepo = (*pushConfigRepo)(nil)
	_ store.PushOptOutRepo = (*pushOptOutRepo)(nil)
	_ store.PushOutboxRepo = (*pushOutboxRepo)(nil)
)
