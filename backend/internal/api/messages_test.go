package api_test

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/bitllow/sild/backend/internal/store/models"
	"github.com/bitllow/sild/backend/internal/testutil"
)

// §5.6: internal notes are stripped from a client's history and posted only to
// the internal channel; the client can never set visibility=internal.
func TestInternalNoteIsolation(t *testing.T) {
	h := testutil.New(t)
	tenant := h.SeedTenant()
	tok := h.MintToken(tenant.ID, "u_client")
	h.SeedAdmin(tenant.ID, "agent@test", models.PlatformAgent)
	agentCookie := loginAs(t, h, "agent@test")

	var conv struct{ ID string `json:"id"` }
	w := h.Request("POST", "/v1/me/support-requests").Bearer(tok).JSON(map[string]any{}).Do()
	testutil.DecodeJSON(t, w, &conv)

	// agent posts an internal note
	w = h.Request("POST", "/v1/conversations/"+conv.ID+"/messages").
		Cookie("sild_admin", agentCookie).JSON(map[string]any{"body": "secret note", "visibility": "internal"}).Do()
	if w.Code != http.StatusCreated {
		t.Fatalf("internal note: %d %s", w.Code, w.Body)
	}
	// agent posts a normal reply
	h.Request("POST", "/v1/conversations/"+conv.ID+"/messages").
		Cookie("sild_admin", agentCookie).JSON(map[string]any{"body": "hello"}).Do()

	// client history excludes the internal note
	var clientView struct {
		Messages []map[string]any `json:"messages"`
	}
	w = h.Request("GET", "/v1/conversations/"+conv.ID+"/messages").Bearer(tok).Do()
	testutil.DecodeJSON(t, w, &clientView)
	for _, m := range clientView.Messages {
		if m["visibility"] == "internal" || m["body"] == "secret note" {
			t.Fatalf("client must not see internal note: %+v", clientView.Messages)
		}
	}

	// agent history includes it
	var agentView struct {
		Messages []map[string]any `json:"messages"`
	}
	w = h.Request("GET", "/v1/conversations/"+conv.ID+"/messages").Cookie("sild_admin", agentCookie).Do()
	testutil.DecodeJSON(t, w, &agentView)
	if len(agentView.Messages) != len(clientView.Messages)+1 {
		t.Fatalf("agent should see one more message than client: agent=%d client=%d", len(agentView.Messages), len(clientView.Messages))
	}

	// internal note was published only to the internal channel (§5.6)
	internalEvents := 0
	for _, e := range h.Pub.OfType("message.created") {
		if e.Target.Internal {
			internalEvents++
		}
	}
	if internalEvents != 1 {
		t.Fatalf("expected 1 internal-channel publish, got %d", internalEvents)
	}
}

// §4.2: a user/guest token may not post an internal note.
func TestUserCannotPostInternal(t *testing.T) {
	h := testutil.New(t)
	tenant := h.SeedTenant()
	tok := h.MintToken(tenant.ID, "u_client")
	var conv struct{ ID string `json:"id"` }
	w := h.Request("POST", "/v1/me/support-requests").Bearer(tok).JSON(map[string]any{}).Do()
	testutil.DecodeJSON(t, w, &conv)

	w = h.Request("POST", "/v1/conversations/"+conv.ID+"/messages").
		Bearer(tok).JSON(map[string]any{"body": "x", "visibility": "internal"}).Do()
	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for user internal note, got %d %s", w.Code, w.Body)
	}
}

// §4.2: history pagination via before= with has_more.
func TestPaginationBeforeHasMore(t *testing.T) {
	h := testutil.New(t)
	tenant := h.SeedTenant()
	tok := h.MintToken(tenant.ID, "u_client")
	var conv struct{ ID string `json:"id"` }
	w := h.Request("POST", "/v1/me/support-requests").Bearer(tok).JSON(map[string]any{}).Do()
	testutil.DecodeJSON(t, w, &conv)

	for i := 0; i < 5; i++ {
		h.Request("POST", "/v1/conversations/"+conv.ID+"/messages").
			Bearer(tok).JSON(map[string]any{"body": fmt.Sprintf("m%d", i)}).Do()
	}
	var page struct {
		Messages []map[string]any `json:"messages"`
		HasMore  bool             `json:"has_more"`
	}
	w = h.Request("GET", "/v1/conversations/"+conv.ID+"/messages?limit=2").Bearer(tok).Do()
	testutil.DecodeJSON(t, w, &page)
	if len(page.Messages) != 2 || !page.HasMore {
		t.Fatalf("expected 2 messages + has_more, got len=%d has_more=%v", len(page.Messages), page.HasMore)
	}
}
