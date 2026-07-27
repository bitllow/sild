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

type webhookRepo struct{ db *gorm.DB }

func (r *webhookRepo) Create(ctx context.Context, e *models.WebhookEndpoint) error {
	return r.db.WithContext(ctx).Create(e).Error
}

func (r *webhookRepo) List(ctx context.Context, tenantID string) ([]models.WebhookEndpoint, error) {
	var es []models.WebhookEndpoint
	err := r.db.WithContext(ctx).Preload("Events").
		Where("tenant_id = ?", tenantID).Order("created_at desc").Find(&es).Error
	return es, err
}

func (r *webhookRepo) SetActive(ctx context.Context, tenantID, id string, active bool) error {
	res := r.db.WithContext(ctx).Model(&models.WebhookEndpoint{}).
		Where("tenant_id = ? AND id = ?", tenantID, id).Update("active", active)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return store.ErrNotFound
	}
	return nil
}

func (r *webhookRepo) Delete(ctx context.Context, tenantID, id string) error {
	res := r.db.WithContext(ctx).Where("tenant_id = ? AND id = ?", tenantID, id).Delete(&models.WebhookEndpoint{})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return store.ErrNotFound
	}
	return nil
}

func (r *webhookRepo) ListForEvent(ctx context.Context, tenantID, event string) ([]models.WebhookEndpoint, error) {
	var es []models.WebhookEndpoint
	err := r.db.WithContext(ctx).Preload("Events").
		Joins("JOIN webhook_events ev ON ev.endpoint_id = webhook_endpoints.id").
		Where("webhook_endpoints.tenant_id = ? AND webhook_endpoints.active = ? AND ev.event = ?",
			tenantID, true, event).
		Find(&es).Error
	return es, err
}

func (r *webhookRepo) LogDelivery(ctx context.Context, d *models.WebhookDelivery) error {
	return r.db.WithContext(ctx).Create(d).Error
}

func (r *webhookRepo) ListDeliveries(ctx context.Context, tenantID, endpointID string) ([]models.WebhookDelivery, error) {
	var ds []models.WebhookDelivery
	err := r.db.WithContext(ctx).
		Where("tenant_id = ? AND endpoint_id = ?", tenantID, endpointID).
		Order("created_at desc").Limit(200).Find(&ds).Error
	return ds, err
}

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

// ClaimDue returns only the rows this caller's lock won — a second relay's UPDATE
// matches nothing already locked, so no event is delivered twice. The token is
// what lets the winner read its own rows back.
func (r *outboxRepo) ClaimDue(ctx context.Context, limit int) ([]models.Outbox, string, error) {
	now := time.Now()
	var ids []string
	err := unclaimed(r.db.WithContext(ctx).Model(&models.Outbox{}), now).
		Where("status = ? AND available_at <= ?", models.DeliveryPending, now).
		Order("available_at").Limit(limit).Pluck("id", &ids).Error
	if err != nil || len(ids) == 0 {
		return nil, "", err
	}
	token := id.New(id.Holder)
	res := unclaimed(r.db.WithContext(ctx).Model(&models.Outbox{}), now).
		Where("id IN ? AND status = ?", ids, models.DeliveryPending).
		Updates(map[string]any{"claim_token": token, "locked_until": now.Add(store.OutboxClaimTTL)})
	if res.Error != nil {
		return nil, "", res.Error
	}
	var os []models.Outbox
	err = r.db.WithContext(ctx).Where("claim_token = ?", token).Order("available_at").Find(&os).Error
	return os, token, err
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

var (
	_ store.WebhookRepo = (*webhookRepo)(nil)
	_ store.OutboxRepo  = (*outboxRepo)(nil)
	_ store.EmailRepo   = (*emailRepo)(nil)
)
