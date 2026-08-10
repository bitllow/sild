package gormstore

import (
	"context"
	"time"

	"github.com/bitllow/sild/backend/internal/config"
	"github.com/bitllow/sild/backend/internal/store"
	"github.com/bitllow/sild/backend/internal/store/models"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type roleAssignmentRepo struct{ db *gorm.DB }

func (r *roleAssignmentRepo) ListByAdmin(ctx context.Context, tenantID, adminUserID string) ([]models.RoleAssignment, error) {
	var as []models.RoleAssignment
	err := r.db.WithContext(ctx).
		Where("tenant_id = ? AND admin_user_id = ?", tenantID, adminUserID).
		Order("role").Find(&as).Error
	return as, err
}

func (r *roleAssignmentRepo) ListByTenant(ctx context.Context, tenantID string) ([]models.RoleAssignment, error) {
	var as []models.RoleAssignment
	err := r.db.WithContext(ctx).Where("tenant_id = ?", tenantID).
		Order("admin_user_id, role").Find(&as).Error
	return as, err
}

// Create adds an assignment. The unique index is what refuses a second one, so
// two concurrent grants cannot both believe they were first.
func (r *roleAssignmentRepo) Create(ctx context.Context, a *models.RoleAssignment) error {
	res := r.db.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(a)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return store.ErrDuplicate
	}
	return nil
}

// Rescope replaces the scope of the assignment the member holds. Read-then-save
// rather than an UPDATE map: the scope column is serialized by the model, and a
// raw map would write the struct's Go rendering.
func (r *roleAssignmentRepo) Rescope(ctx context.Context, tenantID, adminUserID string, role models.PlatformRole, scope models.RoleScope) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var row models.RoleAssignment
		err := tx.Where("tenant_id = ? AND admin_user_id = ? AND role = ?", tenantID, adminUserID, role).
			First(&row).Error
		if err != nil {
			return translateErr(err)
		}
		row.Scope = scope
		row.UpdatedAt = time.Now()
		return tx.Save(&row).Error
	})
}

// Delete removes one assignment. The last-owner invariant is checked against the
// rows the delete leaves behind, inside its transaction and under a lock, so two
// concurrent removals cannot both see the other's owner and orphan the tenant.
func (r *roleAssignmentRepo) Delete(ctx context.Context, tenantID, adminUserID string, role models.PlatformRole) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if role == models.PlatformOwner {
			var owners []string
			q := tx.Model(&models.RoleAssignment{}).
				Where("tenant_id = ? AND role = ?", tenantID, models.PlatformOwner)
			if Dialect(tx) != config.SQLite {
				q = q.Clauses(clause.Locking{Strength: "UPDATE"})
			}
			if err := q.Pluck("admin_user_id", &owners).Error; err != nil {
				return err
			}
			if !anyOther(owners, adminUserID) {
				return store.ErrLastOwner
			}
		}
		res := tx.Where("tenant_id = ? AND admin_user_id = ? AND role = ?", tenantID, adminUserID, role).
			Delete(&models.RoleAssignment{})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return store.ErrNotFound
		}
		return nil
	})
}

func anyOther(ids []string, except string) bool {
	for _, id := range ids {
		if id != except {
			return true
		}
	}
	return false
}

var _ store.RoleAssignmentRepo = (*roleAssignmentRepo)(nil)
