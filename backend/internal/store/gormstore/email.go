package gormstore

import (
	"context"
	"time"

	"github.com/bitllow/sild/backend/internal/id"
	"github.com/bitllow/sild/backend/internal/store"
	"github.com/bitllow/sild/backend/internal/store/models"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type emailRepo struct{ db *gorm.DB }

func (r *emailRepo) CreateThread(ctx context.Context, t *models.EmailThread) error {
	return r.db.WithContext(ctx).Create(t).Error
}

// ClaimIngest inserts the (tenant, message-id) pair, letting the primary key
// decide: the insert that lands is the one delivery that gets to ingest. An
// existing row may still be claimable — an incomplete claim older than staleAfter
// belonged to a process that died mid-ingest.
func (r *emailRepo) ClaimIngest(ctx context.Context, tenantID, messageID string, staleAfter time.Duration) (store.IngestClaim, error) {
	now := time.Now()
	owner := id.New(id.Holder)
	rec := &models.EmailIngest{TenantID: tenantID, MessageID: messageID, Owner: owner, ClaimedAt: now}
	res := r.db.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(rec)
	if res.Error != nil {
		return store.IngestClaim{}, res.Error
	}
	if res.RowsAffected > 0 {
		return store.IngestClaim{Owner: owner}, nil
	}
	res = r.db.WithContext(ctx).Model(&models.EmailIngest{}).
		Where("tenant_id = ? AND message_id = ? AND completed_at IS NULL AND claimed_at < ?",
			tenantID, messageID, now.Add(-staleAfter)).
		Updates(map[string]any{"owner": owner, "claimed_at": now})
	if res.Error != nil {
		return store.IngestClaim{}, res.Error
	}
	if res.RowsAffected > 0 {
		return store.IngestClaim{Owner: owner}, nil
	}
	var existing models.EmailIngest
	if err := r.db.WithContext(ctx).
		First(&existing, "tenant_id = ? AND message_id = ?", tenantID, messageID).Error; err != nil {
		return store.IngestClaim{}, translateErr(err)
	}
	return store.IngestClaim{Done: existing.CompletedAt != nil, InFlight: existing.CompletedAt == nil}, nil
}

// CompleteIngest closes the claim. Only a completed claim refuses a redelivery.
func (r *emailRepo) CompleteIngest(ctx context.Context, tenantID, messageID, owner string) error {
	return r.db.WithContext(ctx).Model(&models.EmailIngest{}).
		Where("tenant_id = ? AND message_id = ? AND owner = ?", tenantID, messageID, owner).
		Update("completed_at", time.Now()).Error
}

func (r *emailRepo) ReleaseIngest(ctx context.Context, tenantID, messageID, owner string) error {
	return r.db.WithContext(ctx).
		Where("tenant_id = ? AND message_id = ? AND owner = ?", tenantID, messageID, owner).
		Delete(&models.EmailIngest{}).Error
}

func (r *emailRepo) FindOpenByToken(ctx context.Context, tenantID, token string) (*models.EmailThread, error) {
	var t models.EmailThread
	err := r.db.WithContext(ctx).
		Joins("JOIN conversations c ON c.id = email_threads.conversation_id").
		Where("email_threads.tenant_id = ? AND email_threads.thread_token = ? AND c.status = ?",
			tenantID, token, models.ConversationOpen).
		First(&t).Error
	if err != nil {
		return nil, translateErr(err)
	}
	return &t, nil
}

func (r *emailRepo) FindOpenBySenderSubject(ctx context.Context, tenantID, sender, subjectKey string) (*models.EmailThread, error) {
	var t models.EmailThread
	err := r.db.WithContext(ctx).
		Joins("JOIN conversations c ON c.id = email_threads.conversation_id").
		Where("email_threads.tenant_id = ? AND email_threads.sender = ? AND email_threads.subject_key = ? AND c.status = ?",
			tenantID, sender, subjectKey, models.ConversationOpen).
		Order("c.created_at DESC").
		First(&t).Error
	if err != nil {
		return nil, translateErr(err)
	}
	return &t, nil
}

func (r *emailRepo) Get(ctx context.Context, tenantID, convID string) (*models.EmailThread, error) {
	var t models.EmailThread
	if err := r.db.WithContext(ctx).First(&t, "tenant_id = ? AND conversation_id = ?", tenantID, convID).Error; err != nil {
		return nil, translateErr(err)
	}
	return &t, nil
}

// Subjects batch-loads subjects for a page of conversations.
func (r *emailRepo) Subjects(ctx context.Context, tenantID string, convIDs []string) (map[string]string, error) {
	out := make(map[string]string, len(convIDs))
	if len(convIDs) == 0 {
		return out, nil
	}
	var rows []struct {
		ConversationID string
		Subject        string
	}
	if err := r.db.WithContext(ctx).Model(&models.EmailThread{}).
		Select("conversation_id, subject").
		Where("tenant_id = ? AND conversation_id IN ?", tenantID, convIDs).
		Scan(&rows).Error; err != nil {
		return nil, err
	}
	for _, row := range rows {
		out[row.ConversationID] = row.Subject
	}
	return out, nil
}

func (r *emailRepo) Update(ctx context.Context, t *models.EmailThread) error {
	return r.db.WithContext(ctx).Save(t).Error
}

var _ store.EmailRepo = (*emailRepo)(nil)
