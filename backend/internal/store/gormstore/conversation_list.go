package gormstore

import (
	"context"
	"time"

	"github.com/bitllow/sild/backend/internal/policy"
	"github.com/bitllow/sild/backend/internal/store"
	"github.com/bitllow/sild/backend/internal/store/models"
	"gorm.io/gorm"
)

// repAssignment keeps one row per conversation: a conversation can carry several
// assignments (AddAssignment), but a list row is per-conversation. The subquery
// scans ALL of a conversation's assignments — no status filter — so the
// representative is the true latest; status/assignee filters then apply to it.
// Without this a conversation duplicates rows, pagination slots and React keys.
const repAssignment = `LEFT JOIN assignments a ON a.conversation_id = conversations.id
	AND NOT EXISTS (SELECT 1 FROM assignments a2
		WHERE a2.conversation_id = a.conversation_id
		  AND (a2.created_at > a.created_at OR (a2.created_at = a.created_at AND a2.id > a.id)))`

// lastActivityExpr is the denormalized ordering key. It is MUTABLE — every
// inbound message rewrites last_message_at — so paging on it is live-view, not
// snapshot: rows can move between pages while a client scrolls. Clients
// deduplicate by conversation id and refresh page one on a realtime reorder.
const lastActivityExpr = `COALESCE(conversations.last_message_at, conversations.created_at)`

func conversationSortExpr(k store.SortKey) string {
	switch k {
	case store.SortCreated:
		return "conversations.created_at"
	case store.SortWaitingSince:
		return "a.created_at"
	default:
		return lastActivityExpr
	}
}

// List is the single conversation list. It replaces ListQueue, ListPeers and
// ListForUser, which were this query with different hard-coded filters.
//
// scope is the policy ceiling and is applied first; q can only narrow it.
func (r *conversationRepo) List(ctx context.Context, scope policy.ResourceScope, q store.ConversationQuery) (store.Page[store.ConversationItem], error) {
	empty := store.Page[store.ConversationItem]{}
	if scope.DenyAll() {
		return empty, nil
	}

	// Select explicitly: the assignment join also has an `id`, and an unqualified
	// SELECT * across the join lets the scan pick the wrong one.
	db := r.db.WithContext(ctx).Model(&models.Conversation{}).
		Select("conversations.*").
		Joins(repAssignment).
		Where("conversations.tenant_id = ?", q.TenantID)

	db = applyScope(db, scope)
	db = applyFilters(db, q)

	page, err := Paginate(db, q.PageParams,
		conversationSortExpr(q.Sort), "conversations.id",
		func(c *models.Conversation) time.Time { return convSortValue(c, q.Sort) },
		func(c *models.Conversation) string { return c.ID },
	)
	if err != nil {
		return empty, err
	}
	out, err := r.enrich(ctx, q.TenantID, page)
	if err != nil {
		return empty, err
	}
	// waiting_since orders by the ASSIGNMENT's created_at, which is not a column
	// on the conversation row Paginate scanned — so its cursor value would be the
	// conversation's last activity and the next page would compare it against an
	// unrelated timestamp. enrich has just loaded the representative assignment,
	// so re-mint the cursor from the value the query actually ordered by.
	if q.Sort == store.SortWaitingSince && out.NextCursor != nil && len(out.Items) > 0 {
		last := out.Items[len(out.Items)-1]
		if last.Assignment == nil {
			// No assignment means no position on this sort key; stopping is
			// correct rather than emitting a cursor that cannot be resumed.
			out.NextCursor, out.HasMore = nil, false
		} else {
			v := last.Assignment.CreatedAt
			out.NextCursor.Value = &v
		}
	}
	return out, nil
}

