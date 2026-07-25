package api_test

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/bitllow/sild/backend/internal/archive"
	"github.com/bitllow/sild/backend/internal/store/models"
	"github.com/bitllow/sild/backend/internal/testutil"
)

// Archived history must page like hot history. The sink stores messages
// ascending (the archive job drains ListAfter), so slicing it with a generic
// descending pager reads the wrong end and re-serves page one forever.
func TestArchivedHistoryPagesToExhaustion(t *testing.T) {
	h := testutil.New(t)
	tenant := h.SeedTenant()
	key := h.SeedAPIKey(tenant.ID)
	tok := h.MintToken(tenant.ID, "u_client")
	ctx := context.Background()

	var conv struct {
		ID string `json:"id"`
	}
	testutil.DecodeJSON(t, h.Request("POST", "/v1/conversations").Bearer(tok).JSON(map[string]any{}).Do(), &conv)

	const total = 7
	for i := range total {
		h.Request("POST", "/v1/conversations/"+conv.ID+"/messages").
			Bearer(tok).JSON(map[string]any{"body": fmt.Sprintf("m%d", i)}).Do()
	}
	if w := h.Request("POST", "/v1/conversations/"+conv.ID+"/close").Bearer(key).Do(); w.Code != http.StatusOK {
		t.Fatalf("close: %d %s", w.Code, w.Body)
	}

	sink, _ := archive.New(h.Cfg)
	job := archive.NewJob(h.Store, sink, h.Cfg)
	job.SetClock(func() time.Time { return time.Now().Add(60 * 24 * time.Hour) })
	if n, err := job.RunOnce(ctx, tenant.ID, 100); err != nil || n != 1 {
		t.Fatalf("archive: n=%d err=%v", n, err)
	}

	seen := map[string]int{}
	path := "/v1/conversations/" + conv.ID + "/messages?limit=2"
	for pages := 0; ; pages++ {
		if pages > total+2 {
			t.Fatal("archived pagination did not terminate — page one is repeating")
		}
		e := getPage(t, h, path, tok)
		for _, m := range e.Items {
			id, _ := m["id"].(string)
			seen[id]++
		}
		if !e.HasMore {
			break
		}
		path = "/v1/conversations/" + conv.ID + "/messages?limit=2&cursor=" + *e.NextCursor
	}

	if len(seen) != total {
		t.Fatalf("paged %d distinct archived messages, want %d", len(seen), total)
	}
	for id, n := range seen {
		if n != 1 {
			t.Fatalf("archived message %s returned %d times", id, n)
		}
	}
}

// An archived support conversation keeps no assignment (PurgeHot removes it), so
// an agent's assignment-requiring scope must exempt it — otherwise the list hides
// exactly what a direct read allows.
func TestArchivedSupportStaysVisibleToAgents(t *testing.T) {
	h := testutil.New(t)
	tenant := h.SeedTenant()
	key := h.SeedAPIKey(tenant.ID)
	tok := h.MintToken(tenant.ID, "u_client")
	ctx := context.Background()

	h.SeedAdmin(tenant.ID, "agent@test", models.PlatformAgent)
	agent := loginAs(t, h, "agent@test")

	var conv struct {
		ID string `json:"id"`
	}
	testutil.DecodeJSON(t, h.Request("POST", "/v1/conversations").Bearer(tok).JSON(map[string]any{}).Do(), &conv)
	h.Request("POST", "/v1/conversations/"+conv.ID+"/messages").Bearer(tok).JSON(map[string]any{"body": "hi"}).Do()
	h.Request("POST", "/v1/conversations/"+conv.ID+"/close").Bearer(key).Do()

	sink, _ := archive.New(h.Cfg)
	job := archive.NewJob(h.Store, sink, h.Cfg)
	job.SetClock(func() time.Time { return time.Now().Add(60 * 24 * time.Hour) })
	if n, err := job.RunOnce(ctx, tenant.ID, 100); err != nil || n != 1 {
		t.Fatalf("archive: n=%d err=%v", n, err)
	}

	// A direct read is allowed...
	if w := h.Request("GET", "/v1/conversations/"+conv.ID).Cookie("sild_admin", agent).Do(); w.Code != http.StatusOK {
		t.Fatalf("agent direct read of archived support: %d %s", w.Code, w.Body)
	}
	// ...so the list must not hide it.
	var page struct {
		Items []map[string]any `json:"items"`
	}
	w := h.Request("GET", "/v1/conversations?kind=support&limit=50").Cookie("sild_admin", agent).Do()
	testutil.DecodeJSON(t, w, &page)
	for _, it := range page.Items {
		if id, _ := it["id"].(string); id == conv.ID {
			return
		}
	}
	t.Fatalf("archived support conversation missing from the agent's list: %+v", page.Items)
}

