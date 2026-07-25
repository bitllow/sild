package api

import (
	"github.com/bitllow/sild/backend/internal/apiutil"
	"github.com/bitllow/sild/backend/internal/domain"
	"github.com/bitllow/sild/backend/internal/httpx"
	"github.com/bitllow/sild/backend/internal/middleware"
	"github.com/bitllow/sild/backend/internal/policy"
	"github.com/bitllow/sild/backend/internal/principal"
	"github.com/bitllow/sild/backend/internal/store"
	"github.com/bitllow/sild/backend/internal/store/models"
	"github.com/gin-gonic/gin"
)

// listConversations: GET /v1/conversations — the one conversation list.
//
// It replaces GET /me/conversations, /admin/assignments,
// /admin/contacts/conversations, /admin/peer-conversations and /admin/search.
// Those were the same query with different hard-coded filters and five different
// response shapes; the credential now decides the subset, not the URL.
func (h *Handler) listConversations(c *gin.Context) {
	scope := apiutil.Scope(c, policy.ConversationsList)
	if scope.DenyAll() {
		apiutil.RespondPage(c, resourceConversations, store.Page[map[string]any]{})
		return
	}

	page, ok := apiutil.PageParams(c, conversationPageDefaults(c.Query("q")))
	if !ok {
		return
	}

	q := store.ConversationQuery{PageParams: page}
	if !applyConversationFilters(c, &q) {
		return
	}

	in := domain.ListConversationsInput{
		Query:         q,
		Search:        c.Query("q"),
		CallerActorID: adminIDOf(c),
	}
	// Unread counts are what a messenger surface renders; the operator queue
	// shows assignment state instead and would pay for the extra query.
	if p := middleware.Get(c); p != nil && p.Kind == principal.KindUser {
		in.IncludeUnread, in.UnreadFor = true, p.Subject
	}

	res, err := h.svc.ListConversations(c.Request.Context(), apiutil.Tenant(c), scope, in)
	if err != nil {
		apiutil.Fail(c, err)
		return
	}

	extra := gin.H{}
	if counts, ok := h.queueCounts(c, &q); ok {
		extra = counts
	}
	apiutil.RespondPageWith(c, resourceConversations, store.Page[map[string]any]{
		Items: res.Items, NextCursor: res.NextCursor, HasMore: res.HasMore,
	}, extra)
}

// applyConversationFilters reads the client's narrowing. Two distinct state
// machines get two distinct parameters: `status` is the CONVERSATION lifecycle
// (open|closed) and `assignment_status` is the assignment's
// (queued→assigned→closed). The inbox's notion of "closed" is the conversation
// being closed, which is why the old exclude_closed flag maps to status=open.
func applyConversationFilters(c *gin.Context, q *store.ConversationQuery) bool {
	if raw := c.Query("kind"); raw != "" {
		k := models.ConversationKind(raw)
		if k != models.KindSupport && k != models.KindPeer {
			httpx.BadRequest(c, "kind must be support or peer")
			return false
		}
		q.Kind = &k
	}
	if raw := c.Query("status"); raw != "" {
		s := models.ConversationStatus(raw)
		if s != models.ConversationOpen && s != models.ConversationClosed {
			httpx.BadRequest(c, "status must be open or closed")
			return false
		}
		q.Status = &s
	}
	if raw := c.Query("assignment_status"); raw != "" {
		s := models.AssignmentStatus(raw)
		switch s {
		case models.AssignmentQueued, models.AssignmentAssigned, models.AssignmentClosed:
			q.AssignmentStatus = &s
		default:
			httpx.BadRequest(c, "assignment_status must be queued, assigned or closed")
			return false
		}
	}
	if raw := c.Query("assignee"); raw != "" {
		switch raw {
		case "none":
			q.Unassigned = true
		case "me":
			q.AssigneeActorID = adminIDOf(c)
		default:
			q.AssigneeActorID = raw
		}
	}
	if raw := c.Query("participant"); raw != "" {
		if raw == "me" {
			raw = apiutil.Subject(c)
		}
		q.Participant = raw
	}
	q.ConvRole = c.Query("role")

	// waiting_since comes from the assignment, which peer and participant-scoped
	// rows do not have — a keyset over a NULL-bearing key is undefined. The
	// framework treats an unsupported sort as a programming error, but here the
	// client picks it, so it is a request error.
	if q.Sort == store.SortWaitingSince && (q.Kind == nil || *q.Kind != models.KindSupport) {
		httpx.BadRequest(c, "sort=waiting_since requires kind=support")
		return false
	}
	return true
}

// queueCounts returns the inbox scope counters, and only for the support queue —
// they are meaningless for a peer list or a user's own conversations.
func (h *Handler) queueCounts(c *gin.Context, q *store.ConversationQuery) (gin.H, bool) {
	p := middleware.Get(c)
	if p == nil || p.Kind != principal.KindAdmin {
		return nil, false
	}
	if q.Kind == nil || *q.Kind != models.KindSupport {
		return nil, false
	}
	ctx, tenant := c.Request.Context(), apiutil.Tenant(c)
	open, _ := h.svc.CountOpenConversations(ctx, tenant)
	counts, _ := h.svc.CountQueue(ctx, tenant, p.AdminID)
	return gin.H{"counts": gin.H{
		"open":       open,
		"you":        counts.You,
		"unassigned": counts.Unassigned,
		"closed":     counts.Closed,
	}}, true
}

// conversationPageDefaults picks the paging contract. A search page is ordered
// and paged by the SEARCH backend (keyset on conversation id), so the client does
// not choose the sort — and the cursor is minted and validated as an id cursor,
// which is what keeps a search cursor from being replayed against the ordinary
// list.
func conversationPageDefaults(q string) apiutil.PageDefaults {
	if q != "" {
		return apiutil.PageDefaults{
			Resource: resourceConversations,
			Limit:    30,
			Sort:     store.SortID,
			Order:    store.OrderDesc,
		}
	}
	return apiutil.PageDefaults{
		Resource: resourceConversations,
		Limit:    30,
		Sort:     store.SortLastActivity,
		Order:    store.OrderDesc,
		Sorts:    []store.SortKey{store.SortCreated, store.SortWaitingSince},
	}
}

func adminIDOf(c *gin.Context) string {
	if p := middleware.Get(c); p != nil {
		return p.AdminID
	}
	return ""
}
