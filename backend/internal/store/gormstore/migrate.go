package gormstore

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/bitllow/sild/backend/internal/config"
	"github.com/bitllow/sild/backend/internal/store/models"
	"gorm.io/gorm"
)

// Migrate builds the schema on any dialect via AutoMigrate, then applies the
// dialect-specific search indexes (ARCHITECTURE §4). Idempotent, and serialized
// across processes: two overlapping deploys wait for each other instead of issuing
// concurrent DDL against the same tables.
func Migrate(db *gorm.DB) error {
	return withMigrationLock(db, func(db *gorm.DB) error {
		if err := db.AutoMigrate(models.All()...); err != nil {
			return err
		}
		if err := backfillLastActivity(db); err != nil {
			return err
		}
		if err := backfillContactNames(db); err != nil {
			return err
		}
		if err := backfillRoleAssignments(db); err != nil {
			return err
		}
		if err := dropRetiredObjects(db); err != nil {
			return err
		}
		return applyDialectIndexes(db)
	})
}

// Lock identity: arbitrary, but must stay stable across releases. Postgres takes an
// int64, MySQL a string.
const (
	migrationLockKey  = 8265168776373
	migrationLockName = "sild_schema_migration"
	// Seconds MySQL waits for the holder, kept under the deploy's own 300s wait for
	// the migration Job so a queued migrator still finishes inside it.
	migrationLockWait = 120
)

// withMigrationLock runs fn holding a cluster-wide lock. Both dialect locks are
// session-scoped, so one dedicated connection holds it while fn keeps using the pool.
func withMigrationLock(db *gorm.DB, fn func(*gorm.DB) error) error {
	dialect := Dialect(db)
	if dialect == config.SQLite {
		return fn(db) // one file, one writer; no advisory lock exists
	}
	sqlDB, err := db.DB()
	if err != nil {
		return fmt.Errorf("migration lock connection: %w", err)
	}
	ctx := context.Background()
	conn, err := sqlDB.Conn(ctx)
	if err != nil {
		return fmt.Errorf("migration lock connection: %w", err)
	}
	defer conn.Close()

	switch dialect {
	case config.Postgres:
		if _, err := conn.ExecContext(ctx, `SELECT pg_advisory_lock($1)`, migrationLockKey); err != nil {
			return fmt.Errorf("take migration lock: %w", err)
		}
		defer conn.ExecContext(ctx, `SELECT pg_advisory_unlock($1)`, migrationLockKey)
	case config.MySQL:
		var got sql.NullInt64
		if err := conn.QueryRowContext(ctx, `SELECT GET_LOCK(?, ?)`, migrationLockName, migrationLockWait).Scan(&got); err != nil {
			return fmt.Errorf("take migration lock: %w", err)
		}
		if got.Int64 != 1 {
			return fmt.Errorf("another migration held the lock for %ds", migrationLockWait)
		}
		defer conn.ExecContext(ctx, `SELECT RELEASE_LOCK(?)`, migrationLockName)
	}
	return fn(db)
}

// backfillLastActivity derives the denormalized conversation columns
// (last_message_at + last_message_preview) for rows that predate them — without
// it, conversations migrated in with existing history would have NULL
// last_message_at and sort by creation time with an empty inbox preview until the
// next message touches them (the queue never reads the messages table). Mirrors
// the maintenance rule (participant-visible, non-system) in domain.SendMessage.
// Idempotent: the NULL/empty guards make it a no-op once a row is filled.
// Portable: the subqueries hit `messages` (not the table being updated) and use
// only SUBSTR/MAX/LIMIT, which behave the same on sqlite, postgres, and mysql.
func backfillLastActivity(db *gorm.DB) error {
	if err := db.Exec(`UPDATE conversations SET last_message_at = (
		SELECT MAX(m.created_at) FROM messages m
		WHERE m.conversation_id = conversations.id
		  AND m.visibility = 'participants' AND m.sender_kind <> 'system')
		WHERE last_message_at IS NULL`).Error; err != nil {
		return err
	}
	return db.Exec(`UPDATE conversations SET last_message_preview = (
		SELECT SUBSTR(m.body, 1, 280) FROM messages m
		WHERE m.conversation_id = conversations.id
		  AND m.visibility = 'participants' AND m.sender_kind <> 'system'
		ORDER BY m.created_at DESC, m.id DESC LIMIT 1)
		WHERE (last_message_preview IS NULL OR last_message_preview = '')
		  AND last_message_at IS NOT NULL`).Error
}

