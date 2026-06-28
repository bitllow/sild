package search

import (
	"context"

	"gorm.io/gorm"
)

// portableBackend implements search with LOWER(col) LIKE — works on every
// dialect, no ranking. The accepted capability tier for MySQL/SQLite (§4.3).
type portableBackend struct{ db *gorm.DB }

func (b *portableBackend) Search(ctx context.Context, tenantID string, q Query) (Results, error) {
	filtered := buildFilters(b.db.WithContext(ctx), tenantID, q, "LIKE")
	return collectHits(ctx, b.db, filtered, q, "LIKE")
}
