package gormstore

import (
	"context"
	"time"

	"github.com/bitllow/sild/backend/internal/policy"
	"github.com/bitllow/sild/backend/internal/store"
	"github.com/bitllow/sild/backend/internal/store/models"
	"gorm.io/gorm"
)

// Contacts are a derived READ MODEL over conversation_members, keyed
// (tenant_id, external_user_id). The scope is applied BEFORE aggregation: a
// pre-aggregated row cannot be scoped safely, because its ordering value and
// metadata winner would encode rows the caller may not see.

type contactRepo struct{ db *gorm.DB }

// contactRow is the aggregate projected per contact. LastActivity is `any`
// because drivers disagree about a computed timestamp (SQLite text, Postgres
// time.Time).
type contactRow struct {
	ExternalUserID    string
	LastActivity      any
	ConversationCount int64
}

// sqlTimeLayouts covers the shapes the supported drivers return.
var sqlTimeLayouts = []string{
	time.RFC3339Nano,
	time.RFC3339,
	"2006-01-02 15:04:05.999999999-07:00",
	"2006-01-02 15:04:05.999999999Z07:00",
	"2006-01-02 15:04:05.999999999",
	"2006-01-02 15:04:05",
}

// parseSQLTime keeps the driver's ORIGINAL offset: the value round-trips through
// a cursor into a comparison against the stored column, and SQLite compares
// datetimes as strings.
func parseSQLTime(v any) time.Time {
	switch t := v.(type) {
	case time.Time:
		return t
	case []byte:
		return parseTimeText(string(t))
	case string:
		return parseTimeText(t)
	}
	return time.Time{}
}

func parseTimeText(v string) time.Time {
	for _, layout := range sqlTimeLayouts {
		if t, err := time.Parse(layout, v); err == nil {
			return t
		}
	}
	return time.Time{}
}

// contactBase builds the scope-filtered membership join every contact query
// starts from.
func (r *contactRepo) contactBase(ctx context.Context, tenantID string, scope policy.ResourceScope) *gorm.DB {
	q := r.db.WithContext(ctx).
		Table("conversation_members m").
		Joins("JOIN conversations c ON c.id = m.conversation_id").
		Joins(`LEFT JOIN assignments a ON a.conversation_id = c.id
			AND NOT EXISTS (SELECT 1 FROM assignments a2
				WHERE a2.conversation_id = a.conversation_id
				  AND (a2.created_at > a.created_at OR (a2.created_at = a.created_at AND a2.id > a.id)))`).
		Where("m.tenant_id = ? AND m.external_user_id IS NOT NULL AND m.external_user_id <> ''", tenantID)

	if kinds := scope.AllowedKinds(); len(kinds) > 0 {
		q = q.Where("c.kind IN ?", kinds)
	}
	if pt, ok := scope.Participant(); ok {
		q = q.Where("m.external_user_id = ?", pt)
	}
	if scope.RequiresAssignment() {
		// Archived support is exempt, or archived-only contacts vanish for agents.
		q = q.Where("c.kind <> ? OR a.id IS NOT NULL OR c.archived_at IS NOT NULL", models.KindSupport)
	}
	return q
}

