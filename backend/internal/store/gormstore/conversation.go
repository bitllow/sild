package gormstore

import (
	"context"
	"time"

	"github.com/bitllow/sild/backend/internal/store"
	"github.com/bitllow/sild/backend/internal/store/models"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type conversationRepo struct{ db *gorm.DB }

func (r *conversationRepo) Create(ctx context.Context, c *models.Conversation) error {
	return r.db.WithContext(ctx).Create(c).Error
}

func (r *conversationRepo) Get(ctx context.Context, tenantID, id string) (*models.Conversation, error) {
	var c models.Conversation
	if err := r.db.WithContext(ctx).First(&c, "tenant_id = ? AND id = ?", tenantID, id).Error; err != nil {
		return nil, translateErr(err)
	}
	return &c, nil
}

func (r *conversationRepo) UpdateStatus(ctx context.Context, tenantID, id string, status models.ConversationStatus) error {
	res := r.db.WithContext(ctx).Model(&models.Conversation{}).
		Where("tenant_id = ? AND id = ?", tenantID, id).Update("status", status)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return store.ErrNotFound
	}
	return nil
}

func (r *conversationRepo) TouchLastMessage(ctx context.Context, tenantID, convID string, at time.Time, preview string) error {
	return r.db.WithContext(ctx).Model(&models.Conversation{}).
		Where("tenant_id = ? AND id = ?", tenantID, convID).
		Updates(map[string]any{"last_message_at": at, "last_message_preview": preview}).Error
}

func (r *conversationRepo) ListForUser(ctx context.Context, tenantID, externalUserID string) ([]models.Conversation, error) {
	var cs []models.Conversation
	err := r.db.WithContext(ctx).
		Joins("JOIN conversation_members m ON m.conversation_id = conversations.id AND m.left_at IS NULL").
		Where("conversations.tenant_id = ? AND m.external_user_id = ?", tenantID, externalUserID).
		Order("conversations.created_at desc").
		Find(&cs).Error
	return cs, err
}

func (r *conversationRepo) ListArchivable(ctx context.Context, tenantID, idleBeforeMsgID string, limit int) ([]models.Conversation, error) {
	// closed conversations whose most recent message id is below the cutoff
	// (ULIDs sort by time, so an id below the cutoff == older than the cutoff).
	var cs []models.Conversation
	q := r.db.WithContext(ctx).
		Where("tenant_id = ? AND status = ?", tenantID, models.ConversationClosed)
	if idleBeforeMsgID != "" {
		q = q.Where(`NOT EXISTS (SELECT 1 FROM messages msg
			WHERE msg.conversation_id = conversations.id AND msg.id > ?)`, idleBeforeMsgID)
	}
	err := q.Limit(limit).Find(&cs).Error
	return cs, err
}

type memberRepo struct{ db *gorm.DB }

func (r *memberRepo) Add(ctx context.Context, m *models.ConversationMember) error {
	return r.db.WithContext(ctx).Create(m).Error
}

func (r *memberRepo) RemoveExternal(ctx context.Context, tenantID, convID, externalUserID string) error {
	res := r.db.WithContext(ctx).Model(&models.ConversationMember{}).
		Where("tenant_id = ? AND conversation_id = ? AND external_user_id = ? AND left_at IS NULL",
			tenantID, convID, externalUserID).
		Update("left_at", time.Now())
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return store.ErrNotFound
	}
	return nil
}

func (r *memberRepo) Get(ctx context.Context, tenantID, convID, externalUserID string) (*models.ConversationMember, error) {
	var m models.ConversationMember
	err := r.db.WithContext(ctx).
		First(&m, "tenant_id = ? AND conversation_id = ? AND external_user_id = ?", tenantID, convID, externalUserID).Error
	if err != nil {
		return nil, translateErr(err)
	}
	return &m, nil
}

func (r *memberRepo) IsActiveMember(ctx context.Context, tenantID, convID, externalUserID string) (bool, error) {
	var n int64
	err := r.db.WithContext(ctx).Model(&models.ConversationMember{}).
		Where("tenant_id = ? AND conversation_id = ? AND external_user_id = ? AND left_at IS NULL",
			tenantID, convID, externalUserID).Count(&n).Error
	return n > 0, err
}

