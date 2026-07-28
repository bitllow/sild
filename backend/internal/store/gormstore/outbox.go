package gormstore

import (
	"context"
	"time"

	"github.com/bitllow/sild/backend/internal/config"
	"github.com/bitllow/sild/backend/internal/id"
	"github.com/bitllow/sild/backend/internal/store"
	"github.com/bitllow/sild/backend/internal/store/models"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type outboxRepo struct{ db *gorm.DB }

func (r *outboxRepo) Enqueue(ctx context.Context, o *models.Outbox) error {
	if o.AvailableAt.IsZero() {
		o.AvailableAt = time.Now()
	}
	return r.db.WithContext(ctx).Create(o).Error
}

// unclaimed matches rows no live relay holds: never claimed, or claimed by one
// that died before its lock lapsed.
func unclaimed(db *gorm.DB, now time.Time) *gorm.DB {
	return db.Where("locked_until IS NULL OR locked_until < ?", now)
}

// skipLocked makes the candidate select pass over rows another relay is already
// claiming, so concurrent relays take disjoint pages instead of contending for
// the same one and coming back empty. Requires row locks: postgres, and mysql
// 8.0+. sqlite has none — its single writer serializes the claim anyway.
func skipLocked(db *gorm.DB) []clause.Expression {
	if Dialect(db) == config.SQLite {
		return nil
	}
	return []clause.Expression{clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}}
}

// ClaimDue returns only the rows this caller's lock won — a second relay's UPDATE
// matches nothing already locked, so no event is delivered twice. The token is
// what lets the winner read its own rows back. Select and claim share one
// transaction: the row locks the select takes are what other relays skip, and
// they only hold until it commits.
func (r *outboxRepo) ClaimDue(ctx context.Context, limit int) ([]models.Outbox, string, error) {
	token := id.New(id.Holder)
	var os []models.Outbox
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		now := time.Now()
		var ids []string
		err := unclaimed(tx.Model(&models.Outbox{}), now).
			Clauses(skipLocked(tx)...).
			Where("status = ? AND available_at <= ?", models.DeliveryPending, now).
			Order("available_at").Limit(limit).Pluck("id", &ids).Error
		if err != nil || len(ids) == 0 {
			return err
		}
		res := unclaimed(tx.Model(&models.Outbox{}), now).
			Where("id IN ? AND status = ?", ids, models.DeliveryPending).
			Updates(map[string]any{"claim_token": token, "locked_until": now.Add(store.OutboxClaimTTL)})
		if res.Error != nil {
			return res.Error
		}
		return tx.Where("claim_token = ?", token).Order("available_at").Find(&os).Error
	})
	if err != nil {
		return nil, "", err
	}
	return os, token, nil
}

// RenewClaim pushes this row's lock out while its relay is still working on the
// batch. False means another relay already took the row — the caller must stop
// delivering it rather than send an event twice.
func (r *outboxRepo) RenewClaim(ctx context.Context, id, claimToken string) (bool, error) {
	res := r.db.WithContext(ctx).Model(&models.Outbox{}).
		Where("id = ? AND claim_token = ?", id, claimToken).
		Update("locked_until", time.Now().Add(store.OutboxClaimTTL))
	if res.Error != nil {
		return false, res.Error
	}
	return res.RowsAffected > 0, nil
}

func (r *outboxRepo) MarkDelivered(ctx context.Context, id string) error {
	return r.db.WithContext(ctx).Model(&models.Outbox{}).Where("id = ?", id).
		Updates(map[string]any{"status": models.DeliveryDelivered, "claim_token": nil, "locked_until": nil}).Error
}

func (r *outboxRepo) Reschedule(ctx context.Context, id string, attempts, availableInSeconds int) error {
	return r.db.WithContext(ctx).Model(&models.Outbox{}).Where("id = ?", id).Updates(map[string]any{
		"attempts":     attempts,
		"available_at": time.Now().Add(time.Duration(availableInSeconds) * time.Second),
		"claim_token":  nil,
		"locked_until": nil,
	}).Error
}

func (r *outboxRepo) MarkFailed(ctx context.Context, id string) error {
	return r.db.WithContext(ctx).Model(&models.Outbox{}).Where("id = ?", id).
		Updates(map[string]any{"status": models.DeliveryFailed, "claim_token": nil, "locked_until": nil}).Error
}

var _ store.OutboxRepo = (*outboxRepo)(nil)
