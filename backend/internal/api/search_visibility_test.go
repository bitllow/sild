package api_test

import (
	"net/http"
	"testing"

	"github.com/bitllow/sild/backend/internal/store/models"
	"github.com/bitllow/sild/backend/internal/testutil"
)

// §5.6: a note a user cannot read must not be findable by them either. Search
// reuses the operator query, so without a visibility filter it both matches the
// conversation and quotes the note back as the snippet.
func TestUserSearchNeverMatchesInternalNotes(t *testing.T) {
	h := testutil.New(t)
	tenant := h.SeedTenant()
	key := h.SeedAPIKey(tenant.ID)
	tok := h.MintToken(tenant.ID, "u_client")

	var conv struct {
		ID string `json:"id"`
	}
	testutil.DecodeJSON(t, h.Request("POST", "/v1/conversations").Bearer(tok).JSON(map[string]any{}).Do(), &conv)
	h.Request("POST", "/v1/conversations/"+conv.ID+"/messages").Bearer(tok).
		JSON(map[string]any{"body": "visible hello"}).Do()

	const secret = "internalsecretxyz"
	if w := h.Request("POST", "/v1/conversations/"+conv.ID+"/messages").Bearer(key).JSON(map[string]any{
		"body": secret, "visibility": "internal",
		"sender_kind": "agent", "internal_actor_id": "agent_42",
	}).Do(); w.Code != http.StatusCreated {
		t.Fatalf("seed internal note: %d %s", w.Code, w.Body)
	}

	w := h.Request("GET", "/v1/conversations?q="+secret).Bearer(tok).Do()
	if w.Code != http.StatusOK {
		t.Fatalf("user search: %d %s", w.Code, w.Body)
	}
	var page struct {
		Items []map[string]any `json:"items"`
	}
	testutil.DecodeJSON(t, w, &page)
	if len(page.Items) != 0 {
		t.Fatalf("an internal note was findable by a user: %+v", page.Items)
	}

	// An operator DOES find it — proving the probe reaches real data.
	h.SeedAdmin(tenant.ID, "owner@test", models.PlatformOwner)
	owner := loginAs(t, h, "owner@test")
	ow := h.Request("GET", "/v1/conversations?kind=support&q="+secret).Cookie("sild_admin", owner).Do()
	testutil.DecodeJSON(t, ow, &page)
	if len(page.Items) != 1 {
		t.Fatalf("operator should find the internal note, got %+v", page.Items)
	}
}

// A user's search is bounded by their own membership, applied inside the query so
// pagination never sees another user's conversations.
func TestUserSearchIsScopedToOwnConversations(t *testing.T) {
	h := testutil.New(t)
	tenant := h.SeedTenant()
	mine := h.MintToken(tenant.ID, "u_mine")
	theirs := h.MintToken(tenant.ID, "u_theirs")

	for tok, body := range map[string]string{mine: "sharedneedle mine", theirs: "sharedneedle theirs"} {
		var conv struct {
			ID string `json:"id"`
		}
		testutil.DecodeJSON(t, h.Request("POST", "/v1/conversations").Bearer(tok).JSON(map[string]any{}).Do(), &conv)
		h.Request("POST", "/v1/conversations/"+conv.ID+"/messages").Bearer(tok).
			JSON(map[string]any{"body": body}).Do()
	}

	w := h.Request("GET", "/v1/conversations?q=sharedneedle").Bearer(mine).Do()
	var page struct {
		Items []map[string]any `json:"items"`
	}
	testutil.DecodeJSON(t, w, &page)
	if len(page.Items) != 1 {
		t.Fatalf("search must return only the caller's conversation, got %d", len(page.Items))
	}
}
