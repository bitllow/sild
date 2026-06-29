package gormstore_test

import (
	"context"
	"fmt"
	"sort"
	"testing"
	"time"

	"github.com/bitllow/sild/backend/internal/config"
	"github.com/bitllow/sild/backend/internal/store"
	"github.com/bitllow/sild/backend/internal/store/gormstore"
	"github.com/bitllow/sild/backend/internal/store/models"
	"gorm.io/gorm"
)

// newQueueStore opens a migrated store for one dialect and wipes the tenant's
// rows so the test is isolated even on a shared (Postgres) test database.
func newQueueStore(t *testing.T, dbc config.DB, tenant string) store.Store {
	t.Helper()
	db, err := gormstore.Open(&config.Config{DB: dbc})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := gormstore.Migrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	wipeTenant(t, db, tenant)
	t.Cleanup(func() { wipeTenant(t, db, tenant) })
	return gormstore.New(db)
}

func wipeTenant(t *testing.T, db *gorm.DB, tenant string) {
	t.Helper()
	for _, table := range []string{"assignments", "conversation_members", "conversations"} {
		if err := db.Exec("DELETE FROM "+table+" WHERE tenant_id = ?", tenant).Error; err != nil {
			t.Fatalf("wipe %s: %v", table, err)
		}
	}
}

// seedRow is the test's source-of-truth for one queue row.
type seedRow struct {
	i       int
	lastAct time.Time
	created time.Time
}

func (s seedRow) convID(tenant string) string   { return fmt.Sprintf("%s_c%d", tenant, s.i) }
func (s seedRow) assignID(tenant string) string { return fmt.Sprintf("%s_as%d", tenant, s.i) }

func seedQueueRow(t *testing.T, st store.Store, tenant string, r seedRow) {
	t.Helper()
	ctx := context.Background()
	la := r.lastAct
	conv := &models.Conversation{
		ID: r.convID(tenant), TenantID: tenant, Status: models.ConversationOpen,
		CreatedAt: la, LastMessageAt: &la, LastMessagePreview: "hi",
	}
	if err := st.Conversations().Create(ctx, conv); err != nil {
		t.Fatalf("create conv: %v", err)
	}
	a := &models.Assignment{
		ID: r.assignID(tenant), TenantID: tenant, ConversationID: r.convID(tenant),
		Status: models.AssignmentQueued, CreatedAt: r.created,
	}
	if err := st.Assignments().Create(ctx, a); err != nil {
		t.Fatalf("create assignment: %v", err)
	}
}

func convOrder(p store.QueuePage) []string {
	ids := make([]string, len(p.Items))
	for i := range p.Items {
		ids[i] = p.Items[i].Conversation.ID
	}
	return ids
}

