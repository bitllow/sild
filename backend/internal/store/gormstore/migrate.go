package gormstore

import (
	"github.com/bitllow/sild/backend/internal/config"
	"github.com/bitllow/sild/backend/internal/store/models"
	"gorm.io/gorm"
)

// Migrate builds the schema on any dialect via AutoMigrate, then applies the
// dialect-specific search indexes (ARCHITECTURE §4). Idempotent.
func Migrate(db *gorm.DB) error {
	if err := db.AutoMigrate(models.All()...); err != nil {
		return err
	}
	return applyDialectIndexes(db)
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
			`CREATE INDEX IF NOT EXISTS idx_member_search_trgm ON conversation_members USING gin (member_search_text gin_trgm_ops)`,
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
			{"idx_member_search_ft", "conversation_members", "member_search_text"},
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
