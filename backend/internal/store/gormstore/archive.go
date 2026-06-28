package gormstore

import (
	"context"

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

// PurgeHot deletes all hot rows for a conversation (§12 step 3). Must run inside
// the archival transaction, after the sink write is confirmed.
func (r *archiveRepo) PurgeHot(ctx context.Context, tenantID, convID string) error {
	tx := r.db.WithContext(ctx)
	// attachments first (FK to messages), then the rest.
	if err := tx.Where("tenant_id = ? AND message_id IN (SELECT id FROM messages WHERE conversation_id = ?)",
		tenantID, convID).Delete(&models.MessageAttachment{}).Error; err != nil {
		return err
	}
	for _, m := range []any{
		&models.Message{}, &models.ReadReceipt{}, &models.Assignment{},
		&models.ConversationMember{}, &models.EmailThread{},
	} {
		if err := tx.Where("tenant_id = ? AND conversation_id = ?", tenantID, convID).Delete(m).Error; err != nil {
			return err
		}
	}
	return tx.Where("tenant_id = ? AND id = ?", tenantID, convID).Delete(&models.Conversation{}).Error
}

var _ store.ArchiveRepo = (*archiveRepo)(nil)
