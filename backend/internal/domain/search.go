package domain

import (
	"context"

	"github.com/bitllow/sild/backend/internal/policy"
	"github.com/bitllow/sild/backend/internal/search"
	"github.com/bitllow/sild/backend/internal/store"
	"github.com/bitllow/sild/backend/internal/store/models"
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

// maxSearchLimit bounds one search page. It exceeds the API's 100-row ceiling by
// one so a has-more probe at the maximum page size still fits.
const maxSearchLimit = 101

// SearchInput is one search request. Kind must already have been intersected
// with the caller's scope.
type SearchInput struct {
	Query         string
	CallerActorID string
	Before        string
	Limit         int
	Kind          *models.ConversationKind
	// Structured filters the backend can express in SQL. They must be applied
	// inside the search query: applying them afterwards, to an already-limited
	// page of ids, yields sparse pages and a lying has_more.
	Status   *models.ConversationStatus
	Assignee string
	ConvRole string
}

// Search executes a query against hot data only (§4.3). The policy scope decides
// which conversation kinds are visible — search does not make that call itself,
// which is what stops a new list path from forgetting the peer/support boundary.
func (s *SearchService) Search(ctx context.Context, tenantID string, scope policy.ResourceScope, in SearchInput) (search.Results, error) {
	if scope.DenyAll() {
		return search.Results{}, nil
	}
	// Clamp rather than reset: a caller asking for limit+1 as a has-more probe
	// would otherwise silently drop to 25 at the boundary.
	limit := in.Limit
	if limit <= 0 {
		limit = 25
	}
	if limit > maxSearchLimit {
		limit = maxSearchLimit
	}
	q := search.Parse(in.Query)
	q.ResolveAssignee(in.CallerActorID)
	if in.Status != nil {
		v := string(*in.Status)
		q.Status = &v
	}
	if in.Assignee != "" {
		v := in.Assignee
		q.Assignee = &v
	}
	if in.ConvRole != "" {
		v := in.ConvRole
		q.Role = &v
	}
	q.Kinds = searchKinds(scope, in.Kind)
	// Only a peer-scoped search widens to raw metadata; support search stays
	// bound to the tenant's searchable_metadata_keys allowlist.
	q.MatchRawMetadata = len(q.Kinds) == 1 && q.Kinds[0] == models.KindPeer
	q.Before = in.Before
	q.Limit = limit
	return s.backend.Search(ctx, tenantID, q)
}

// searchKinds intersects the requested kind with the scope ceiling. Asking for a
// kind the scope excludes yields an impossible filter, never a widened one.
func searchKinds(scope policy.ResourceScope, want *models.ConversationKind) []models.ConversationKind {
	if want == nil {
		return scope.AllowedKinds()
	}
	if !scope.AllowsKind(*want) {
		return []models.ConversationKind{""} // matches nothing
	}
	return []models.ConversationKind{*want}
}