func (r *memberRepo) ListActive(ctx context.Context, tenantID, convID string) ([]models.ConversationMember, error) {
	var ms []models.ConversationMember
	err := r.db.WithContext(ctx).
		Where("tenant_id = ? AND conversation_id = ? AND left_at IS NULL", tenantID, convID).
		Find(&ms).Error
	return ms, err
}

func (r *memberRepo) ListActiveForUser(ctx context.Context, tenantID, externalUserID string) ([]models.ConversationMember, error) {
	var ms []models.ConversationMember
	err := r.db.WithContext(ctx).
		Where("tenant_id = ? AND external_user_id = ? AND left_at IS NULL", tenantID, externalUserID).
		Find(&ms).Error
	return ms, err
}

func (r *memberRepo) CountActive(ctx context.Context, tenantID, convID string) (int, error) {
	var n int64
	err := r.db.WithContext(ctx).Model(&models.ConversationMember{}).
		Where("tenant_id = ? AND conversation_id = ? AND left_at IS NULL", tenantID, convID).Count(&n).Error
	return int(n), err
}

// Remap rewrites a member's external id to a real user, preserving history by
// also rewriting that user's messages and receipts within the conversation (§4.5).
func (r *memberRepo) Remap(ctx context.Context, tenantID, convID, fromExternalID, toExternalID string) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, m := range []any{&models.ConversationMember{}, &models.Message{}, &models.ReadReceipt{}} {
			if err := tx.Model(m).
				Where("tenant_id = ? AND conversation_id = ? AND external_user_id = ?", tenantID, convID, fromExternalID).
				Update("external_user_id", toExternalID).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

func (r *memberRepo) UpdateSearchText(ctx context.Context, tenantID, memberID, text string) error {
	return r.db.WithContext(ctx).Model(&models.ConversationMember{}).
		Where("tenant_id = ? AND id = ?", tenantID, memberID).
		Update("member_search_text", text).Error
}

type assignmentRepo struct{ db *gorm.DB }

func (r *assignmentRepo) Create(ctx context.Context, a *models.Assignment) error {
	return r.db.WithContext(ctx).Create(a).Error
}

func (r *assignmentRepo) Get(ctx context.Context, tenantID, id string) (*models.Assignment, error) {
	var a models.Assignment
	if err := r.db.WithContext(ctx).First(&a, "tenant_id = ? AND id = ?", tenantID, id).Error; err != nil {
		return nil, translateErr(err)
	}
	return &a, nil
}

func (r *assignmentRepo) GetByConversation(ctx context.Context, tenantID, convID string) (*models.Assignment, error) {
	var a models.Assignment
	err := r.db.WithContext(ctx).
		Where("tenant_id = ? AND conversation_id = ?", tenantID, convID).
		Order("created_at desc").First(&a).Error
	if err != nil {
		return nil, translateErr(err)
	}
	return &a, nil
}

func (r *assignmentRepo) Update(ctx context.Context, a *models.Assignment) error {
	return r.db.WithContext(ctx).Save(a).Error
}

func (r *assignmentRepo) ConversationIDs(ctx context.Context, tenantID string) ([]string, error) {
	var ids []string
	err := r.db.WithContext(ctx).Model(&models.Assignment{}).
		Where("tenant_id = ?", tenantID).
		Distinct().Pluck("conversation_id", &ids).Error
	return ids, err
}

// convLastActivity mirrors the COALESCE used for ordering: the denormalized last
// message time, or the creation time when there are no messages yet.
func convLastActivity(c models.Conversation) time.Time {
	if c.LastMessageAt != nil {
		return *c.LastMessageAt
	}
	return c.CreatedAt
}

func (r *assignmentRepo) ListQueue(ctx context.Context, tenantID string, p store.QueueParams) (store.QueuePage, error) {
	// Sort/keyset on the conversation's last activity (denormalized, COALESCE to
	// creation time when there are no messages yet). We ORDER BY / filter on the
	// expression but don't SELECT it — the computed column loses its type on
	// SQLite — and recompute LastActivity in Go from the loaded conversation.
	const laExpr = "COALESCE(c.last_message_at, c.created_at)"
	sortExpr := laExpr
	if p.Sort == store.QueueSortCreated {
		sortExpr = "a.created_at"
	}
	limit := p.Limit
	if limit <= 0 || limit > 100 {
		limit = 30
	}
	dir, cmp := "ASC", ">"
	if p.Desc {
		dir, cmp = "DESC", "<"
	}

	q := r.db.WithContext(ctx).Table("assignments AS a").
		Select("a.*").
		Joins("JOIN conversations c ON c.id = a.conversation_id").
		Where("a.tenant_id = ?", tenantID).
		// One row per conversation: a conversation can carry several assignments
		// (AddAssignment), but the queue is per-conversation. Keep only the latest
		// assignment (its current state) — the representative — before status
		// filtering and keyset pagination, so a conversation never duplicates rows,
		// pagination slots, or React keys. The subquery scans ALL of the
		// conversation's assignments (no status filter) so the representative is the
		// true latest; the status/assignee filters below then apply to it.
		Where(`NOT EXISTS (SELECT 1 FROM assignments a2
			WHERE a2.conversation_id = a.conversation_id
			  AND (a2.created_at > a.created_at OR (a2.created_at = a.created_at AND a2.id > a.id)))`)
	if p.Status != nil {
		q = q.Where("a.status = ?", *p.Status)
	}
	if p.Assignee != nil {
		q = q.Where("a.assignee_actor_id = ?", *p.Assignee)
	}
	if p.Cursor != nil {
		// keyset: rows strictly after the cursor in (sort, id) order.
		q = q.Where(sortExpr+" "+cmp+" ? OR ("+sortExpr+" = ? AND a.id "+cmp+" ?)",
			p.Cursor.Value, p.Cursor.Value, p.Cursor.ID)
	}
	var rows []models.Assignment
	if err := q.Order(sortExpr + " " + dir).Order("a.id " + dir).
		Limit(limit + 1).Scan(&rows).Error; err != nil {
		return store.QueuePage{}, err
	}

	page := store.QueuePage{}
	if len(rows) > limit {
		page.HasMore = true
		rows = rows[:limit]
	}
	if len(rows) == 0 {
		return page, nil
	}

	// Batch-load the page's conversations + active members (no message history).
	ids := make([]string, len(rows))
	for i := range rows {
		ids[i] = rows[i].ConversationID
	}
	var convs []models.Conversation
	if err := r.db.WithContext(ctx).Where("tenant_id = ? AND id IN ?", tenantID, ids).
		Find(&convs).Error; err != nil {
		return store.QueuePage{}, err
	}
	convByID := make(map[string]models.Conversation, len(convs))
	for _, c := range convs {
		convByID[c.ID] = c
	}
	var members []models.ConversationMember
	if err := r.db.WithContext(ctx).
		Where("tenant_id = ? AND conversation_id IN ? AND left_at IS NULL", tenantID, ids).
		Find(&members).Error; err != nil {
		return store.QueuePage{}, err
	}
	membersByConv := make(map[string][]models.ConversationMember)
	for _, m := range members {
		membersByConv[m.ConversationID] = append(membersByConv[m.ConversationID], m)
	}

	page.Items = make([]store.QueueItem, 0, len(rows))
	for i := range rows {
		conv := convByID[rows[i].ConversationID]
		page.Items = append(page.Items, store.QueueItem{
			Assignment:   rows[i],
			Conversation: conv,
			Members:      membersByConv[rows[i].ConversationID],
			LastActivity: convLastActivity(conv),
		})
	}
	if page.HasMore {
		last := page.Items[len(page.Items)-1]
		val := last.LastActivity
		if p.Sort == store.QueueSortCreated {
			val = last.Assignment.CreatedAt
		}
		page.NextCursor = &store.QueueCursor{Value: val, ID: last.Assignment.ID}
	}
	return page, nil
}

type receiptRepo struct{ db *gorm.DB }

// Upsert applies the monotonic read-receipt rule portably: read-modify-write in
// a transaction, ignoring an id older than the stored one (§3 GREATEST guard).
func (r *receiptRepo) Upsert(ctx context.Context, rr *models.ReadReceipt) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var existing models.ReadReceipt
		q := tx.Where("tenant_id = ? AND conversation_id = ? AND participant_kind = ?",
			rr.TenantID, rr.ConversationID, rr.ParticipantKind)
		q = applyParticipant(q, rr.ExternalUserID, rr.InternalActorID)
		err := q.First(&existing).Error
		switch err {
		case gorm.ErrRecordNotFound:
			return tx.Create(rr).Error
		case nil:
			if rr.LastReadMessageID > existing.LastReadMessageID { // ULID lexical == chronological
				existing.LastReadMessageID = rr.LastReadMessageID
				existing.UpdatedAt = time.Now()
				return tx.Save(&existing).Error
			}
			return nil // stale/duplicate — ignore
		default:
			return err
		}
	})
}

