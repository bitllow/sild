package gormstore

import (
	"context"
	"time"

	"github.com/bitllow/sild/backend/internal/policy"
	"github.com/bitllow/sild/backend/internal/store"
	"github.com/bitllow/sild/backend/internal/store/models"
	"gorm.io/gorm"
)

// Contacts are a derived READ MODEL over conversation_members joined to
// conversations — there is no contacts table. A contact is the pair
// (tenant_id, external_user_id); membership ULIDs are never exposed, because
// they identify a membership, not a person.
//
// The scope is applied to membership rows BEFORE aggregation. That ordering is
// the whole design: a pre-aggregated row per contact cannot be scoped safely,
// because the metadata winner would be chosen from rows the caller may not see,
// the ordering value would encode hidden activity (and so would the cursor), and
// filtering a fetched page yields short pages and skipped rows.

type contactRepo struct{ db *gorm.DB }

// contactRow is the aggregate projected per contact.
//
// LastActivity is scanned as `any` because drivers disagree about a computed
// timestamp: SQLite hands back text (the same reason ListQueue never SELECTs its
// sort expression), Postgres a real time.Time.
type contactRow struct {
	ExternalUserID    string
	LastActivity      any
	ConversationCount int64
}

// sqlTimeLayouts covers what the supported drivers hand back for a computed
// timestamp.
var sqlTimeLayouts = []string{
	time.RFC3339Nano,
	time.RFC3339,
	"2006-01-02 15:04:05.999999999-07:00",
	"2006-01-02 15:04:05.999999999Z07:00",
	"2006-01-02 15:04:05.999999999",
	"2006-01-02 15:04:05",
}

func parseSQLTime(v any) time.Time {
	switch t := v.(type) {
	case time.Time:
		return t.UTC()
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
			return t.UTC()
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
		q = q.Where("c.kind <> ? OR a.id IS NOT NULL", models.KindSupport)
	}
	return q
}

// ListContacts returns one page of contacts.
//
// Existence and activity are different questions and range over different sets:
// a contact appears if they have ANY authorized membership (left or archived
// included), while conversation_count counts only CURRENT ones. That is what
// keeps an archived-only contact findable with a count of zero.
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

	// Aggregate over the authorized rows only. The sort value is an aggregate, so
	// the keyset predicate lives in HAVING rather than WHERE.
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

	// Scanned row-by-row rather than through gorm's struct mapping: drivers
	// disagree about the type of a computed timestamp (SQLite text, Postgres
	// time.Time) and a struct field typed `any` is left unpopulated. Taking the
	// driver's own value keeps this portable.
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

// winningMetadata picks each contact's metadata from their most recently joined
// AUTHORIZED membership, tiebroken by membership id.
//
// One membership's blob wins whole; keys are never unioned across memberships.
// Merging would fabricate a person who never existed — a phone from one trip, a
// plate from another — and make the result order-dependent. A single snapshot is
// explainable: this is what we knew when they last joined.
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
