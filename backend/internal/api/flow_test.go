package api_test

import (
	"net/http"
	"testing"

	"github.com/bitllow/sild/backend/internal/store/models"
	"github.com/bitllow/sild/backend/internal/testutil"
)

// TestEndToEndSupportFlow exercises the core loop end-to-end through HTTP:
// mint token → open support request → agent answers → client catches up.
func TestEndToEndSupportFlow(t *testing.T) {
	h := testutil.New(t)
	tenant := h.SeedTenant()
	apiKey := h.SeedAPIKey(tenant.ID)

	// 1) Host mints a user token via the API key (§4.1).
	var tokRes struct {
		Token string `json:"token"`
	}
	w := h.Request("POST", "/v1/tokens").Bearer(apiKey).JSON(map[string]any{"user_id": "u_client_1"}).Do()
	if w.Code != http.StatusOK {
		t.Fatalf("mint token: %d %s", w.Code, w.Body)
	}
	testutil.DecodeJSON(t, w, &tokRes)
	if tokRes.Token == "" {
		t.Fatal("expected a token")
	}

	// 2) Client opens a support request (§4.2).
	var convRes struct {
		ID         string         `json:"id"`
		Status     string         `json:"status"`
		Assignment map[string]any `json:"assignment"`
	}
	w = h.Request("POST", "/v1/me/support-requests").Bearer(tokRes.Token).JSON(map[string]any{}).Do()
	if w.Code != http.StatusCreated {
		t.Fatalf("open support: %d %s", w.Code, w.Body)
	}
	testutil.DecodeJSON(t, w, &convRes)
	if convRes.ID == "" || convRes.Status != "open" || convRes.Assignment == nil {
		t.Fatalf("unexpected conversation: %+v", convRes)
	}

	// 3) Client sends a message (§4.2).
	w = h.Request("POST", "/v1/conversations/"+convRes.ID+"/messages").
		Bearer(tokRes.Token).JSON(map[string]any{"body": "I need help", "client_msg_id": "c1"}).Do()
	if w.Code != http.StatusCreated {
		t.Fatalf("send message: %d %s", w.Code, w.Body)
	}

	// 3a) Idempotency: same client_msg_id returns the same message (§4.2).
	var m1, m2 struct {
		ID string `json:"id"`
	}
	testutil.DecodeJSON(t, w, &m1)
	w = h.Request("POST", "/v1/conversations/"+convRes.ID+"/messages").
		Bearer(tokRes.Token).JSON(map[string]any{"body": "I need help", "client_msg_id": "c1"}).Do()
	testutil.DecodeJSON(t, w, &m2)
	if m1.ID != m2.ID {
		t.Fatalf("idempotency broken: %s != %s", m1.ID, m2.ID)
	}

	// 4) Agent logs in (dev stub) and answers.
	h.SeedAdmin(tenant.ID, "agent@test.local", models.PlatformAgent)
	w = h.Request("GET", "/v1/admin/auth/google/dev?email=agent@test.local").Do()
	if w.Code != http.StatusOK {
		t.Fatalf("admin login: %d %s", w.Code, w.Body)
	}
	cookie := extractCookie(w.Header().Get("Set-Cookie"))
	w = h.Request("POST", "/v1/conversations/"+convRes.ID+"/messages").
		Cookie("sild_admin", cookie).JSON(map[string]any{"body": "How can I help?"}).Do()
	if w.Code != http.StatusCreated {
		t.Fatalf("agent reply: %d %s", w.Code, w.Body)
	}

	// 5) Client catch-up via after= sees the agent reply (§5.4).
	var after struct {
		Messages []map[string]any `json:"messages"`
	}
	w = h.Request("GET", "/v1/conversations/"+convRes.ID+"/messages?after="+m1.ID).Bearer(tokRes.Token).Do()
	if w.Code != http.StatusOK {
		t.Fatalf("catch-up: %d %s", w.Code, w.Body)
	}
	testutil.DecodeJSON(t, w, &after)
	if len(after.Messages) != 1 || after.Messages[0]["body"] != "How can I help?" {
		t.Fatalf("expected the agent reply in catch-up, got %+v", after.Messages)
	}

	// 6) message.created events were emitted to realtime (§5.3).
	if len(h.Pub.OfType("message.created")) < 2 {
		t.Fatalf("expected >=2 message.created events, got %d", len(h.Pub.OfType("message.created")))
	}
}

// extractCookie pulls the cookie value out of a Set-Cookie header.
func extractCookie(setCookie string) string {
	// "sild_admin=VALUE; Path=/; ..."
	for i := 0; i < len(setCookie); i++ {
		if setCookie[i] == '=' {
			rest := setCookie[i+1:]
			for j := 0; j < len(rest); j++ {
				if rest[j] == ';' {
					return rest[:j]
				}
			}
			return rest
		}
	}
	return ""
}
