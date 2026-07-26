package api_test

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/bitllow/sild/backend/internal/store/models"
	"github.com/bitllow/sild/backend/internal/testutil"
)

// A contact's sort value is an aggregate compared back through a HAVING clause,
// which SQLite evaluates as a string — so this is timezone-sensitive.
func TestContactsPageToExhaustion(t *testing.T) {
	h := testutil.New(t)
	tenant := h.SeedTenant()
	h.SeedAdmin(tenant.ID, "owner@test", models.PlatformOwner)
	owner := loginAs(t, h, "owner@test")

	const total = 12
	for i := range total {
		tok := h.MintToken(tenant.ID, fmt.Sprintf("u_%02d", i))
		var conv struct {
			ID string `json:"id"`
		}
		testutil.DecodeJSON(t, h.Request("POST", "/v1/conversations").Bearer(tok).JSON(map[string]any{}).Do(), &conv)
		h.Request("POST", "/v1/conversations/"+conv.ID+"/messages").
			Bearer(tok).JSON(map[string]any{"body": fmt.Sprintf("m%d", i)}).Do()
	}

	seen := map[string]int{}
	path := "/v1/contacts?limit=5"
	for pages := 0; ; pages++ {
		if pages > total {
			t.Fatal("contacts pagination did not terminate")
		}
		w := h.Request("GET", path).Cookie("sild_admin", owner).Do()
		if w.Code != http.StatusOK {
			t.Fatalf("GET %s: %d %s", path, w.Code, w.Body)
		}
		var e envelope
		testutil.DecodeJSON(t, w, &e)
		for _, it := range e.Items {
			id, _ := it["external_user_id"].(string)
			seen[id]++
		}
		if !e.HasMore {
			break
		}
		path = "/v1/contacts?limit=5&cursor=" + *e.NextCursor
	}

	if len(seen) != total {
		t.Fatalf("paged %d distinct contacts, want %d", len(seen), total)
	}
	for id, n := range seen {
		if n != 1 {
			t.Fatalf("contact %s returned %d times", id, n)
		}
	}
}
