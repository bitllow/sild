package api_test

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/bitllow/sild/backend/internal/store/models"
	"github.com/bitllow/sild/backend/internal/testutil"
)

// Every collection is driven to exhaustion at limit=2, asserting no duplicate and
// no skipped row. Envelope-shape coverage does not catch a keyset that stalls,
// repeats or drops — the contacts cursor passed the shape test while silently
// losing seven of twelve rows.
func TestEveryCollectionPagesToExhaustion(t *testing.T) {
	h := testutil.New(t)
	tenant := h.SeedTenant()
	h.SeedAdmin(tenant.ID, "owner@ex", models.PlatformOwner)
	owner := loginAs(t, h, "owner@ex")

	// Seed enough of everything to need several pages at limit=2.
	const n = 5
	var convID, webhookID string
	for i := range n {
		tok := h.MintToken(tenant.ID, fmt.Sprintf("u_ex_%02d", i))
		var conv struct {
			ID string `json:"id"`
		}
		testutil.DecodeJSON(t, h.Request("POST", "/v1/conversations").Bearer(tok).JSON(map[string]any{}).Do(), &conv)
		convID = conv.ID
		for j := range 3 {
			h.Request("POST", "/v1/conversations/"+conv.ID+"/messages").
				Bearer(tok).JSON(map[string]any{"body": fmt.Sprintf("hay %d-%d", i, j)}).Do()
		}
		h.Request("POST", "/v1/api-keys").Cookie("sild_admin", owner).
			JSON(map[string]any{"label": fmt.Sprintf("k%d", i)}).Do()
		var wh struct {
			ID string `json:"id"`
		}
		testutil.DecodeJSON(t, h.Request("POST", "/v1/webhooks").Cookie("sild_admin", owner).
			JSON(map[string]any{"url": fmt.Sprintf("https://ex.test/%d", i), "events": []string{"message.created"}}).Do(), &wh)
		webhookID = wh.ID
		h.Request("POST", "/v1/team").Cookie("sild_admin", owner).
			JSON(map[string]any{"email": fmt.Sprintf("ex%d@test", i), "platform_role": "agent"}).Do()
	}

	// Deliveries are written by the webhook worker, not by any route, so seed them
	// directly — otherwise the collection is "paged" with nothing in it.
	const deliveries = 5
	for i := range deliveries {
		if err := h.Store.Webhooks().LogDelivery(context.Background(), &models.WebhookDelivery{
			TenantID:   tenant.ID,
			EndpointID: webhookID,
			EventID:    fmt.Sprintf("ev_%02d", i),
			EventType:  "message.created",
			Attempt:    1,
			Status:     models.DeliveryDelivered,
			StatusCode: 200,
		}); err != nil {
			t.Fatalf("seed delivery %d: %v", i, err)
		}
	}

	admin := func(r *testutil.Req) *testutil.Req { return r.Cookie("sild_admin", owner) }

	for _, tc := range []struct {
		name string
		path string
		want int
		as   func(*testutil.Req) *testutil.Req
	}{
		{"conversations", "/v1/conversations", n, admin},
		{"conversations search", "/v1/conversations?q=hay", n, admin},
		{"messages", "/v1/conversations/" + convID + "/messages", 3, admin},
		{"contacts", "/v1/contacts", n, admin},
		{"contacts search", "/v1/contacts?q=u_ex", n, admin},
		{"api keys", "/v1/api-keys", n, admin},
		{"webhooks", "/v1/webhooks", n, admin},
		{"webhook deliveries", "/v1/webhooks/" + webhookID + "/deliveries", deliveries, admin},
		// n agents + the owner.
		{"team", "/v1/team", n + 1, admin},
	} {
		t.Run(tc.name, func(t *testing.T) {
			seen := map[string]int{}
			path := withParam(tc.path, "limit=2")
			for page := 0; ; page++ {
				if page > 40 {
					t.Fatal("pagination did not terminate — a page is repeating")
				}
				w := tc.as(h.Request("GET", path)).Do()
				if w.Code != http.StatusOK {
					t.Fatalf("GET %s: %d %s", path, w.Code, w.Body)
				}
				var e envelope
				testutil.DecodeJSON(t, w, &e)
				if !e.HasMore != (e.NextCursor == nil) {
					t.Errorf("has_more=%v but next_cursor=%v — null exactly when exhausted",
						e.HasMore, e.NextCursor)
				}
				for _, it := range e.Items {
					seen[rowKey(t, it)]++
				}
				if !e.HasMore {
					break
				}
				path = withParam(tc.path, "limit=2&cursor="+*e.NextCursor)
			}

			for k, count := range seen {
				if count != 1 {
					t.Errorf("row %s returned %d times across pages", k, count)
				}
			}
			if len(seen) != tc.want {
				t.Errorf("paged %d distinct rows, want %d", len(seen), tc.want)
			}
		})
	}
}

// ?since= is a sync read, not a page: continuation is the last id received, and
// has_more must be honoured or a gap longer than one limit is lost silently.
func TestCatchUpDrainsBeyondOnePage(t *testing.T) {
	h := testutil.New(t)
	tenant := h.SeedTenant()
	tok := h.MintToken(tenant.ID, "u_catchup")

	var conv struct {
		ID string `json:"id"`
	}
	testutil.DecodeJSON(t, h.Request("POST", "/v1/conversations").Bearer(tok).JSON(map[string]any{}).Do(), &conv)

	const total = 7
	ids := make([]string, 0, total)
	for i := range total {
		var m struct {
			ID string `json:"id"`
		}
		testutil.DecodeJSON(t, h.Request("POST", "/v1/conversations/"+conv.ID+"/messages").
			Bearer(tok).JSON(map[string]any{"body": fmt.Sprintf("m%d", i)}).Do(), &m)
		ids = append(ids, m.ID)
	}

	// Pretend the socket dropped right after the first message.
	since, seen := ids[0], []string{}
	for round := 0; ; round++ {
		if round > total {
			t.Fatal("catch-up did not terminate")
		}
		w := h.Request("GET", "/v1/conversations/"+conv.ID+"/messages?since="+since+"&limit=2").Bearer(tok).Do()
		if w.Code != http.StatusOK {
			t.Fatalf("since=%s: %d %s", since, w.Code, w.Body)
		}
		var e envelope
		testutil.DecodeJSON(t, w, &e)
		if e.NextCursor != nil {
			t.Errorf("catch-up must return next_cursor:null, got %q", *e.NextCursor)
		}
		if len(e.Items) == 0 {
			break
		}
		for _, it := range e.Items {
			seen = append(seen, rowKey(t, it))
		}
		since = seen[len(seen)-1]
		if !e.HasMore {
			break
		}
	}

	if len(seen) != total-1 {
		t.Fatalf("drained %d messages, want %d", len(seen), total-1)
	}
	for i, id := range seen {
		if id != ids[i+1] {
			t.Fatalf("position %d is %s, want %s — catch-up is oldest-first", i, id, ids[i+1])
		}
	}
}

// rowKey is a collection-agnostic identity: most rows carry `id`, contacts are
// keyed by external_user_id.
func rowKey(t *testing.T, row map[string]any) string {
	t.Helper()
	for _, field := range []string{"id", "external_user_id"} {
		if v, ok := row[field].(string); ok && v != "" {
			return v
		}
	}
	t.Fatalf("row has no identity field: %v", row)
	return ""
}

func withParam(path, param string) string {
	if strings.Contains(path, "?") {
		return path + "&" + param
	}
	return path + "?" + param
}