// ListContacts returns one page of contacts. Existence spans ANY authorized
// membership (left or archived included); conversation_count only CURRENT ones.
func (r *contactRepo) ListContacts(ctx context.Context, tenantID string, scope policy.ResourceScope, q store.ContactQuery) (store.Page[store.Contact], error) {
	empty := store.Page[store.Contact]{}
	if scope.DenyAll() {
		return empty, nil
	}

	base := r.contactBase(ctx, tenantID, scope)
	if q.Exact != "" {
		base = base.Where("m.external_user_id = ?", q.Exact)
	}
	if q.Search != "" {
		like := "%" + q.Search + "%"
		base = base.Where("LOWER(m.member_search_text) LIKE LOWER(?) OR LOWER(m.external_user_id) LIKE LOWER(?)", like, like)
	}

	// The sort value is an aggregate, so the keyset predicate lives in HAVING.
	agg := base.Select(`m.external_user_id AS external_user_id,
		MAX(COALESCE(c.last_message_at, c.created_at)) AS last_activity,
		SUM(CASE WHEN m.left_at IS NULL AND c.archived_at IS NULL THEN 1 ELSE 0 END) AS conversation_count`).
		Group("m.external_user_id")

	limit := store.ClampLimit(q.Limit, 30)
	if cur := q.Cursor; cur != nil && cur.Value != nil {
		agg = agg.Having(
			`MAX(COALESCE(c.last_message_at, c.created_at)) < ?
			 OR (MAX(COALESCE(c.last_message_at, c.created_at)) = ? AND m.external_user_id < ?)`,
			cur.Value, cur.Value, cur.ID)
	}

	// Scanned row-by-row: gorm leaves an `any` field unpopulated, and drivers
	// disagree about a computed timestamp's type.
	sqlRows, err := agg.
		Order("last_activity DESC").
		Order("m.external_user_id DESC").
		Limit(limit + 1).Rows()
	if err != nil {
		return empty, err
	}
	var rows []contactRow
	for sqlRows.Next() {
		var r contactRow
		if err := sqlRows.Scan(&r.ExternalUserID, &r.LastActivity, &r.ConversationCount); err != nil {
			sqlRows.Close()
			return empty, err
		}
		rows = append(rows, r)
	}
	if err := sqlRows.Err(); err != nil {
		sqlRows.Close()
		return empty, err
	}
	if err := sqlRows.Close(); err != nil {
		return empty, err
	}

	page := store.Page[store.Contact]{}
	if len(rows) > limit {
		page.HasMore = true
		rows = rows[:limit]
	}
	if len(rows) == 0 {
		return page, nil
	}

	ids := make([]string, len(rows))
	for i := range rows {
		ids[i] = rows[i].ExternalUserID
	}
	meta, err := r.winningMetadata(ctx, tenantID, scope, ids)
	if err != nil {
		return empty, err
	}

	page.Items = make([]store.Contact, 0, len(rows))
	for _, row := range rows {
		page.Items = append(page.Items, store.Contact{
			ExternalUserID:    row.ExternalUserID,
			LastActivity:      parseSQLTime(row.LastActivity),
			ConversationCount: row.ConversationCount,
			Metadata:          meta[row.ExternalUserID],
		})
	}
	if page.HasMore {
		last := page.Items[len(page.Items)-1]
		v := last.LastActivity
		page.NextCursor = &store.Cursor{
			Key: store.SortLastActivity, Order: store.OrderDesc,
			Value: &v, ID: last.ExternalUserID,
		}
	}
	return page, nil
}

// winningMetadata takes each contact's metadata from their most recently joined
// AUTHORIZED membership. One blob wins whole — merging keys across memberships
// would fabricate a person who never existed.
func (r *contactRepo) winningMetadata(ctx context.Context, tenantID string, scope policy.ResourceScope, externalIDs []string) (map[string][]byte, error) {
	type row struct {
		ExternalUserID string
		Metadata       []byte
		JoinedAt       time.Time
		ID             string
	}
	var rows []row
	err := r.contactBase(ctx, tenantID, scope).
		Where("m.external_user_id IN ?", externalIDs).
		Select("m.external_user_id AS external_user_id, m.metadata AS metadata, m.joined_at AS joined_at, m.id AS id").
		Order("m.joined_at ASC").Order("m.id ASC").
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	out := make(map[string][]byte, len(externalIDs))
	for _, r := range rows { // ascending scan, so the last write is the newest
		out[r.ExternalUserID] = r.Metadata
	}
	return out, nil
}

// GetContact returns one contact, or ErrNotFound when the caller's scope admits
// none of their conversations — the same rule the list applies, so a contact
// cannot be probed for through the single-object route.
func (r *contactRepo) GetContact(ctx context.Context, tenantID string, scope policy.ResourceScope, externalUserID string) (*store.Contact, error) {
	page, err := r.ListContacts(ctx, tenantID, scope, store.ContactQuery{
		PageParams: store.PageParams{Limit: 1},
		Exact:      externalUserID,
	})
	if err != nil {
		return nil, err
	}
	if len(page.Items) == 0 {
		return nil, store.ErrNotFound
	}
	return &page.Items[0], nil
}
