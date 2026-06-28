package api_test

import (
	"net/http"
	"testing"

	"github.com/bitllow/sild/backend/internal/store/models"
	"github.com/bitllow/sild/backend/internal/testutil"
)

// §2.1: an invalid API key is rejected.
func TestInvalidAPIKeyRejected(t *testing.T) {
	h := testutil.New(t)
	h.SeedTenant()
	w := h.Request("POST", "/v1/conversations").Bearer("sild_live_deadbeef_nope").JSON(map[string]any{
		"members": []map[string]any{{"user_id": "u1"}},
	}).Do()
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d %s", w.Code, w.Body)
	}
}

// §4.0: a user JWT may NOT create arbitrary conversations (API-key only route).
func TestUserCannotCreateConversation(t *testing.T) {
	h := testutil.New(t)
	tenant := h.SeedTenant()
	tok := h.MintToken(tenant.ID, "u_client")
	w := h.Request("POST", "/v1/conversations").Bearer(tok).JSON(map[string]any{
		"members": []map[string]any{{"user_id": "u_client"}},
	}).Do()
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for user creating conversation, got %d %s", w.Code, w.Body)
	}
}

// §4.2/§7: a non-member user cannot read a conversation.
func TestNonMemberForbidden(t *testing.T) {
	h := testutil.New(t)
	tenant := h.SeedTenant()
	key := h.SeedAPIKey(tenant.ID)
	var conv struct{ ID string `json:"id"` }
	w := h.Request("POST", "/v1/conversations").Bearer(key).JSON(map[string]any{
		"members": []map[string]any{{"user_id": "u_member"}},
	}).Do()
	testutil.DecodeJSON(t, w, &conv)

	outsider := h.MintToken(tenant.ID, "u_outsider")
	if w = h.Request("GET", "/v1/conversations/"+conv.ID).Bearer(outsider).Do(); w.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for non-member, got %d %s", w.Code, w.Body)
	}
}

// §7: platform RBAC — an agent cannot manage API keys; an owner can.
func TestPlatformRoleGuardsAPIKeys(t *testing.T) {
	h := testutil.New(t)
	tenant := h.SeedTenant()
	h.SeedAdmin(tenant.ID, "agent@test", models.PlatformAgent)
	h.SeedAdmin(tenant.ID, "owner@test", models.PlatformOwner)

	agentCookie := loginAs(t, h, "agent@test")
	ownerCookie := loginAs(t, h, "owner@test")

	if w := h.Request("POST", "/v1/admin/api-keys").Cookie("sild_admin", agentCookie).JSON(map[string]any{"label": "x"}).Do(); w.Code != http.StatusForbidden {
		t.Fatalf("agent must not create api keys, got %d %s", w.Code, w.Body)
	}
	if w := h.Request("POST", "/v1/admin/api-keys").Cookie("sild_admin", ownerCookie).JSON(map[string]any{"label": "x"}).Do(); w.Code != http.StatusCreated {
		t.Fatalf("owner should create api keys, got %d %s", w.Code, w.Body)
	}
}

// §2.5: JWKS endpoint exposes verification keys.
func TestJWKSEndpoint(t *testing.T) {
	h := testutil.New(t)
	w := h.Request("GET", "/.well-known/jwks.json").Do()
	if w.Code != http.StatusOK {
		t.Fatalf("jwks: %d", w.Code)
	}
	var set struct {
		Keys []map[string]any `json:"keys"`
	}
	testutil.DecodeJSON(t, w, &set)
	if len(set.Keys) == 0 {
		t.Fatal("expected at least one JWK")
	}
}

// loginAs performs the dev-stub admin login and returns the session cookie.
func loginAs(t *testing.T, h *testutil.Harness, email string) string {
	t.Helper()
	w := h.Request("GET", "/v1/admin/auth/google/dev?email="+email).Do()
	if w.Code != http.StatusOK {
		t.Fatalf("login %s: %d %s", email, w.Code, w.Body)
	}
	return extractCookie(w.Header().Get("Set-Cookie"))
}