func eqIDs(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// expectedOrder computes the order the DB must produce for a given sort/order,
// independently of the query: sort by the chosen key, tiebroken by assignment id
// in the SAME direction as the primary key (matching the keyset ORDER BY).
func expectedOrder(tenant string, rows []seedRow, sortKey store.QueueSort, desc bool) []string {
	rs := append([]seedRow(nil), rows...)
	key := func(r seedRow) time.Time {
		if sortKey == store.QueueSortCreated {
			return r.created
		}
		return r.lastAct
	}
	sort.SliceStable(rs, func(i, j int) bool {
		ki, kj := key(rs[i]), key(rs[j])
		if !ki.Equal(kj) {
			if desc {
				return ki.After(kj)
			}
			return ki.Before(kj)
		}
		if desc {
			return rs[i].assignID(tenant) > rs[j].assignID(tenant)
		}
		return rs[i].assignID(tenant) < rs[j].assignID(tenant)
	})
	out := make([]string, len(rs))
	for i := range rs {
		out[i] = rs[i].convID(tenant)
	}
	return out
}

// TestListQueueSortAndCursor exercises EVERY (sort key × direction) combination
// and, for each, verifies both a single full page and a cursor walk reproduce the
// exact expected order — all rows, correct order, no gaps, no repeats — including
// ties on each sort value (tiebroken by assignment id).
func TestListQueueSortAndCursor(t *testing.T) {
	for _, dbc := range dialects(t) {
		t.Run(string(dbc.Driver), func(t *testing.T) {
			const tenant = "t_queue_modes"
			st := newQueueStore(t, dbc, tenant)
			ctx := context.Background()
			base := time.Now().UTC().Truncate(time.Second)
			at := func(min int) time.Time { return base.Add(time.Duration(min) * time.Minute) }

			// 8 rows. The two sort keys deliberately disagree, and each key has one
			// tie pair so the id tiebreaker is exercised in both sort modes:
			//   last activity: distinct except rows 3 & 4 (tie)
			//   created:       distinct except rows 6 & 7 (tie)
			rows := []seedRow{
				{i: 0, lastAct: at(0), created: at(7)},
				{i: 1, lastAct: at(1), created: at(6)},
				{i: 2, lastAct: at(2), created: at(5)},
				{i: 3, lastAct: at(3), created: at(4)},
				{i: 4, lastAct: at(3), created: at(3)}, // last-activity tie with row 3
				{i: 5, lastAct: at(5), created: at(2)},
				{i: 6, lastAct: at(6), created: at(1)},
				{i: 7, lastAct: at(7), created: at(1)}, // created tie with row 6
			}
			for _, r := range rows {
				seedQueueRow(t, st, tenant, r)
			}

			modes := []struct {
				name string
				sort store.QueueSort
				desc bool
			}{
				{"last_activity_desc", store.QueueSortLastActivity, true},
				{"last_activity_asc", store.QueueSortLastActivity, false},
				{"created_desc", store.QueueSortCreated, true},
				{"created_asc", store.QueueSortCreated, false},
			}

			for _, m := range modes {
				t.Run(m.name, func(t *testing.T) {
					want := expectedOrder(tenant, rows, m.sort, m.desc)

					// (1) single full page: exact order + every row present.
					full, err := st.Assignments().ListQueue(ctx, tenant, store.QueueParams{
						Sort: m.sort, Desc: m.desc, Limit: 100,
					})
					if err != nil {
						t.Fatalf("full: %v", err)
					}
					if full.HasMore || full.NextCursor != nil {
						t.Fatalf("full page should not report more")
					}
					if got := convOrder(full); !eqIDs(got, want) {
						t.Fatalf("full order\n got %v\nwant %v", got, want)
					}

					// (2) cursor walk in small pages reproduces the same order.
					var paged []string
					var cursor *store.QueueCursor
					steps := 0
					for {
						p, err := st.Assignments().ListQueue(ctx, tenant, store.QueueParams{
							Sort: m.sort, Desc: m.desc, Limit: 3, Cursor: cursor,
						})
						if err != nil {
							t.Fatalf("page %d: %v", steps, err)
						}
						if len(p.Items) > 3 {
							t.Fatalf("page returned %d > limit", len(p.Items))
						}
						paged = append(paged, convOrder(p)...)
						steps++
						if !p.HasMore {
							if p.NextCursor != nil {
								t.Fatalf("terminal page must not carry a cursor")
							}
							break
						}
						if p.NextCursor == nil {
							t.Fatalf("non-terminal page must carry a cursor")
						}
						cursor = p.NextCursor
						if steps > len(rows)+2 {
							t.Fatalf("pagination did not terminate")
						}
					}
					if !eqIDs(paged, want) {
						t.Fatalf("paged order\n got %v\nwant %v", paged, want)
					}
					if len(paged) != len(rows) {
						t.Fatalf("paged %d rows, want %d (gaps or repeats)", len(paged), len(rows))
					}
				})
			}
		})
	}
}

// Migrate backfills last_message_at + preview for legacy conversations whose rows
// predate the denormalized columns, picking the latest participant-visible,
// non-system message (a later system message must NOT win).
func TestBackfillLastActivity(t *testing.T) {
	for _, dbc := range dialects(t) {
		t.Run(string(dbc.Driver), func(t *testing.T) {
			const tenant = "t_backfill"
			db, err := gormstore.Open(&config.Config{DB: dbc})
			if err != nil {
				t.Fatalf("open: %v", err)
			}
			if err := gormstore.Migrate(db); err != nil {
				t.Fatalf("migrate: %v", err)
			}
			clean := func() {
				wipeTenant(t, db, tenant)
				db.Exec("DELETE FROM messages WHERE tenant_id = ?", tenant)
			}
			clean()
			t.Cleanup(clean)
			st := gormstore.New(db)
			ctx := context.Background()
			base := time.Now().UTC().Truncate(time.Second)

			// Legacy conversation: has messages but last_message_at left NULL.
			conv := &models.Conversation{
				ID: tenant + "_c", TenantID: tenant, Status: models.ConversationOpen, CreatedAt: base,
			}
			if err := st.Conversations().Create(ctx, conv); err != nil {
				t.Fatalf("create conv: %v", err)
			}
			ext := "u1"
			mk := func(body string, min int, kind models.SenderKind, vis models.Visibility) {
				m := &models.Message{
					TenantID: tenant, ConversationID: conv.ID, SenderKind: kind, Visibility: vis,
					Channel: models.ChannelApp, ExternalUserID: &ext, Body: body,
					CreatedAt: base.Add(time.Duration(min) * time.Minute),
				}
				if err := st.Messages().Create(ctx, m); err != nil {
					t.Fatalf("create msg: %v", err)
				}
			}
			mk("hello", 1, models.SenderUser, models.VisibilityParticipants)
			mk("world", 2, models.SenderUser, models.VisibilityParticipants) // latest qualifying
			mk("conversation closed", 3, models.SenderSystem, models.VisibilityParticipants) // newer but system → ignored

			pre, _ := st.Conversations().Get(ctx, tenant, conv.ID)
			if pre.LastMessageAt != nil {
				t.Fatalf("precondition: last_message_at should be NULL, got %v", pre.LastMessageAt)
			}

			// Migrate is idempotent and runs the backfill.
			if err := gormstore.Migrate(db); err != nil {
				t.Fatalf("re-migrate: %v", err)
			}

			got, _ := st.Conversations().Get(ctx, tenant, conv.ID)
			if got.LastMessageAt == nil {
				t.Fatalf("backfill did not set last_message_at")
			}
			if got.LastMessageAt.Unix() != base.Add(2*time.Minute).Unix() {
				t.Fatalf("last_message_at = %v, want the +2m participant message (not the +3m system one)", got.LastMessageAt.UTC())
			}
			if got.LastMessagePreview != "world" {
				t.Fatalf("preview = %q, want \"world\"", got.LastMessagePreview)
			}
		})
	}
}

// A conversation with multiple assignments (AddAssignment) appears exactly once,
// represented by its latest assignment — never duplicated across rows or pages.
func TestListQueueDedupesByConversation(t *testing.T) {
	for _, dbc := range dialects(t) {
		t.Run(string(dbc.Driver), func(t *testing.T) {
			const tenant = "t_queue_dedup"
			st := newQueueStore(t, dbc, tenant)
			ctx := context.Background()
			base := time.Now().UTC().Truncate(time.Second)

			// One conversation with two assignments: an older one and a newer one.
			conv := &models.Conversation{
				ID: tenant + "_c", TenantID: tenant, Status: models.ConversationOpen,
				CreatedAt: base, LastMessageAt: &base, LastMessagePreview: "hi",
			}
			if err := st.Conversations().Create(ctx, conv); err != nil {
				t.Fatalf("create conv: %v", err)
			}
			old := &models.Assignment{
				ID: tenant + "_old", TenantID: tenant, ConversationID: conv.ID,
				Status: models.AssignmentQueued, CreatedAt: base,
			}
			latest := &models.Assignment{
				ID: tenant + "_new", TenantID: tenant, ConversationID: conv.ID,
				Status: models.AssignmentAssigned, CreatedAt: base.Add(time.Minute),
			}
			for _, a := range []*models.Assignment{old, latest} {
				if err := st.Assignments().Create(ctx, a); err != nil {
					t.Fatalf("create assignment: %v", err)
				}
			}

			got, err := st.Assignments().ListQueue(ctx, tenant, store.QueueParams{Desc: true, Limit: 100})
			if err != nil {
				t.Fatalf("list: %v", err)
			}
			if len(got.Items) != 1 {
				t.Fatalf("conversation with 2 assignments produced %d rows, want 1", len(got.Items))
			}
			if got.Items[0].Assignment.ID != latest.ID {
				t.Fatalf("row carries assignment %s, want the latest %s", got.Items[0].Assignment.ID, latest.ID)
			}

			// The representative's status governs filtering: it's assigned, so it
			// must NOT appear under the queued filter even though an older queued
			// assignment exists.
			queued := models.AssignmentQueued
			q, _ := st.Assignments().ListQueue(ctx, tenant, store.QueueParams{Status: &queued, Desc: true, Limit: 100})
			if len(q.Items) != 0 {
				t.Fatalf("status=queued returned %d rows; the current assignment is assigned", len(q.Items))
			}
		})
	}
}

// status + assignee filters compose with pagination.
func TestListQueueFilters(t *testing.T) {
	for _, dbc := range dialects(t) {
		t.Run(string(dbc.Driver), func(t *testing.T) {
			const tenant = "t_queue_filter"
			st := newQueueStore(t, dbc, tenant)
			ctx := context.Background()
			base := time.Now().UTC().Truncate(time.Second)

			// 3 queued + 2 closed.
			for i := 0; i < 5; i++ {
				r := seedRow{i: i, lastAct: base.Add(time.Duration(i) * time.Minute), created: base}
				seedQueueRow(t, st, tenant, r)
				if i >= 3 {
					a, _ := st.Assignments().Get(ctx, tenant, r.assignID(tenant))
					a.Status = models.AssignmentClosed
					if err := st.Assignments().Update(ctx, a); err != nil {
						t.Fatalf("close: %v", err)
					}
				}
			}

			queued := models.AssignmentQueued
			got, err := st.Assignments().ListQueue(ctx, tenant, store.QueueParams{
				Status: &queued, Desc: true, Limit: 100,
			})
			if err != nil {
				t.Fatalf("filtered: %v", err)
			}
			if len(got.Items) != 3 {
				t.Fatalf("status=queued returned %d, want 3", len(got.Items))
			}
			for _, it := range got.Items {
				if it.Assignment.Status != models.AssignmentQueued {
					t.Fatalf("non-queued row leaked: %s", it.Assignment.Status)
				}
			}
		})
	}
}
