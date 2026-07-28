package gormstore

import (
	"context"
	"time"

	"github.com/bitllow/sild/backend/internal/store"
	"github.com/bitllow/sild/backend/internal/store/models"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type leaseRepo struct{ db *gorm.DB }

// Acquire takes the lease if it is free or expired, or renews it for the same
// owner. Both statements are single conditional writes, so two replicas racing
// resolve in the database: exactly one reports true.
func (r *leaseRepo) Acquire(ctx context.Context, name, owner string, ttl time.Duration) (bool, error) {
	now := time.Now()
	l := &models.JobLease{Name: name, Owner: owner, ExpiresAt: now.Add(ttl)}
	res := r.db.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(l)
	if res.Error != nil {
		return false, res.Error
	}
	if res.RowsAffected > 0 {
		return true, nil
	}
	res = r.db.WithContext(ctx).Model(&models.JobLease{}).
		Where("name = ? AND (expires_at < ? OR owner = ?)", name, now, owner).
		Updates(map[string]any{"owner": owner, "expires_at": now.Add(ttl)})
	if res.Error != nil {
		return false, res.Error
	}
	return res.RowsAffected > 0, nil
}

// Release drops the lease, but only if this owner still holds it — a holder that
// overran its TTL must not release the lease someone else has since taken.
func (r *leaseRepo) Release(ctx context.Context, name, owner string) error {
	return r.db.WithContext(ctx).
		Where("name = ? AND owner = ?", name, owner).
		Delete(&models.JobLease{}).Error
}

// Held reports whether a live (unexpired) lease exists under this name.
func (r *leaseRepo) Held(ctx context.Context, name string) (bool, error) {
	var n int64
	err := r.db.WithContext(ctx).Model(&models.JobLease{}).
		Where("name = ? AND expires_at > ?", name, time.Now()).Count(&n).Error
	return n > 0, err
}

var _ store.LeaseRepo = (*leaseRepo)(nil)
