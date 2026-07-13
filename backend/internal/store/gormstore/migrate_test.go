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

// Review fix: the kind backfill was removed. A pre-existing assignment-less
// conversation (defaulted to 'support' by AutoMigrate) must therefore stay
// 'support' across a re-migration — it must NOT be silently reclassified to
// 'peer' (which would drop it from the support surfaces and expose it on the peer
// surface on upgrade). New conversations get their kind at creation instead.
func TestMigrateDoesNotReclassifyAssignmentless(t *testing.T) {
	for _, dbc := range dialects(t) {
		t.Run(string(dbc.Driver), func(t *testing.T) {
			db, err := gormstore.Open(&config.Config{DB: dbc})
			if err != nil {
				t.Fatalf("open: %v", err)
			}
			if err := gormstore.Migrate(db); err != nil {
				t.Fatalf("migrate: %v", err)
			}

			// An assignment-less conversation, classified support (the AutoMigrate
			// default), carrying no assignment — exactly what the removed backfill
			// used to promote to peer.
			if err := db.Create(&models.Conversation{
				ID: "c_bare", TenantID: "t1", Status: models.ConversationOpen,
				Kind: models.KindSupport, CreatedAt: time.Now(),
			}).Error; err != nil {
				t.Fatalf("insert: %v", err)
			}

			if err := gormstore.Migrate(db); err != nil {
				t.Fatalf("re-migrate: %v", err)
			}

			var kind string
			if err := db.Raw("SELECT kind FROM conversations WHERE id = ?", "c_bare").Scan(&kind).Error; err != nil {
				t.Fatalf("read kind: %v", err)
			}
			if kind != string(models.KindSupport) {
				t.Fatalf("assignment-less conversation reclassified to %q, want support (backfill removed)", kind)
			}
		})
	}
}
