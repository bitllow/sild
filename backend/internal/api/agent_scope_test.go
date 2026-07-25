package api_test

import (
	"net/http"
	"testing"

	"github.com/bitllow/sild/backend/internal/store/models"
	"github.com/bitllow/sild/backend/internal/testutil"
)

// §7: a platform agent may access support conversations (those with an
// assignment) but NOT arbitrary driver/client conversations.
func TestAgentScopedToSupportConversations(t *testing.T) {
	h := testutil.New(t)
	tenant := h.SeedTenant()
	key := h.SeedAPIKey(tenant.ID)
	h.SeedAdmin(tenant.ID, "agent@test", models.PlatformAgent)
	agent := loginAs(t, h, "agent@test")

	// a plain (non-support) conversation — no assignment
	var plain struct {
		ID string `json:"id"`
	}
	w := h.Request("POST", "/v1/conversations").Bearer(key).JSON(map[string]any{
		"members": []map[string]any{
			{"user_id": "u_client", "conv_role": "client"},
			{"user_id": "u_driver", "conv_role": "driver"},
		},
	}).Do()
	testutil.DecodeJSON(t, w, &plain)

	// a support conversation — carries an assignment
	var support struct {
		ID string `json:"id"`
	}
	w = h.Request("POST", "/v1/conversations").Bearer(key).JSON(map[string]any{
		"members":         []map[string]any{{"user_id": "u_client", "conv_role": "client"}},
		"open_assignment": true,
	}).Do()
	testutil.DecodeJSON(t, w, &support)

	// agent is blocked from the plain conversation
	if w = h.Request("GET", "/v1/conversations/"+plain.ID).Cookie("sild_admin", agent).Do(); w.Code != http.StatusForbidden {
		t.Fatalf("agent must not access a non-support conversation, got %d %s", w.Code, w.Body)
	}
	// agent can access the support conversation
	if w = h.Request("GET", "/v1/conversations/"+support.ID).Cookie("sild_admin", agent).Do(); w.Code != http.StatusOK {
		t.Fatalf("agent should access a support conversation, got %d %s", w.Code, w.Body)
	}

	// owner has tenant-wide access to support conversations, with no assignment needed
	h.SeedAdmin(tenant.ID, "owner@test", models.PlatformOwner)
	owner := loginAs(t, h, "owner@test")
	if w = h.Request("GET", "/v1/conversations/"+support.ID).Cookie("sild_admin", owner).Do(); w.Code != http.StatusOK {
		t.Fatalf("owner should access any support conversation, got %d %s", w.Code, w.Body)
	}
	// ...but the plain conversation is a PEER conversation (two end-user parties, no
	// assignment), and reaching one is a per-operator opt-in the owner grants — so
	// tenant-wide role scope does not cover it. See TestPeerAccessGatesEveryRole.
	if w = h.Request("GET", "/v1/conversations/"+plain.ID).Cookie("sild_admin", owner).Do(); w.Code != http.StatusForbidden {
		t.Fatalf("owner without peer access must not read a peer conversation, got %d %s", w.Code, w.Body)
	}
}
