package gormstore

import (
	"context"
	"time"

	"github.com/bitllow/sild/backend/internal/store"
	"github.com/bitllow/sild/backend/internal/store/models"
	"gorm.io/gorm"
)

type outboxRepo struct{ db *gorm.DB }

func (r *outboxRepo) Enqueue(ctx context.Context, o *models.Outbox) error {
	if o.AvailableAt.IsZero() {
		o.AvailableAt = time.Now()
	}
	return r.db.WithContext(ctx).Create(o).Error
}

func (r *outboxRepo) ClaimDue(ctx context.Context, limit int) ([]models.Outbox, string, error) {
	return claimDue[models.Outbox](ctx, r.db, limit)
}

func (r *outboxRepo) RenewClaim(ctx context.Context, id, claimToken string) (bool, error) {
	return renewClaim[models.Outbox](ctx, r.db, id, claimToken)
}

func (r *outboxRepo) MarkDelivered(ctx context.Context, id string) error {
	return settle[models.Outbox](ctx, r.db, id, models.DeliveryDelivered)
}

func (r *outboxRepo) MarkFailed(ctx context.Context, id string) error {
	return settle[models.Outbox](ctx, r.db, id, models.DeliveryFailed)
}

func (r *outboxRepo) Reschedule(ctx context.Context, id string, attempts, availableInSeconds int) error {
	return reschedule[models.Outbox](ctx, r.db, id, attempts, availableInSeconds)
}

var _ store.OutboxRepo = (*outboxRepo)(nil)