// Search pages through ALL matches. Capping the match set and paginating that
// fixed slice reports has_more:false once it is exhausted, silently truncating
// large result sets.
func TestSearchPagesBeyondTheFirstPage(t *testing.T) {
	h := testutil.New(t)
	tenant := h.SeedTenant()
	tok := h.MintToken(tenant.ID, "u_client")
	h.SeedAdmin(tenant.ID, "owner@test", models.PlatformOwner)
	owner := loginAs(t, h, "owner@test")

	const total = 5
	for i := range total {
		var conv struct {
			ID string `json:"id"`
		}
		testutil.DecodeJSON(t, h.Request("POST", "/v1/conversations").Bearer(tok).JSON(map[string]any{}).Do(), &conv)
		h.Request("POST", "/v1/conversations/"+conv.ID+"/messages").
			Bearer(tok).JSON(map[string]any{"body": fmt.Sprintf("needle%d haystack", i)}).Do()
	}

	seen := map[string]int{}
	path := "/v1/conversations?kind=support&q=haystack&limit=2"
	for pages := 0; ; pages++ {
		if pages > total+2 {
			t.Fatal("search pagination did not terminate")
		}
		w := h.Request("GET", path).Cookie("sild_admin", owner).Do()
		if w.Code != http.StatusOK {
			t.Fatalf("search: %d %s", w.Code, w.Body)
		}
		var e envelope
		testutil.DecodeJSON(t, w, &e)
		for _, it := range e.Items {
			id, _ := it["id"].(string)
			seen[id]++
		}
		if !e.HasMore {
			break
		}
		path = "/v1/conversations?kind=support&q=haystack&limit=2&cursor=" + *e.NextCursor
	}

	if len(seen) != total {
		t.Fatalf("search paged %d distinct conversations, want %d", len(seen), total)
	}
	for id, n := range seen {
		if n != 1 {
			t.Fatalf("search returned %s %d times", id, n)
		}
	}
}

// sort=waiting_since orders by the ASSIGNMENT's created_at, so its cursor must
// carry that timestamp — not the conversation's last activity, which would make
// the next page compare against an unrelated value.
func TestWaitingSinceCursorPagesCorrectly(t *testing.T) {
	h := testutil.New(t)
	tenant := h.SeedTenant()
	tok := h.MintToken(tenant.ID, "u_client")
	h.SeedAdmin(tenant.ID, "owner@test", models.PlatformOwner)
	owner := loginAs(t, h, "owner@test")

	const total = 5
	for range total {
		h.Request("POST", "/v1/conversations").Bearer(tok).JSON(map[string]any{}).Do()
	}

	seen := map[string]int{}
	base := "/v1/conversations?kind=support&sort=waiting_since&order=asc&limit=2"
	path := base
	for pages := 0; ; pages++ {
		if pages > total+2 {
			t.Fatal("waiting_since pagination did not terminate")
		}
		w := h.Request("GET", path).Cookie("sild_admin", owner).Do()
		if w.Code != http.StatusOK {
			t.Fatalf("queue: %d %s", w.Code, w.Body)
		}
		var e envelope
		testutil.DecodeJSON(t, w, &e)
		for _, it := range e.Items {
			id, _ := it["id"].(string)
			seen[id]++
		}
		if !e.HasMore {
			break
		}
		path = base + "&cursor=" + *e.NextCursor
	}

	if len(seen) != total {
		t.Fatalf("waiting_since paged %d distinct conversations, want %d", len(seen), total)
	}
	for id, n := range seen {
		if n != 1 {
			t.Fatalf("waiting_since returned %s %d times", id, n)
		}
	}
}

// Ids that cannot be addressed later are rejected where they ENTER the system:
// gin unescapes the path before matching, so an id containing "/" would never
// reach the handler that could reject it on read.
func TestExternalUserIDValidatedAtWrite(t *testing.T) {
	h := testutil.New(t)
	tenant := h.SeedTenant()
	key := h.SeedAPIKey(tenant.ID)

	bad := []string{"has/slash", "ctrl\x01char", string(make([]byte, 200))}
	for _, id := range bad {
		w := h.Request("POST", "/v1/tokens").Bearer(key).JSON(map[string]any{"user_id": id}).Do()
		if w.Code < 400 {
			t.Fatalf("token mint accepted an unaddressable id %q: %d %s", id, w.Code, w.Body)
		}
		w = h.Request("POST", "/v1/conversations").Bearer(key).JSON(map[string]any{
			"members": []map[string]any{{"user_id": id}},
		}).Do()
		if w.Code < 400 {
			t.Fatalf("member add accepted an unaddressable id %q: %d %s", id, w.Code, w.Body)
		}
	}
}
