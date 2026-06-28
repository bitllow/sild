package api_test

import (
	"net/http"
	"testing"

	"github.com/bitllow/sild/backend/internal/testutil"
)

// §1: create is atomic — conversation + members + assignment in one shot.
func TestCreateConversationAtomicWithAssignment(t *testing.T) {
	h := testutil.New(t)
	tenant := h.SeedTenant()
	key := h.SeedAPIKey(tenant.ID)

	var res struct {
		ID         string           `json:"id"`
		Members    []map[string]any `json:"members"`
		Assignment map[string]any   `json:"assignment"`
	}
	w := h.Request("POST", "/v1/conversations").Bearer(key).JSON(map[string]any{
		"reference": "trip_1",
		"members": []map[string]any{
			{"user_id": "u_client", "conv_role": "client"},
			{"user_id": "u_driver", "conv_role": "driver"},
		},
		"open_assignment": true,
	}).Do()
	if w.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", w.Code, w.Body)
	}
	testutil.DecodeJSON(t, w, &res)
	if len(res.Members) != 2 || res.Assignment == nil {
		t.Fatalf("expected 2 members + assignment, got %+v", res)
	}
}

// §1: removing the last member of an OPEN conversation is rejected (409).
func TestRemoveLastMemberRejected(t *testing.T) {
	h := testutil.New(t)
	tenant := h.SeedTenant()
	key := h.SeedAPIKey(tenant.ID)

	var conv struct {
		ID string `json:"id"`
	}
	w := h.Request("POST", "/v1/conversations").Bearer(key).JSON(map[string]any{
		"members": []map[string]any{{"user_id": "u_only", "conv_role": "client"}},
	}).Do()
	testutil.DecodeJSON(t, w, &conv)

	w = h.Request("DELETE", "/v1/conversations/"+conv.ID+"/members/u_only").Bearer(key).Do()
	if w.Code != http.StatusConflict {
		t.Fatalf("expected 409 removing last member, got %d %s", w.Code, w.Body)
	}
}

// §1: closed is terminal; close is idempotent and a closed conv may go empty.
func TestCloseThenRemoveAllowed(t *testing.T) {
	h := testutil.New(t)
	tenant := h.SeedTenant()
	key := h.SeedAPIKey(tenant.ID)

	var conv struct {
		ID string `json:"id"`
	}
	w := h.Request("POST", "/v1/conversations").Bearer(key).JSON(map[string]any{
		"members": []map[string]any{{"user_id": "u_only", "conv_role": "client"}},
	}).Do()
	testutil.DecodeJSON(t, w, &conv)

	if w = h.Request("POST", "/v1/conversations/"+conv.ID+"/close").Bearer(key).Do(); w.Code != http.StatusOK {
		t.Fatalf("close: %d %s", w.Code, w.Body)
	}
	// now removing the last member is allowed (conversation is closed)
	if w = h.Request("DELETE", "/v1/conversations/"+conv.ID+"/members/u_only").Bearer(key).Do(); w.Code != http.StatusNoContent {
		t.Fatalf("expected 204 removing member from closed conv, got %d %s", w.Code, w.Body)
	}
}

// §1: tenant is never client-supplied — tenant B cannot read tenant A's conv.
func TestCrossTenantIsolation(t *testing.T) {
	h := testutil.New(t)
	tenantA := h.SeedTenant()
	keyA := h.SeedAPIKey(tenantA.ID)
	tenantB := h.SeedTenant()
	keyB := h.SeedAPIKey(tenantB.ID)

	var conv struct {
		ID string `json:"id"`
	}
	w := h.Request("POST", "/v1/conversations").Bearer(keyA).JSON(map[string]any{
		"members": []map[string]any{{"user_id": "u1", "conv_role": "client"}},
	}).Do()
	testutil.DecodeJSON(t, w, &conv)

	// tenant B's key must not see tenant A's conversation.
	if w = h.Request("GET", "/v1/conversations/"+conv.ID).Bearer(keyB).Do(); w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 cross-tenant, got %d %s", w.Code, w.Body)
	}
	// tenant A's key sees it.
	if w = h.Request("GET", "/v1/conversations/"+conv.ID).Bearer(keyA).Do(); w.Code != http.StatusOK {
		t.Fatalf("owner should read own conv, got %d %s", w.Code, w.Body)
	}
}
