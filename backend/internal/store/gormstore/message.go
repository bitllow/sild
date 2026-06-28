package gormstore

import (
	"context"

	"github.com/bitllow/sild/backend/internal/store"
	"github.com/bitllow/sild/backend/internal/store/models"
	"gorm.io/gorm"
)

type messageRepo struct{ db *gorm.DB }

func (r *messageRepo) Create(ctx context.Context, m *models.Message) error {
	return r.db.WithContext(ctx).Create(m).Error
}

func (r *messageRepo) Get(ctx context.Context, tenantID, id string) (*models.Message, error) {
	var m models.Message
	if err := r.db.WithContext(ctx).Preload("Attachments").
		First(&m, "tenant_id = ? AND id = ?", tenantID, id).Error; err != nil {
		return nil, translateErr(err)
	}
	return &m, nil
}

func (r *messageRepo) FindByClientMsgID(ctx context.Context, tenantID, convID, clientMsgID string) (*models.Message, error) {
	var m models.Message
	err := r.db.WithContext(ctx).Preload("Attachments").
		First(&m, "tenant_id = ? AND conversation_id = ? AND client_msg_id = ?", tenantID, convID, clientMsgID).Error
	if err != nil {
		return nil, translateErr(err)
	}
	return &m, nil
}

// visibilityScope hides internal notes from non-agent callers (§5.6).
func visibilityScope(q *gorm.DB, includeInternal bool) *gorm.DB {
	if includeInternal {
		return q
	}
	return q.Where("visibility = ?", models.VisibilityParticipants)
}

func (r *messageRepo) ListBefore(ctx context.Context, tenantID, convID, before string, limit int, includeInternal bool) (*store.MessagePage, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	q := r.db.WithContext(ctx).Preload("Attachments").
		Where("tenant_id = ? AND conversation_id = ?", tenantID, convID)
	q = visibilityScope(q, includeInternal)
	if before != "" {
		q = q.Where("id < ?", before)
	}
	var ms []models.Message
	if err := q.Order("id desc").Limit(limit + 1).Find(&ms).Error; err != nil {
		return nil, err
	}
	page := &store.MessagePage{}
	if len(ms) > limit {
		page.HasMore = true
		ms = ms[:limit]
	}
	// return ascending (oldest first)
	for i, j := 0, len(ms)-1; i < j; i, j = i+1, j-1 {
		ms[i], ms[j] = ms[j], ms[i]
	}
	page.Messages = ms
	return page, nil
}

func (r *messageRepo) ListAfter(ctx context.Context, tenantID, convID, after string, includeInternal bool) ([]models.Message, error) {
	q := r.db.WithContext(ctx).Preload("Attachments").
		Where("tenant_id = ? AND conversation_id = ?", tenantID, convID)
	q = visibilityScope(q, includeInternal)
	if after != "" {
		q = q.Where("id > ?", after)
	}
	var ms []models.Message
	err := q.Order("id asc").Limit(500).Find(&ms).Error
	return ms, err
}

func (r *messageRepo) Last(ctx context.Context, tenantID, convID string, includeInternal bool) (*models.Message, error) {
	q := r.db.WithContext(ctx).Where("tenant_id = ? AND conversation_id = ?", tenantID, convID)
	q = visibilityScope(q, includeInternal)
	var m models.Message
	if err := q.Order("id desc").First(&m).Error; err != nil {
		return nil, translateErr(err)
	}
	return &m, nil
}

func (r *messageRepo) UnreadCount(ctx context.Context, tenantID, convID, lastReadMessageID string, includeInternal bool) (int, error) {
	q := r.db.WithContext(ctx).Model(&models.Message{}).
		Where("tenant_id = ? AND conversation_id = ?", tenantID, convID)
	q = visibilityScope(q, includeInternal)
	if lastReadMessageID != "" {
		q = q.Where("id > ?", lastReadMessageID)
	}
	var n int64
	err := q.Count(&n).Error
	return int(n), err
}

var _ store.MessageRepo = (*messageRepo)(nil)
