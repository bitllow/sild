package domain

import (
	"context"

	"github.com/bitllow/sild/backend/internal/search"
	"github.com/bitllow/sild/backend/internal/store"
)

// SearchService runs the admin mixed-token search (§4.3). It tokenizes the raw
// bar string into structured filters + free keywords and dispatches to the
// dialect-appropriate search.Backend (trigram on Postgres, LIKE elsewhere).
type SearchService struct {
	store   store.Store
	backend search.Backend
}

// NewSearch constructs the search service. dig provides it.
func NewSearch(st store.Store, backend search.Backend) *SearchService {
	return &SearchService{store: st, backend: backend}
}

// Search executes a query against hot data only (§4.3). callerActorID resolves
// the assignee:me shortcut.
func (s *SearchService) Search(ctx context.Context, tenantID, rawQuery, callerActorID, before string, limit int) (search.Results, error) {
	if limit <= 0 || limit > 100 {
		limit = 25
	}
	q := search.Parse(rawQuery)
	q.ResolveAssignee(callerActorID)
	q.Before = before
	q.Limit = limit
	return s.backend.Search(ctx, tenantID, q)
}