// applyScope translates the policy ceiling into SQL. Everything downstream sees
// only rows the caller may see, so aggregates and ordering cannot leak what a
// later filter would have hidden.
func applyScope(db *gorm.DB, scope policy.ResourceScope) *gorm.DB {
	if kinds := scope.AllowedKinds(); len(kinds) > 0 {
		db = db.Where("conversations.kind IN ?", kinds)
	}
	if pt, ok := scope.Participant(); ok {
		db = db.Where(`EXISTS (SELECT 1 FROM conversation_members sm
			WHERE sm.conversation_id = conversations.id AND sm.left_at IS NULL
			  AND sm.external_user_id = ?)`, pt)
	}
	if scope.RequiresAssignment() {
		// An assignment must EXIST — anyone's, or nobody's. Not "assigned to the
		// caller": that is a client filter and lives on the query.
		//
		// Archived support conversations are exempt: PurgeHot deletes assignments
		// while retaining the conversation, and single-object policy admits an
		// archived formerly-support conversation for ANY agent. Without this the
		// list would hide exactly what a direct read allows.
		db = db.Where("conversations.kind <> ? OR a.id IS NOT NULL OR conversations.archived_at IS NOT NULL",
			models.KindSupport)
	}
	return db
}

func applyFilters(db *gorm.DB, q store.ConversationQuery) *gorm.DB {
	if q.Kind != nil {
		db = db.Where("conversations.kind = ?", *q.Kind)
	}
	if q.Status != nil {
		db = db.Where("conversations.status = ?", *q.Status)
	}
	if q.AssignmentStatus != nil {
		db = db.Where("a.status = ?", *q.AssignmentStatus)
	}
	if q.AssigneeActorID != "" {
		db = db.Where("a.assignee_actor_id = ?", q.AssigneeActorID)
	}
	if q.Unassigned {
		db = db.Where("a.assignee_actor_id IS NULL")
	}
	if q.Participant != "" {
		db = db.Where(`EXISTS (SELECT 1 FROM conversation_members fm
			WHERE fm.conversation_id = conversations.id AND fm.left_at IS NULL
			  AND fm.external_user_id = ?)`, q.Participant)
	}
	if q.ConvRole != "" {
		db = db.Where(`EXISTS (SELECT 1 FROM conversation_members rm
			WHERE rm.conversation_id = conversations.id AND rm.left_at IS NULL
			  AND rm.conv_role = ?)`, q.ConvRole)
	}
	if q.IDs != nil {
		db = db.Where("conversations.id IN ?", q.IDs)
	}
	return db
}

func convSortValue(c *models.Conversation, k store.SortKey) time.Time {
	if k == store.SortCreated {
		return c.CreatedAt
	}
	return convLastActivity(*c)
}

// enrich batch-loads members and representative assignments for a page — one
// query each, never per row.
func (r *conversationRepo) enrich(ctx context.Context, tenantID string, page store.Page[models.Conversation]) (store.Page[store.ConversationItem], error) {
	out := store.Page[store.ConversationItem]{NextCursor: page.NextCursor, HasMore: page.HasMore}
	if len(page.Items) == 0 {
		return out, nil
	}

	ids := make([]string, len(page.Items))
	for i := range page.Items {
		ids[i] = page.Items[i].ID
	}

	var members []models.ConversationMember
	if err := r.db.WithContext(ctx).
		Where("tenant_id = ? AND conversation_id IN ? AND left_at IS NULL", tenantID, ids).
		Find(&members).Error; err != nil {
		return out, err
	}
	byConv := make(map[string][]models.ConversationMember, len(ids))
	for _, m := range members {
		byConv[m.ConversationID] = append(byConv[m.ConversationID], m)
	}

	var assignments []models.Assignment
	if err := r.db.WithContext(ctx).
		Where("tenant_id = ? AND conversation_id IN ?", tenantID, ids).
		Order("created_at asc, id asc").
		Find(&assignments).Error; err != nil {
		return out, err
	}
	// Last write wins, matching repAssignment's "latest is representative".
	repByConv := make(map[string]*models.Assignment, len(ids))
	for i := range assignments {
		a := assignments[i]
		repByConv[a.ConversationID] = &a
	}

	out.Items = make([]store.ConversationItem, 0, len(page.Items))
	for i := range page.Items {
		c := page.Items[i]
		out.Items = append(out.Items, store.ConversationItem{
			Conversation: c,
			Members:      byConv[c.ID],
			Assignment:   repByConv[c.ID],
			LastActivity: convLastActivity(c),
		})
	}
	return out, nil
}
