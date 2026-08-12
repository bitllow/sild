package gormstore_test

import (
	"os"
	"testing"
	"time"

	"github.com/bitllow/sild/backend/internal/config"
	"github.com/bitllow/sild/backend/internal/id"
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
			bare := id.New(id.Conversation) // fresh: the dialect DBs persist between runs
			if err := db.Create(&models.Conversation{
				ID: bare, TenantID: "t1", Status: models.ConversationOpen,
				Kind: models.KindSupport, CreatedAt: time.Now(),
			}).Error; err != nil {
				t.Fatalf("insert: %v", err)
			}

			if err := gormstore.Migrate(db); err != nil {
				t.Fatalf("re-migrate: %v", err)
			}

			var kind string
			if err := db.Raw("SELECT kind FROM conversations WHERE id = ?", bare).Scan(&kind).Error; err != nil {
				t.Fatalf("read kind: %v", err)
			}
			if kind != string(models.KindSupport) {
				t.Fatalf("assignment-less conversation reclassified to %q, want support (backfill removed)", kind)
			}
		})
	}
}

// The profile move replaced per-membership metadata and the opt-out table.
// AutoMigrate never drops, so a database upgraded in place must be cleaned up
// explicitly — and the cleanup has to survive a second migration.
func TestMigrateDropsRetiredProfileObjects(t *testing.T) {
	for _, dbc := range dialects(t) {
		t.Run(string(dbc.Driver), func(t *testing.T) {
			db, err := gormstore.Open(&config.Config{DB: dbc})
			if err != nil {
				t.Fatalf("open: %v", err)
			}
			if err := gormstore.Migrate(db); err != nil {
				t.Fatalf("migrate: %v", err)
			}
			// Reintroduce the pre-move shape, then migrate over it.
			db.Exec(`ALTER TABLE conversation_members ADD COLUMN member_search_text text`)
			db.Exec(`CREATE TABLE push_opt_outs (tenant_id varchar(40), external_user_id varchar(255))`)
			if err := gormstore.Migrate(db); err != nil {
				t.Fatalf("re-migrate: %v", err)
			}

			if hasColumn(t, db, "conversation_members", "member_search_text") {
				t.Errorf("[%s] member_search_text survived", dbc.Driver)
			}
			if db.Migrator().HasTable("push_opt_outs") {
				t.Errorf("[%s] push_opt_outs survived", dbc.Driver)
			}
		})
	}
}

// An upgrade must carry the retired role column and peer flag into assignments,
// or every existing member authenticates holding nothing.
func TestMigrationCarriesRetiredRolesIntoAssignments(t *testing.T) {
	for _, dbc := range dialects(t) {
		t.Run(string(dbc.Driver), func(t *testing.T) {
			db, err := gormstore.Open(&config.Config{DB: dbc})
			if err != nil {
				t.Fatalf("open: %v", err)
			}
			if err := gormstore.Migrate(db); err != nil {
				t.Fatalf("migrate: %v", err)
			}
			// Rebuild the shape the previous release left behind.
			if err := db.Exec("ALTER TABLE admin_users ADD COLUMN platform_role varchar(16)").Error; err != nil {
				t.Fatalf("re-add platform_role: %v", err)
			}
			if err := db.Exec("ALTER TABLE admin_users ADD COLUMN peer_access boolean").Error; err != nil {
				t.Fatalf("re-add peer_access: %v", err)
			}
			tenantID := id.New(id.Tenant)
			adminID := id.New(id.AdminUser)
			err = db.Exec(`INSERT INTO admin_users (id, tenant_id, email, platform_role, peer_access, created_at)
			               VALUES (?, ?, ?, ?, ?, ?)`,
				adminID, tenantID, "legacy@test", string(models.PlatformAdmin), true, time.Now()).Error
			if err != nil {
				t.Fatalf("seed legacy admin: %v", err)
			}

			if err := gormstore.Migrate(db); err != nil {
				t.Fatalf("re-migrate: %v", err)
			}

			var held []models.RoleAssignment
			if err := db.Where("admin_user_id = ?", adminID).Find(&held).Error; err != nil {
				t.Fatalf("read assignments: %v", err)
			}
			byRole := map[models.PlatformRole]models.RoleScope{}
			for _, a := range held {
				byRole[a.Role] = a.Scope
			}
			if _, ok := byRole[models.PlatformAdmin]; !ok {
				t.Fatalf("the retired role did not become an assignment: %v", held)
			}
			// Peer access was the person's whatever their role, so it survives as the
			// agent assignment that now carries it.
			if !byRole[models.PlatformAgent].Peer {
				t.Fatalf("peer access was lost: %v", held)
			}
			if db.Migrator().HasColumn("admin_users", "platform_role") {
				t.Error("the retired column is still there after the backfill")
			}

			// An ungranted translator used to reach every project and language; the
			// empty set now means the opposite, so the move has to spell it out.
			if err := db.Exec("ALTER TABLE admin_users ADD COLUMN platform_role varchar(16)").Error; err != nil {
				t.Fatalf("re-add platform_role: %v", err)
			}
			if err := db.Exec("ALTER TABLE admin_users ADD COLUMN peer_access boolean").Error; err != nil {
				t.Fatalf("re-add peer_access: %v", err)
			}
			translatorID := id.New(id.AdminUser)
			err = db.Exec(`INSERT INTO admin_users (id, tenant_id, email, platform_role, peer_access, created_at)
			               VALUES (?, ?, ?, ?, ?, ?)`,
				translatorID, tenantID, "legacy-translator@test", string(models.PlatformTranslator), false, time.Now()).Error
			if err != nil {
				t.Fatalf("seed legacy translator: %v", err)
			}
			if err := gormstore.Migrate(db); err != nil {
				t.Fatalf("re-migrate: %v", err)
			}
			var translator models.RoleAssignment
			err = db.Where("admin_user_id = ? AND role = ?", translatorID, models.PlatformTranslator).
				First(&translator).Error
			if err != nil {
				t.Fatalf("read translator assignment: %v", err)
			}
			if len(translator.Scope.Projects) == 0 || translator.Scope.Projects[0] != models.ScopeAll {
				t.Fatalf("an unrestricted translator lost their projects: %+v", translator.Scope)
			}
			if len(translator.Scope.Locales) == 0 || translator.Scope.Locales[0] != models.ScopeAll {
				t.Fatalf("an unrestricted translator lost their languages: %+v", translator.Scope)
			}
		})
	}
}
