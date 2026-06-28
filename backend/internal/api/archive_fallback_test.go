package api_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/bitllow/sild/backend/internal/archive"
	"github.com/bitllow/sild/backend/internal/testutil"
)

// §12: once a conversation is archived (hot rows purged), GET messages and GET
// conversation fall back to the sink, and a former member is still authorized
// via the membership snapshot in the tombstone.
func TestArchivedReadFallbackThroughAPI(t *testing.T) {
	h := testutil.New(t)
	tenant := h.SeedTenant()
	key := h.SeedAPIKey(tenant.ID)
	tok := h.MintToken(tenant.ID, "u_client")
	ctx := context.Background()

	// client opens a support request and sends a message
	var conv struct{ ID string `json:"id"` }
	w := h.Request("POST", "/v1/me/support-requests").Bearer(tok).JSON(map[string]any{}).Do()
	testutil.DecodeJSON(t, w, &conv)
	h.Request("POST", "/v1/conversations/"+conv.ID+"/messages").Bearer(tok).JSON(map[string]any{"body": "archived hello"}).Do()

	// close it (API key) and archive it (idle clock)
	if w = h.Request("POST", "/v1/conversations/"+conv.ID+"/close").Bearer(key).Do(); w.Code != http.StatusOK {
		t.Fatalf("close: %d %s", w.Code, w.Body)
	}
	sink, _ := archive.New(h.Cfg)
	job := archive.NewJob(h.Store, sink, h.Cfg)
	job.SetClock(func() time.Time { return time.Now().Add(60 * 24 * time.Hour) })
	if n, err := job.RunOnce(ctx, tenant.ID, 100); err != nil || n != 1 {
		t.Fatalf("archive: n=%d err=%v", n, err)
	}

	// the former member can still read the (archived) messages
	var page struct {
		Messages []map[string]any `json:"messages"`
	}
	w = h.Request("GET", "/v1/conversations/"+conv.ID+"/messages").Bearer(tok).Do()
	if w.Code != http.StatusOK {
		t.Fatalf("archived messages read: %d %s", w.Code, w.Body)
	}
	testutil.DecodeJSON(t, w, &page)
	if len(page.Messages) != 1 || page.Messages[0]["body"] != "archived hello" {
		t.Fatalf("expected archived message via fallback, got %+v", page.Messages)
	}

	// and the archived conversation view
	var cv struct {
		ID       string `json:"id"`
		Archived bool   `json:"archived"`
	}
	w = h.Request("GET", "/v1/conversations/"+conv.ID).Bearer(tok).Do()
	if w.Code != http.StatusOK {
		t.Fatalf("archived conv read: %d %s", w.Code, w.Body)
	}
	testutil.DecodeJSON(t, w, &cv)
	if cv.ID != conv.ID || !cv.Archived {
		t.Fatalf("expected archived conversation view, got %+v", cv)
	}
}
