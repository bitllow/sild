package gormstore

import (
	"context"

	"github.com/bitllow/sild/backend/internal/store/models"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type queueRepo struct{ db *gorm.DB }

func (r *queueRepo) Get(ctx context.Context, tenantID, key string) (*models.Queue, error) {
	var q models.Queue
	err := r.db.WithContext(ctx).Where("tenant_id = ? AND queue_key = ?", tenantID, key).First(&q).Error
	if err != nil {
		return nil, translateErr(err)
	}
	return &q, nil
}

func (r *queueRepo) List(ctx context.Context, tenantID string) ([]models.Queue, error) {
	var qs []models.Queue
	err := r.db.WithContext(ctx).Where("tenant_id = ?", tenantID).Order("queue_key asc").Find(&qs).Error
	return qs, err
}

func (r *queueRepo) Upsert(ctx context.Context, q *models.Queue) error {
	return r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "tenant_id"}, {Name: "queue_key"}},
		DoUpdates: clause.AssignmentColumns([]string{"label", "requeue_on_reply", "webhooks"}),
	}).Create(q).Error
}
