package api_test

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/bitllow/sild/backend/internal/store/models"
	"github.com/bitllow/sild/backend/internal/testutil"
)

type envelope struct {
	Items      []map[string]any `json:"items"`
	NextCursor *string          `json:"next_cursor"`
	HasMore    bool             `json:"has_more"`
}

func getPage(t *testing.T, h *testutil.Harness, path, tok string) envelope {
	t.Helper()
	w := h.Request("GET", path).Bearer(tok).Do()
	if w.Code != http.StatusOK {
		t.Fatalf("GET %s: %d %s", path, w.Code, w.Body)
	}
	var e envelope
	testutil.DecodeJSON(t, w, &e)
	return e
}

// Every collection returns {items, next_cursor, has_more}, and next_cursor is
// null exactly when has_more is false. One list parser, no exceptions.
func TestListEnvelopeIsUniform(t *testing.T) {
	h := testutil.New(t)
	tenant := h.SeedTenant()
	tok := h.MintToken(tenant.ID, "u_client")
	h.SeedAdmin(tenant.ID, "owner@test", models.PlatformOwner)
	owner := loginAs(t, h, "owner@test")

	var conv struct {
		ID string `json:"id"`
	}
	testutil.DecodeJSON(t, h.Request("POST", "/v1/conversations").Bearer(tok).JSON(map[string]any{}).Do(), &conv)
	h.Request("POST", "/v1/conversations/"+conv.ID+"/messages").Bearer(tok).JSON(map[string]any{"body": "hi"}).Do()

	cases := []struct {
		path   string
		cookie bool
	}{
		{"/v1/conversations", false},
		{"/v1/conversations/" + conv.ID + "/messages", false},
		{"/v1/contacts", true},
		{"/v1/team", true},
		{"/v1/api-keys", true},
		{"/v1/webhooks", true},
	}
	for _, tc := range cases {
		t.Run(tc.path, func(t *testing.T) {
			var e envelope
			var w = h.Request("GET", tc.path)
			if tc.cookie {
				w = w.Cookie("sild_admin", owner)
			} else {
				w = w.Bearer(tok)
			}
			res := w.Do()
			if res.Code != http.StatusOK {
				t.Fatalf("GET %s: %d %s", tc.path, res.Code, res.Body)
			}
			testutil.DecodeJSON(t, res, &e)
			if e.Items == nil {
				t.Fatal("items must be present (empty array, never null)")
			}
			if e.HasMore != (e.NextCursor != nil) {
				t.Fatalf("next_cursor must be null exactly when has_more is false: has_more=%v cursor=%v",
					e.HasMore, e.NextCursor)
			}
		})
	}
}

// Paging to exhaustion returns every row exactly once — no duplicates, no gaps.
func TestCursorRoundTrip(t *testing.T) {
	h := testutil.New(t)
	tenant := h.SeedTenant()
	tok := h.MintToken(tenant.ID, "u_client")

	const total = 7
	for i := 0; i < total; i++ {
		var conv struct {
			ID string `json:"id"`
		}
		w := h.Request("POST", "/v1/conversations").Bearer(tok).JSON(map[string]any{}).Do()
		testutil.DecodeJSON(t, w, &conv)
		h.Request("POST", "/v1/conversations/"+conv.ID+"/messages").
			Bearer(tok).JSON(map[string]any{"body": fmt.Sprintf("m%d", i)}).Do()
	}

	seen := map[string]int{}
	path := "/v1/conversations?limit=2"
	for pages := 0; ; pages++ {
		if pages > total+2 {
			t.Fatal("pagination did not terminate")
		}
		e := getPage(t, h, path, tok)
		for _, it := range e.Items {
			id, _ := it["id"].(string)
			seen[id]++
		}
		if !e.HasMore {
			break
		}
		path = "/v1/conversations?limit=2&cursor=" + *e.NextCursor
	}

	if len(seen) != total {
		t.Fatalf("paged %d distinct conversations, want %d", len(seen), total)
	}
	for id, n := range seen {
		if n != 1 {
			t.Fatalf("conversation %s returned %d times", id, n)
		}
	}
}

