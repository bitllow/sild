package gormstore_test

import (
	"os"
	"testing"
	"time"

	"github.com/bitllow/sild/backend/internal/config"
	"github.com/bitllow/sild/backend/internal/store/gormstore"
	"github.com/bitllow/sild/backend/internal/store/models"
	"gorm.io/gorm"
)

// dialects under test: SQLite always; Postgres/MySQL when a DSN is provided
// (docker-compose up + SILD_TEST_POSTGRES_DSN / SILD_TEST_MYSQL_DSN).
func dialects(t *testing.T) []config.DB {
	t.Helper()
	dbs := []config.DB{{Driver: config.SQLite, DSN: t.TempDir() + "/m.db"}}
	if dsn := os.Getenv("SILD_TEST_POSTGRES_DSN"); dsn != "" {
		dbs = append(dbs, config.DB{Driver: config.Postgres, DSN: dsn})
	}
	if dsn := os.Getenv("SILD_TEST_MYSQL_DSN"); dsn != "" {
		dbs = append(dbs, config.DB{Driver: config.MySQL, DSN: dsn})
	}
	return dbs
}

// §1/§3: every table carries tenant_id, including the three the spec listing
// omitted (conversation_members, message_attachments, read_receipts). Migration
// is portable and idempotent across dialects.
func TestMigratePortableAndTenantScoped(t *testing.T) {
	for _, dbc := range dialects(t) {
		t.Run(string(dbc.Driver), func(t *testing.T) {
			db, err := gormstore.Open(&config.Config{DB: dbc})
			if err != nil {
				t.Fatalf("open: %v", err)
			}
			if err := gormstore.Migrate(db); err != nil {
				t.Fatalf("migrate: %v", err)
			}
			// idempotent
			if err := gormstore.Migrate(db); err != nil {
				t.Fatalf("re-migrate: %v", err)
			}
			for _, table := range []string{"conversation_members", "message_attachments", "read_receipts"} {
				if !hasColumn(t, db, table, "tenant_id") {
					t.Errorf("[%s] %s missing tenant_id", dbc.Driver, table)
				}
			}
		})
	}
}

func hasColumn(t *testing.T, db *gorm.DB, table, col string) bool {
	t.Helper()
	cols, err := db.Migrator().ColumnTypes(table)
	if err != nil {
		t.Fatalf("columns(%s): %v", table, err)
	}
	for _, c := range cols {
		if c.Name() == col {
			return true
		}
	}
	return false
}

// The kind backfill classifies rows that predate the conversations.kind column:
// an assignment-less conversation (defaulted to 'support' by AutoMigrate) is
// promoted to 'peer', while one carrying an assignment stays 'support'. This is
// what keeps a legacy peer conversation off the support surfaces after upgrade.
func TestBackfillConversationKind(t *testing.T) {
	for _, dbc := range dialects(t) {
		t.Run(string(dbc.Driver), func(t *testing.T) {
			db, err := gormstore.Open(&config.Config{DB: dbc})
			if err != nil {
				t.Fatalf("open: %v", err)
			}
			if err := gormstore.Migrate(db); err != nil {
				t.Fatalf("migrate: %v", err)
			}

			// Simulate pre-column rows: both default to 'support' (Kind left unset,
			// so the DB default applies) — one has an assignment, one does not.
			now := time.Now()
			for _, id := range []string{"c_legacy_peer", "c_legacy_support"} {
				if err := db.Create(&models.Conversation{
					ID: id, TenantID: "t1", Status: models.ConversationOpen, CreatedAt: now,
				}).Error; err != nil {
					t.Fatalf("insert %s: %v", id, err)
				}
			}
			if err := db.Create(&models.Assignment{
				ID: "a1", TenantID: "t1", ConversationID: "c_legacy_support",
				Status: models.AssignmentQueued, CreatedAt: now,
			}).Error; err != nil {
				t.Fatalf("insert assignment: %v", err)
			}

			// Re-run migration → the backfill runs.
			if err := gormstore.Migrate(db); err != nil {
				t.Fatalf("re-migrate: %v", err)
			}

			kindOf := func(id string) string {
				var k string
				if err := db.Raw("SELECT kind FROM conversations WHERE id = ?", id).Scan(&k).Error; err != nil {
					t.Fatalf("read kind %s: %v", id, err)
				}
				return k
			}
			if got := kindOf("c_legacy_peer"); got != "peer" {
				t.Fatalf("assignment-less legacy row: kind = %q, want peer", got)
			}
			if got := kindOf("c_legacy_support"); got != "support" {
				t.Fatalf("assigned legacy row: kind = %q, want support", got)
			}
		})
	}
}
