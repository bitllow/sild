package search

import (
	"context"

	"github.com/bitllow/sild/backend/internal/config"
	"gorm.io/gorm"
)

// postgresBackend implements partial/substring search with pg_trgm: ILIKE for
// matching (uses the GIN trigram indexes) plus similarity ranking (§4.3). For
// now it ranks by recency via the shared collector; trigram similarity ordering
// is layered in the realtime/search buildout.
type postgresBackend struct{ db *gorm.DB }

func (b *postgresBackend) Search(ctx context.Context, tenantID string, q Query) (Results, error) {
	filtered := buildFilters(b.db.WithContext(ctx), tenantID, q, config.Postgres)
	return collectHits(ctx, b.db, filtered, q, config.Postgres)
}