// backfillContactNames fills contacts.name for rows whose profile predates the
// column. Decoded in Go, not SQL: the three dialects spell JSON extraction three
// different ways, and a contact table is small enough to walk.
//
// Only empty names are written, so it is idempotent and never overwrites a name
// a later profile write already materialized.
func backfillContactNames(db *gorm.DB) error {
	var rows []models.ContactMeta
	if err := db.
		Joins("JOIN contacts c ON c.tenant_id = contacts_meta.tenant_id AND c.external_user_id = contacts_meta.external_user_id").
		Where("c.name IS NULL OR c.name = ''").
		Find(&rows).Error; err != nil {
		return err
	}
	for i := range rows {
		var profile struct {
			Name string `json:"name"`
		}
		if json.Unmarshal(rows[i].Metadata, &profile) != nil || profile.Name == "" {
			continue
		}
		if err := db.Model(&models.Contact{}).
			Where("tenant_id = ? AND external_user_id = ?", rows[i].TenantID, rows[i].ExternalUserID).
			Update("name", profile.Name).Error; err != nil {
			return err
		}
	}
	return nil
}

// dropRetiredObjects removes what the profile move replaced. AutoMigrate never
// drops, so a database upgraded in place would keep them forever.
//
// Raw DDL by table NAME: the fields are gone from the models, so gorm's
// model-driven migrator has no schema to resolve them against. Dropping a column
// takes its indexes with it on every dialect.
// backfillRoleAssignments turns the retired per-member role and peer flag into
// the assignment rows that replaced them. Without it an existing member
// authenticates holding nothing and cannot even repair themselves.
func backfillRoleAssignments(db *gorm.DB) error {
	m := db.Migrator()
	if !m.HasColumn("admin_users", "platform_role") {
		return nil
	}
	var rows []struct {
		ID         string
		TenantID   string
		Role       string
		PeerAccess bool
	}
	err := db.Raw(`SELECT id, tenant_id, platform_role AS role, peer_access
	               FROM admin_users
	               WHERE id NOT IN (SELECT admin_user_id FROM role_assignments)`).Scan(&rows).Error
	if err != nil {
		return err
	}
	for _, r := range rows {
		held := []models.RoleAssignment{{
			TenantID: r.TenantID, AdminUserID: r.ID, Role: models.PlatformRole(r.Role),
		}}
		// A translator's grant used to be the rows in translator_scopes, and none of
		// them meant every project and language. The narrowed ones are read back below.
		if held[0].Role == models.PlatformTranslator {
			held[0].Scope.Projects = []string{models.ScopeAll}
			held[0].Scope.Locales = []string{models.ScopeAll}
		}
		// Peer access was the person's, whatever their role; it becomes the agent
		// assignment's dimension, so a non-agent who held it keeps reaching peers.
		if r.PeerAccess && held[0].Role != models.PlatformAgent {
			held = append(held, models.RoleAssignment{
				TenantID: r.TenantID, AdminUserID: r.ID, Role: models.PlatformAgent,
			})
		}
		held[len(held)-1].Scope.Peer = r.PeerAccess
		if err := db.Create(&held).Error; err != nil {
			return err
		}
	}
	return backfillTranslatorScopes(db)
}