// A cursor is bound to the ordering it was minted under. Replaying one under a
// reversed direction flips the comparison operator, so the query would return
// the rows BEFORE the position — a wrong answer with a 200.
func TestCursorRejectsChangedOrdering(t *testing.T) {
	h := testutil.New(t)
	tenant := h.SeedTenant()
	tok := h.MintToken(tenant.ID, "u_client")
	for i := 0; i < 3; i++ {
		h.Request("POST", "/v1/conversations").Bearer(tok).JSON(map[string]any{}).Do()
	}

	e := getPage(t, h, "/v1/conversations?limit=1", tok)
	if e.NextCursor == nil {
		t.Fatal("expected a cursor")
	}
	w := h.Request("GET", "/v1/conversations?limit=1&order=asc&cursor="+*e.NextCursor).Bearer(tok).Do()
	if w.Code != http.StatusBadRequest {
		t.Fatalf("cursor replayed under a different order: %d %s, want 400", w.Code, w.Body)
	}
}

// A cursor is bound to its filter set and parent, so it cannot be carried to a
// different query. Message ids are comparable ULIDs across conversations, so an
// unbound cursor would silently return a wrong-but-plausible page.
func TestCursorRejectsChangedFilters(t *testing.T) {
	h := testutil.New(t)
	tenant := h.SeedTenant()
	tok := h.MintToken(tenant.ID, "u_client")
	for i := 0; i < 3; i++ {
		h.Request("POST", "/v1/conversations").Bearer(tok).JSON(map[string]any{}).Do()
	}

	e := getPage(t, h, "/v1/conversations?limit=1", tok)
	if e.NextCursor == nil {
		t.Fatal("expected a cursor")
	}
	w := h.Request("GET", "/v1/conversations?limit=1&kind=support&cursor="+*e.NextCursor).Bearer(tok).Do()
	if w.Code != http.StatusBadRequest {
		t.Fatalf("cursor replayed with a different filter set: %d %s, want 400", w.Code, w.Body)
	}
}

// Limits clamp rather than error; a malformed cursor is a 400.
func TestLimitClampsAndBadCursorRejected(t *testing.T) {
	h := testutil.New(t)
	tenant := h.SeedTenant()
	tok := h.MintToken(tenant.ID, "u_client")

	for _, limit := range []string{"0", "-1", "99999"} {
		w := h.Request("GET", "/v1/conversations?limit="+limit).Bearer(tok).Do()
		if w.Code != http.StatusOK {
			t.Fatalf("limit=%s should clamp, got %d %s", limit, w.Code, w.Body)
		}
	}
	if w := h.Request("GET", "/v1/conversations?cursor=garbage").Bearer(tok).Do(); w.Code != http.StatusBadRequest {
		t.Fatalf("malformed cursor should be 400, got %d", w.Code)
	}
}

// sort=waiting_since comes from the assignment, which peer and
// participant-scoped rows do not have; a keyset over a NULL-bearing key is
// undefined. The client picks the key, so this is a request error.
func TestWaitingSinceRequiresSupportKind(t *testing.T) {
	h := testutil.New(t)
	tenant := h.SeedTenant()
	tok := h.MintToken(tenant.ID, "u_client")

	if w := h.Request("GET", "/v1/conversations?sort=waiting_since").Bearer(tok).Do(); w.Code != http.StatusBadRequest {
		t.Fatalf("waiting_since without kind=support should be 400, got %d %s", w.Code, w.Body)
	}
	if w := h.Request("GET", "/v1/conversations?sort=waiting_since&kind=support").Bearer(tok).Do(); w.Code != http.StatusOK {
		t.Fatalf("waiting_since with kind=support: %d %s", w.Code, w.Body)
	}
}
