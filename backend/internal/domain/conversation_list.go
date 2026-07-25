package domain

import (
	"context"

	"github.com/bitllow/sild/backend/internal/policy"
	"github.com/bitllow/sild/backend/internal/search"
	"github.com/bitllow/sild/backend/internal/store"
	"github.com/bitllow/sild/backend/internal/store/models"
	"github.com/bitllow/sild/backend/internal/views"
)

// ListConversationsInput is one unified list request. It backs every surface
// that used to have its own endpoint: the support queue, the peer inbox, a
// user's own conversations, a contact's history, and search.
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
}

// ConversationPage is a rendered page plus the paging position.
type ConversationPage struct {
	Items      []map[string]any
	NextCursor *store.Cursor
	HasMore    bool
}

// ListConversations returns one page of conversations visible to the scope.
//
// Search is layered onto the same query rather than forking a second one: the
// search backend returns matching ids, which narrow the list. That keeps
// filtering, ordering, scope and pagination in one place — the previous split
// meant search results were unpaginated and shaped differently from the queue.
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

	out := ConversationPage{
		Items:      make([]map[string]any, 0, len(page.Items)),
		NextCursor: page.NextCursor,
		HasMore:    page.HasMore,
	}
	for i := range page.Items {
		out.Items = append(out.Items, s.renderRow(ctx, tenantID, &page.Items[i], in, nil))
	}
	return out, nil
}

// searchConversations pages the SEARCH, then hydrates that page.
//
// The search backend is keyset-pageable on conversation id, so it owns the
// paging; the list query only turns the page's ids into rows. Taking a fixed
// slice of matches and paginating THAT would cap results at the slice size and
// then report has_more:false — the silent truncation this endpoint exists to
// remove.
func (s *Service) searchConversations(ctx context.Context, tenantID string, scope policy.ResourceScope, in ListConversationsInput, q store.ConversationQuery) (ConversationPage, error) {
	limit := store.ClampLimit(q.Limit, 30)
	before := ""
	if q.Cursor != nil {
		before = q.Cursor.ID
	}

	res, err := s.search.Search(ctx, tenantID, scope, SearchInput{
		Query: in.Search, CallerActorID: in.CallerActorID, Kind: q.Kind,
		Before: before, Limit: limit + 1, // +1 probes for another page
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
	// Preserve the search's ranking rather than the list's ordering.
	for _, id := range ids {
		it, ok := byID[id]
		if !ok {
			continue // scope dropped it between the two queries
		}
		out.Items = append(out.Items, s.renderRow(ctx, tenantID, it, in, snippets))
	}
	if out.HasMore {
		out.NextCursor = &store.Cursor{
			Key: store.SortID, Order: store.OrderDesc, ID: ids[len(ids)-1],
		}
	}
	return out, nil
}

// renderRow builds one list row through the shared view builders, so the queue,
// the widget and search all render the same shape.
func (s *Service) renderRow(ctx context.Context, tenantID string, it *store.ConversationItem, in ListConversationsInput, snippets map[string]search.ConversationHit) map[string]any {
	conv := views.Conversation(&it.Conversation, it.Members, it.Assignment)
	conv["kind"] = it.Conversation.Kind
	conv["last_activity"] = it.LastActivity
	if it.Conversation.LastMessagePreview != "" {
		conv["last_message"] = map[string]any{
			"body":       it.Conversation.LastMessagePreview,
			"created_at": it.Conversation.LastMessageAt,
		}
	}
	if subject := s.EmailSubject(ctx, tenantID, it.Conversation.ID); subject != "" {
		conv["subject"] = subject
	}

	// The handling agent's display name, so a messenger surface can label the
	// row with a real person instead of "Support".
	if it.Assignment != nil && it.Assignment.AssigneeActorID != nil {
		if name := s.AgentDisplayName(ctx, tenantID, *it.Assignment.AssigneeActorID); name != "" {
			conv["agent_name"] = name
		}
	}

	// Search annotations describe why THIS row matched. Without them a hit on an
	// older message renders with the newest message as its preview and the
	// operator cannot see the connection.
	if hit, ok := snippets[it.Conversation.ID]; ok {
		if hit.Snippet != "" {
			conv["snippet"] = hit.Snippet
		}
		conv["matched_fields"] = hit.MatchedFields
	}

	if in.IncludeUnread && in.UnreadFor != "" {
		conv["unread_count"] = s.unreadFor(ctx, tenantID, it.Conversation.ID, in.UnreadFor)
	}
	return conv
}

func (s *Service) unreadFor(ctx context.Context, tenantID, convID, externalUserID string) int {
	const includeInternal = false
	uid := externalUserID
	var lastReadID string
	if rr, err := s.store.Receipts().Get(ctx, tenantID, convID, store.Participant{
		Kind: models.MemberUser, ExternalUserID: &uid,
	}); err == nil {
		lastReadID = rr.LastReadMessageID
	}
	n, err := s.store.Messages().UnreadCount(ctx, tenantID, convID, lastReadID, includeInternal)
	if err != nil {
		return 0
	}
	return n
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
