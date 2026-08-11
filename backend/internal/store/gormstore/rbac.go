package gormstore

import (
	"context"
	"slices"
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

// Create adds an assignment, leaving the unique index to refuse a second one.
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

// Rescope replaces the scope of the assignment the member holds. One conditional
// update, never a read-then-save: gorm's Save inserts when its update matches no
// row, which would resurrect an assignment another admin removed mid-edit.
func (r *roleAssignmentRepo) Rescope(ctx context.Context, tenantID, adminUserID string, role models.PlatformRole, scope models.RoleScope) error {
	res := r.db.WithContext(ctx).Model(&models.RoleAssignment{}).
		Where("tenant_id = ? AND admin_user_id = ? AND role = ?", tenantID, adminUserID, role).
		Select("scope", "updated_at").
		Updates(&models.RoleAssignment{Scope: scope, UpdatedAt: time.Now()})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return store.ErrNotFound
	}
	return nil
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
			if !slices.ContainsFunc(owners, func(id string) bool { return id != adminUserID }) {
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

var _ store.RoleAssignmentRepo = (*roleAssignmentRepo)(nil)
