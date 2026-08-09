package domain

import (
	"context"

	"github.com/bitllow/sild/backend/internal/policy"
	"github.com/bitllow/sild/backend/internal/search"
	"github.com/bitllow/sild/backend/internal/store"
	"github.com/bitllow/sild/backend/internal/store/models"
	"github.com/bitllow/sild/backend/internal/views"
)

// ListConversationsInput is one list request, backing the support queue, the peer
// inbox, a user's own conversations, a contact's history and search.
type ListConversationsInput struct {
	Query store.ConversationQuery
	// Search, when non-empty, narrows the page to conversations matching the
	// mixed-token bar string. It runs through the same scope, so it cannot widen
	// what the list would otherwise return.
	Search string
	// CallerActorID resolves the assignee:me shortcut inside a search bar.
	CallerActorID string
	// IncludeUnread computes per-conversation unread counts (the messenger
	// surfaces need them; the operator queue does not).
	IncludeUnread bool
	// UnreadFor is the participant unread counts are computed for.
	UnreadFor string
	// IncludeInternal permits internal notes to match a search (§5.6).
	IncludeInternal bool
	// WithProfiles reads the profile blobs too, for `expand=contacts.metadata`.
	// Left false the page never touches the blob table.
	WithProfiles bool
}

// ConversationPage is a rendered page plus the paging position. Participants,
// Names and Profiles are the distinct external people on the page and what was
// loaded for them, so an expansion re-reads nothing. Profiles is nil unless the
// request asked for the blob.
type ConversationPage struct {
	Items        []map[string]any
	Participants []string
	Names        map[string]string
	Profiles     map[string][]byte
	NextCursor   *store.Cursor
	HasMore      bool
}

// ListConversations returns one page of conversations visible to the scope.
// Search narrows the same query rather than forking a second one.
func (s *Service) ListConversations(ctx context.Context, tenantID string, scope policy.ResourceScope, in ListConversationsInput) (ConversationPage, error) {
	if scope.DenyAll() {
		return ConversationPage{Items: []map[string]any{}}, nil
	}

	q := in.Query
	q.TenantID = tenantID

	if in.Search != "" {
		return s.searchConversations(ctx, tenantID, scope, in, q)
	}

	page, err := s.store.Conversations().List(ctx, scope, q)
	if err != nil {
		return ConversationPage{}, err
	}

	return s.renderPage(ctx, tenantID, page.Items, in, nil, page.NextCursor, page.HasMore)
}

// rowExtras holds the per-page lookups a row needs, batched. Resolving these per
// row cost one query each — 30 rows meant up to 120 extra queries per page.
type rowExtras struct {
	subjects   map[string]string
	agentNames map[string]string
	unread     map[string]int
	snippets   map[string]search.ConversationHit
	profiles   views.Profiles
}

// renderPage batches every per-row lookup, then renders.
func (s *Service) renderPage(ctx context.Context, tenantID string, items []store.ConversationItem, in ListConversationsInput, snippets map[string]search.ConversationHit, next *store.Cursor, hasMore bool) (ConversationPage, error) {
	out := ConversationPage{
		Items:      make([]map[string]any, 0, len(items)),
		NextCursor: next,
		HasMore:    hasMore,
	}
	if len(items) == 0 {
		return out, nil
	}

	ids := make([]string, 0, len(items))
	actors := make([]string, 0, len(items))
	var members []models.ConversationMember
	for i := range items {
		ids = append(ids, items[i].Conversation.ID)
		if a := items[i].Assignment; a != nil && a.AssigneeActorID != nil {
			actors = append(actors, *a.AssigneeActorID)
		}
		members = append(members, items[i].Members...)
	}

	// Assignees resolve in the same pass as agent members: an operator who is both
	// would otherwise be read twice.
	load := s.MemberNames
	if in.WithProfiles {
		load = s.MemberProfiles
	}
	profiles, err := load(ctx, tenantID, members, actors...)
	if err != nil {
		return ConversationPage{}, err
	}
	out.Participants = ExternalParticipants(members)
	out.Names = profiles.Names
	out.Profiles = profiles.Contacts

	x := rowExtras{snippets: snippets, profiles: profiles, agentNames: profiles.Agents}
	x.subjects, _ = s.store.Email().Subjects(ctx, tenantID, ids)
	if in.IncludeUnread && in.UnreadFor != "" {
		x.unread, _ = s.store.Messages().UnreadCounts(ctx, tenantID, ids, in.UnreadFor)
	}

	for i := range items {
		out.Items = append(out.Items, s.renderRow(&items[i], x))
	}
	return out, nil
}

// agentNames resolves distinct actor ids in one pass.
func (s *Service) agentNames(ctx context.Context, tenantID string, actorIDs []string) map[string]string {
	out := make(map[string]string, len(actorIDs))
	for _, id := range actorIDs {
		if _, done := out[id]; done {
			continue
		}
		out[id] = s.AgentDisplayName(ctx, tenantID, id)
	}
	return out
}

