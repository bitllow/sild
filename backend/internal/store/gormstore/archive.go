package gormstore

import (
	"context"
	"time"

	"github.com/bitllow/sild/backend/internal/store"
	"github.com/bitllow/sild/backend/internal/store/models"
	"gorm.io/gorm"
)

type archiveRepo struct{ db *gorm.DB }

func (r *archiveRepo) CreateTombstone(ctx context.Context, a *models.ConversationArchive) error {
	return r.db.WithContext(ctx).Create(a).Error
}

func (r *archiveRepo) GetTombstone(ctx context.Context, tenantID, convID string) (*models.ConversationArchive, error) {
	var a models.ConversationArchive
	if err := r.db.WithContext(ctx).
		First(&a, "tenant_id = ? AND conversation_id = ?", tenantID, convID).Error; err != nil {
		return nil, translateErr(err)
	}
	return &a, nil
}

// PurgeHot removes a conversation's message BULK (§12 step 3) and marks the
// conversation archived. Must run inside the archival transaction, after the
// sink write is confirmed.
//
// The conversation row and its members are deliberately RETAINED. Archival
// exists to reclaim unbounded message history, not the handful of rows
// describing who took part — and deleting those is what made contacts vanish on
// a retention job and forced archived-conversation authorization to rehydrate a
// membership snapshot out of the tombstone JSON.
func (r *archiveRepo) PurgeHot(ctx context.Context, tenantID, convID string) error {
	tx := r.db.WithContext(ctx)
	// attachments first (FK to messages), then the rest of the bulk.
	if err := tx.Where("tenant_id = ? AND message_id IN (SELECT id FROM messages WHERE conversation_id = ?)",
		tenantID, convID).Delete(&models.MessageAttachment{}).Error; err != nil {
		return err
	}
	for _, m := range []any{
		&models.Message{}, &models.ReadReceipt{}, &models.Assignment{}, &models.EmailThread{},
	} {
		if err := tx.Where("tenant_id = ? AND conversation_id = ?", tenantID, convID).Delete(m).Error; err != nil {
			return err
		}
	}
	now := time.Now().UTC()
	return tx.Model(&models.Conversation{}).
		Where("tenant_id = ? AND id = ?", tenantID, convID).
		Update("archived_at", now).Error
}

var _ store.ArchiveRepo = (*archiveRepo)(nil)
