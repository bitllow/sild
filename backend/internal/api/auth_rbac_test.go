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

// §4.0: a user JWT may open a SUPPORT conversation for itself, but never a peer
// one — a peer conversation is visible to every peer_access operator, so minting
// one would let a user inject rows into the operator peer inbox.
func TestUserCanOpenSupportButNotPeer(t *testing.T) {
	h := testutil.New(t)
	tenant := h.SeedTenant()
	tok := h.MintToken(tenant.ID, "u_client")

	w := h.Request("POST", "/v1/conversations").Bearer(tok).JSON(map[string]any{}).Do()
	if w.Code != http.StatusCreated {
		t.Fatalf("user should open a support conversation, got %d %s", w.Code, w.Body)
	}

	w = h.Request("POST", "/v1/conversations").Bearer(tok).JSON(map[string]any{
		"open_assignment": false,
	}).Do()
	if w.Code != http.StatusForbidden {
		t.Fatalf("user must not create a peer conversation, got %d %s", w.Code, w.Body)
	}
}

// The member list in the body is ignored for a user JWT: self is the only
// member, so a user cannot add anyone to a conversation they open.
func TestUserCreateIgnoresSuppliedMembers(t *testing.T) {
	h := testutil.New(t)
	tenant := h.SeedTenant()
	tok := h.MintToken(tenant.ID, "u_client")

	var conv struct {
		Members []struct {
			ExternalUserID string `json:"external_user_id"`
		} `json:"members"`
	}
	w := h.Request("POST", "/v1/conversations").Bearer(tok).JSON(map[string]any{
		"members": []map[string]any{{"user_id": "u_someone_else"}},
	}).Do()
	if w.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", w.Code, w.Body)
	}
	testutil.DecodeJSON(t, w, &conv)
	if len(conv.Members) != 1 || conv.Members[0].ExternalUserID != "u_client" {
		t.Fatalf("expected self as the only member, got %+v", conv.Members)
	}
}

// §4.2/§7: a non-member user cannot read a conversation.
func TestNonMemberForbidden(t *testing.T) {
	h := testutil.New(t)
	tenant := h.SeedTenant()
	key := h.SeedAPIKey(tenant.ID)
	var conv struct {
		ID string `json:"id"`
	}
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

	if w := h.Request("POST", "/v1/api-keys").Cookie("sild_admin", agentCookie).JSON(map[string]any{"label": "x"}).Do(); w.Code != http.StatusForbidden {
		t.Fatalf("agent must not create api keys, got %d %s", w.Code, w.Body)
	}
	if w := h.Request("POST", "/v1/api-keys").Cookie("sild_admin", ownerCookie).JSON(map[string]any{"label": "x"}).Do(); w.Code != http.StatusCreated {
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
