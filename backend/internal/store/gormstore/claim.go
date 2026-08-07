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

// The claim protocol shared by every delivery queue: take due rows under a lock
// no other process can steal, work them, then mark or reschedule. The webhook
// outbox and the push queue are separate tables (see models.PushOutbox) but the
// contention problem is identical, so the solution lives here once.
//
// Any row type using this needs the same columns: status, available_at,
// attempts, claim_token, locked_until.

// unclaimed matches rows no live worker holds: never claimed, or claimed by one
// that died before its lock lapsed.
func unclaimed(db *gorm.DB, now time.Time) *gorm.DB {
	return db.Where("locked_until IS NULL OR locked_until < ?", now)
}

// skipLocked makes the candidate select pass over rows another worker is already
// claiming, so concurrent workers take disjoint pages instead of contending for
// the same one and coming back empty. Requires row locks: postgres, and mysql
// 8.0+. sqlite has none — its single writer serializes the claim anyway.
func skipLocked(db *gorm.DB) []clause.Expression {
	if Dialect(db) == config.SQLite {
		return nil
	}
	return []clause.Expression{clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}}
}

// claimDue returns only the rows this caller's lock won — a second worker's
// UPDATE matches nothing already locked, so no row is delivered twice. The token
// is what lets the winner read its own rows back. Select and claim share one
// transaction: the row locks the select takes are what other workers skip, and
// they only hold until it commits.
func claimDue[T any](ctx context.Context, db *gorm.DB, limit int) ([]T, string, error) {
	token := id.New(id.Holder)
	var rows []T
	var model T
	err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		now := time.Now()
		var ids []string
		err := unclaimed(tx.Model(&model), now).
			Clauses(skipLocked(tx)...).
			Where("status = ? AND available_at <= ?", models.DeliveryPending, now).
			Order("available_at").Limit(limit).Pluck("id", &ids).Error
		if err != nil || len(ids) == 0 {
			return err
		}
		res := unclaimed(tx.Model(&model), now).
			Where("id IN ? AND status = ?", ids, models.DeliveryPending).
			Updates(map[string]any{"claim_token": token, "locked_until": now.Add(store.ClaimTTL)})
		if res.Error != nil {
			return res.Error
		}
		return tx.Where("claim_token = ?", token).Order("available_at").Find(&rows).Error
	})
	if err != nil {
		return nil, "", err
	}
	return rows, token, nil
}

// renewClaim pushes a row's lock out while its worker is still on the batch.
// False means another worker took it — the caller must stop rather than deliver
// twice.
func renewClaim[T any](ctx context.Context, db *gorm.DB, rowID, claimToken string) (bool, error) {
	var model T
	res := db.WithContext(ctx).Model(&model).
		Where("id = ? AND claim_token = ?", rowID, claimToken).
		Update("locked_until", time.Now().Add(store.ClaimTTL))
	if res.Error != nil {
		return false, res.Error
	}
	return res.RowsAffected > 0, nil
}

// settle marks a row terminal and drops its claim.
func settle[T any](ctx context.Context, db *gorm.DB, rowID string, status models.DeliveryStatus) error {
	var model T
	return db.WithContext(ctx).Model(&model).Where("id = ?", rowID).
		Updates(map[string]any{"status": status, "claim_token": nil, "locked_until": nil}).Error
}

// reschedule returns a row to the queue after a failed attempt.
func reschedule[T any](ctx context.Context, db *gorm.DB, rowID string, attempts, availableInSeconds int) error {
	var model T
	return db.WithContext(ctx).Model(&model).Where("id = ?", rowID).Updates(map[string]any{
		"attempts":     attempts,
		"available_at": time.Now().Add(time.Duration(availableInSeconds) * time.Second),
		"claim_token":  nil,
		"locked_until": nil,
	}).Error
}