// searchConversations pages the SEARCH, then hydrates that page: the backend is
// keyset-pageable on conversation id, so it owns the paging. Paginating a fixed
// slice of matches instead would cap the result set and lie about has_more.
func (s *Service) searchConversations(ctx context.Context, tenantID string, scope policy.ResourceScope, in ListConversationsInput, q store.ConversationQuery) (ConversationPage, error) {
	limit := store.ClampLimit(q.Limit, 30)
	before := ""
	if q.Cursor != nil {
		before = q.Cursor.ID
	}

	res, err := s.search.Search(ctx, tenantID, scope, SearchInput{
		Query: in.Search, CallerActorID: in.CallerActorID, Kind: q.Kind,
		Status: q.Status, Assignee: q.AssigneeActorID, ConvRole: q.ConvRole,
		IncludeInternal: in.IncludeInternal,
		Before:          before, Limit: limit + 1, // +1 probes for another page
	})
	if err != nil {
		return ConversationPage{}, err
	}

	hits := res.Conversations
	out := ConversationPage{Items: []map[string]any{}}
	if len(hits) > limit {
		out.HasMore = true
		hits = hits[:limit]
	}
	if len(hits) == 0 {
		return out, nil
	}

	snippets := make(map[string]search.ConversationHit, len(hits))
	ids := make([]string, 0, len(hits))
	for _, hit := range hits {
		ids = append(ids, hit.ConversationID)
		snippets[hit.ConversationID] = hit
	}

	// Hydrate exactly this page: the ids are already the page, so the list query
	// must not paginate again.
	hydrate := q
	hydrate.IDs = ids
	hydrate.Cursor = nil
	hydrate.Limit = len(ids)
	page, err := s.store.Conversations().List(ctx, scope, hydrate)
	if err != nil {
		return ConversationPage{}, err
	}

	byID := make(map[string]*store.ConversationItem, len(page.Items))
	for i := range page.Items {
		byID[page.Items[i].Conversation.ID] = &page.Items[i]
	}
	// Keep the search's ordering (id DESC, the key it pages on) rather than the
	// list's.
	ordered := make([]store.ConversationItem, 0, len(ids))
	for _, id := range ids {
		if it, ok := byID[id]; ok { // scope may drop one between the two queries
			ordered = append(ordered, *it)
		}
	}

	var next *store.Cursor
	if out.HasMore {
		next = &store.Cursor{Key: store.SortID, Order: store.OrderDesc, ID: ids[len(ids)-1]}
	}
	return s.renderPage(ctx, tenantID, ordered, in, snippets, next, out.HasMore)
}

// renderRow builds one list row through the shared view builders, so the queue,
// the widget and search all render the same shape.
func (s *Service) renderRow(it *store.ConversationItem, x rowExtras) map[string]any {
	id := it.Conversation.ID
	conv := views.Conversation(&it.Conversation, it.Members, it.Assignment, x.profiles)
	conv["kind"] = it.Conversation.Kind
	conv["last_activity"] = it.LastActivity
	if it.Conversation.LastMessagePreview != "" {
		conv["last_message"] = map[string]any{
			"body":       it.Conversation.LastMessagePreview,
			"created_at": it.Conversation.LastMessageAt,
		}
	}
	if subject := x.subjects[id]; subject != "" {
		conv["subject"] = subject
	}

	// The handling agent's name, so a messenger surface labels the row with a
	// real person instead of "Support".
	if a := it.Assignment; a != nil && a.AssigneeActorID != nil {
		if name := x.agentNames[*a.AssigneeActorID]; name != "" {
			conv["agent_name"] = name
		}
	}

	// A search hit previews the MATCHING fragment: a match in an older message
	// would otherwise render with an unrelated newest-message preview.
	if hit, ok := x.snippets[id]; ok && hit.Snippet != "" {
		conv["snippet"] = hit.Snippet
	}

	if x.unread != nil {
		conv["unread_count"] = x.unread[id]
	}
	return conv
}

// ListContacts returns one page of the tenant's contacts, scoped.
func (s *Service) ListContacts(ctx context.Context, tenantID string, scope policy.ResourceScope, q store.ContactQuery) (store.Page[store.Contact], error) {
	return s.store.Contacts().ListContacts(ctx, tenantID, scope, q)
}

// GetContact returns one contact.
func (s *Service) GetContact(ctx context.Context, tenantID string, scope policy.ResourceScope, externalUserID string) (*store.Contact, error) {
	c, err := s.store.Contacts().GetContact(ctx, tenantID, scope, externalUserID)
	if err != nil {
		return nil, mapStoreErr(err)
	}
	return c, nil
}
