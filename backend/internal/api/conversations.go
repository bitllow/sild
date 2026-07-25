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

// listConversations: GET /v1/conversations — the one conversation list. The
// credential decides the subset; kind/participant/assignee/q are just filters.
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
		Query:           q,
		Search:          c.Query("q"),
		CallerActorID:   adminIDOf(c),
		IncludeInternal: apiutil.IsAgent(c),
	}
	// Only messenger surfaces render unread counts; the queue shows assignment
	// state and would pay for the extra query.
	if p := middleware.Get(c); p != nil && p.Kind == principal.KindUser {
		in.IncludeUnread, in.UnreadFor = true, p.Subject
	}

	// The badges are independent of the page, so compute them concurrently with
	// the list rather than adding serial round-trips. Buffered so the goroutine
	// never blocks if the list below errors out.
	countsCh := h.queueCounts(c, &q)

	res, err := h.svc.ListConversations(c.Request.Context(), apiutil.Tenant(c), scope, in)
	if err != nil {
		apiutil.Fail(c, err)
		return
	}

	extra := gin.H{}
	if countsCh != nil {
		extra = gin.H{"counts": <-countsCh}
	}
	apiutil.RespondPageWith(c, resourceConversations, store.Page[map[string]any]{
		Items: res.Items, NextCursor: res.NextCursor, HasMore: res.HasMore,
	}, extra)
}

// applyConversationFilters reads the client's narrowing. Two state machines, two
// parameters: `status` is the conversation lifecycle, `assignment_status` the
// assignment's.
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

	// waiting_since comes from the assignment, which peer rows lack, and a keyset
	// over a NULL-bearing key is undefined. The client picks it, so it is a 400.
	if q.Sort == store.SortWaitingSince && (q.Kind == nil || *q.Kind != models.KindSupport) {
		httpx.BadRequest(c, "sort=waiting_since requires kind=support")
		return false
	}

	// A search page is produced by the search query, which cannot express these.
	// Applying them afterwards would thin the page and make has_more lie, so the
	// combination is refused rather than answered wrongly.
	if c.Query("q") != "" {
		switch {
		case q.AssignmentStatus != nil:
			httpx.BadRequest(c, "q cannot be combined with assignment_status")
			return false
		case q.Participant != "":
			httpx.BadRequest(c, "q cannot be combined with participant")
			return false
		case q.Unassigned:
			httpx.BadRequest(c, "q cannot be combined with assignee=none")
			return false
		}
	}
	return true
}

// queueCounts starts the inbox scope counters, support queue only, and returns a
// channel to read them from — nil when they do not apply.
func (h *Handler) queueCounts(c *gin.Context, q *store.ConversationQuery) <-chan gin.H {
	p := middleware.Get(c)
	if p == nil || p.Kind != principal.KindAdmin {
		return nil
	}
	if q.Kind == nil || *q.Kind != models.KindSupport {
		return nil
	}
	ctx, tenant, actor := c.Request.Context(), apiutil.Tenant(c), p.AdminID
	ch := make(chan gin.H, 1)
	go func() {
		open, _ := h.svc.CountOpenConversations(ctx, tenant)
		counts, _ := h.svc.CountQueue(ctx, tenant, actor)
		ch <- gin.H{
			"open":       open,
			"you":        counts.You,
			"unassigned": counts.Unassigned,
			"closed":     counts.Closed,
		}
	}()
	return ch
}

// conversationPageDefaults picks the paging contract. A search page is paged by
// the search backend on conversation id, so the client does not choose the sort.
func conversationPageDefaults(q string) apiutil.PageDefaults {
	if q != "" {
		return apiutil.PageDefaults{
			Resource:   resourceConversations,
			Limit:      30,
			Sort:       store.SortID,
			Order:      store.OrderDesc,
			FixedOrder: true, // the search backend queries newest-first only
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