// backfillTranslatorScopes folds the retired grant table into the translator
// assignment. Its empty set meant "everything", which is now spelled "all".
func backfillTranslatorScopes(db *gorm.DB) error {
	if !db.Migrator().HasTable("translator_scopes") {
		return nil
	}
	var rows []struct {
		TenantID    string
		AdminUserID string
		Kind        string
		Value       string
	}
	if err := db.Raw("SELECT tenant_id, admin_user_id, kind, value FROM translator_scopes").Scan(&rows).Error; err != nil {
		return err
	}

	byMember := map[[2]string]*models.RoleScope{}
	for _, r := range rows {
		key := [2]string{r.TenantID, r.AdminUserID}
		if byMember[key] == nil {
			byMember[key] = &models.RoleScope{}
		}
		switch r.Kind {
		case "project":
			byMember[key].Projects = append(byMember[key].Projects, r.Value)
		case "locale":
			byMember[key].Locales = append(byMember[key].Locales, r.Value)
		}
	}
	for key, scope := range byMember {
		if len(scope.Projects) == 0 {
			scope.Projects = []string{models.ScopeAll}
		}
		if len(scope.Locales) == 0 {
			scope.Locales = []string{models.ScopeAll}
		}
		err := db.Model(&models.RoleAssignment{}).
			Where("tenant_id = ? AND admin_user_id = ? AND role = ?", key[0], key[1], models.PlatformTranslator).
			Update("scope", scope).Error
		if err != nil {
			return err
		}
	}
	return nil
}

func dropRetiredObjects(db *gorm.DB) error {
	const members = "conversation_members"
	m := db.Migrator()
	for _, col := range []string{"metadata", "member_search_text"} {
		if !m.HasColumn(members, col) {
			continue
		}
		if err := db.Exec("ALTER TABLE " + members + " DROP COLUMN " + col).Error; err != nil {
			return err
		}
	}
	if m.HasTable("push_opt_outs") {
		if err := db.Exec("DROP TABLE push_opt_outs").Error; err != nil {
			return err
		}
	}
	// Retired by role assignments, and only after the backfill above has read them.
	for _, col := range []string{"platform_role", "peer_access"} {
		if !m.HasColumn("admin_users", col) {
			continue
		}
		if err := db.Exec("ALTER TABLE admin_users DROP COLUMN " + col).Error; err != nil {
			return err
		}
	}
	if m.HasTable("translator_scopes") {
		return db.Exec("DROP TABLE translator_scopes").Error
	}
	return nil
}

// applyDialectIndexes adds the search indexes that AutoMigrate can't express:
//   - postgres: pg_trgm extension + GIN(gin_trgm_ops) for partial/substring search
//   - mysql:    FULLTEXT (ngram) — partial-ish, the middle capability tier
//   - sqlite:   none — the portable LIKE path handles search
//
// Failures here are non-fatal for non-search functionality, but we surface them
// so a misconfigured Postgres (missing pg_trgm privileges) is visible.
func applyDialectIndexes(db *gorm.DB) error {
	switch Dialect(db) {
	case config.Postgres:
		stmts := []string{
			`CREATE EXTENSION IF NOT EXISTS pg_trgm`,
			`CREATE INDEX IF NOT EXISTS idx_messages_body_trgm ON messages USING gin (body gin_trgm_ops)`,
			`CREATE INDEX IF NOT EXISTS idx_contact_search_trgm ON contacts USING gin (search_text gin_trgm_ops)`,
		}
		for _, s := range stmts {
			if err := db.Exec(s).Error; err != nil {
				return err
			}
		}
	case config.MySQL:
		// FULLTEXT with the ngram parser approximates substring matching.
		// CREATE FULLTEXT INDEX has no IF NOT EXISTS; guard via catalog check.
		stmts := []struct{ name, table, col string }{
			{"idx_messages_body_ft", "messages", "body"},
			{"idx_contact_search_ft", "contacts", "search_text"},
		}
		for _, s := range stmts {
			var n int64
			db.Raw(
				`SELECT COUNT(*) FROM information_schema.statistics
				 WHERE table_schema = DATABASE() AND table_name = ? AND index_name = ?`,
				s.table, s.name,
			).Scan(&n)
			if n == 0 {
				if err := db.Exec(
					"CREATE FULLTEXT INDEX " + s.name + " ON " + s.table + "(" + s.col + ") WITH PARSER ngram",
				).Error; err != nil {
					return err
				}
			}
		}
	case config.SQLite:
		// No special index; search uses LIKE (portable backend).
	}
	return nil
}