func (r *receiptRepo) Get(ctx context.Context, tenantID, convID string, p store.Participant) (*models.ReadReceipt, error) {
	var rr models.ReadReceipt
	q := r.db.WithContext(ctx).Where("tenant_id = ? AND conversation_id = ? AND participant_kind = ?",
		tenantID, convID, p.Kind)
	q = applyParticipant(q, p.ExternalUserID, p.InternalActorID)
	if err := q.First(&rr).Error; err != nil {
		return nil, translateErr(err)
	}
	return &rr, nil
}

// applyParticipant scopes a query to a participant by whichever id is set.
func applyParticipant(q *gorm.DB, external, internal *string) *gorm.DB {
	if external != nil {
		return q.Where("external_user_id = ?", *external)
	}
	if internal != nil {
		return q.Where("internal_actor_id = ?", *internal)
	}
	return q
}

type uploadRepo struct{ db *gorm.DB }

func (r *uploadRepo) Create(ctx context.Context, u *models.Upload) error {
	return r.db.WithContext(ctx).Create(u).Error
}

func (r *uploadRepo) GetByObjectKey(ctx context.Context, tenantID, objectKey string) (*models.Upload, error) {
	var u models.Upload
	if err := r.db.WithContext(ctx).First(&u, "tenant_id = ? AND object_key = ?", tenantID, objectKey).Error; err != nil {
		return nil, translateErr(err)
	}
	return &u, nil
}

func (r *uploadRepo) MarkCompleted(ctx context.Context, tenantID, objectKey string) error {
	return r.db.WithContext(ctx).Model(&models.Upload{}).
		Where("tenant_id = ? AND object_key = ?", tenantID, objectKey).
		Update("status", models.UploadCompleted).Error
}

type pushTokenRepo struct{ db *gorm.DB }

func (r *pushTokenRepo) Upsert(ctx context.Context, t *models.PushToken) error {
	t.UpdatedAt = time.Now()
	return r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "token"}},
		DoUpdates: clause.AssignmentColumns([]string{"external_user_id", "internal_actor_id", "platform", "updated_at", "tenant_id", "member_kind"}),
	}).Create(t).Error
}

func (r *pushTokenRepo) DeleteByToken(ctx context.Context, tenantID, token string, owner store.Participant) error {
	q := r.db.WithContext(ctx).Where("tenant_id = ? AND token = ?", tenantID, token)
	q = applyParticipant(q, owner.ExternalUserID, owner.InternalActorID)
	return q.Delete(&models.PushToken{}).Error
}

func (r *pushTokenRepo) ListForUser(ctx context.Context, tenantID, externalUserID string) ([]models.PushToken, error) {
	var ts []models.PushToken
	err := r.db.WithContext(ctx).
		Where("tenant_id = ? AND external_user_id = ?", tenantID, externalUserID).Find(&ts).Error
	return ts, err
}

var (
	_ store.ConversationRepo = (*conversationRepo)(nil)
	_ store.MemberRepo       = (*memberRepo)(nil)
	_ store.AssignmentRepo   = (*assignmentRepo)(nil)
	_ store.ReceiptRepo      = (*receiptRepo)(nil)
	_ store.UploadRepo       = (*uploadRepo)(nil)
	_ store.PushTokenRepo    = (*pushTokenRepo)(nil)
)
