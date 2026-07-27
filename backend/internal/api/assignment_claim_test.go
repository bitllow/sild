package api_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/bitllow/sild/backend/internal/store/models"
	"github.com/bitllow/sild/backend/internal/testutil"
)

// queuedAssignment seeds a support request and returns its assignment id.
func queuedAssignment(t *testing.T, h *testutil.Harness, tenantID string) string {
	t.Helper()
	w := h.Request("POST", "/v1/conversations").
		Bearer(h.MintToken(tenantID, "u_client")).JSON(map[string]any{}).Do()
	if w.Code != http.StatusCreated {
		t.Fatalf("open support request: %d %s", w.Code, w.Body)
	}
	var conv struct {
		Assignment *struct {
			ID string `json:"id"`
		} `json:"assignment"`
	}
	testutil.DecodeJSON(t, w, &conv)
	if conv.Assignment == nil {
		t.Fatal("support conversation carries no assignment")
	}
	return conv.Assignment.ID
}

// Two agents, one queued conversation, both press Claim. The API contract must
// tell the loser it lost — a shared queue makes this the normal case, and the
// old read-modify-write answered 200 to both.
func TestSecondClaimIsRejected(t *testing.T) {
	h := testutil.New(t)
	tenant := h.SeedTenant()
	h.SeedAdmin(tenant.ID, "first@agents", models.PlatformOwner)
	h.SeedAdmin(tenant.ID, "second@agents", models.PlatformOwner)
	first := loginAs(t, h, "first@agents")
	second := loginAs(t, h, "second@agents")

	assignmentID := queuedAssignment(t, h, tenant.ID)

	claim := func(session string) (int, string) {
		w := h.Request("PATCH", "/v1/assignments/"+assignmentID).
			Cookie("sild_admin", session).JSON(map[string]any{"assignee_actor_id": "me"}).Do()
		return w.Code, w.Body.String()
	}

	if code, body := claim(first); code != http.StatusOK {
		t.Fatalf("first claim: %d %s", code, body)
	}
	code, body := claim(second)
	if code != http.StatusConflict {
		t.Fatalf("second claim: %d %s — both agents were told they own the conversation", code, body)
	}
	if !strings.Contains(body, "assignment_already_taken") {
		t.Fatalf("conflict does not name the reason: %s", body)
	}

	// The winner retrying its own claim is a retry, not a contest.
	if code, body := claim(first); code != http.StatusOK {
		t.Fatalf("owner re-claim: %d %s", code, body)
	}
}

// Claiming a closed assignment is a different conflict, and the code says which.
func TestClaimOnClosedAssignmentSaysClosed(t *testing.T) {
	h := testutil.New(t)
	tenant := h.SeedTenant()
	h.SeedAdmin(tenant.ID, "owner@agents", models.PlatformOwner)
	session := loginAs(t, h, "owner@agents")

	assignmentID := queuedAssignment(t, h, tenant.ID)

	closed := h.Request("PATCH", "/v1/assignments/"+assignmentID).
		Cookie("sild_admin", session).JSON(map[string]any{"status": "closed"}).Do()
	if closed.Code != http.StatusOK {
		t.Fatalf("close: %d %s", closed.Code, closed.Body)
	}
	w := h.Request("PATCH", "/v1/assignments/"+assignmentID).
		Cookie("sild_admin", session).JSON(map[string]any{"assignee_actor_id": "me"}).Do()
	if w.Code != http.StatusConflict {
		t.Fatalf("claim on closed: %d %s", w.Code, w.Body)
	}
	if !strings.Contains(w.Body.String(), "assignment_already_closed") {
		t.Fatalf("wrong conflict code: %s", w.Body)
	}
}

// Close stays idempotent: the guard must not turn a repeated close into an error.
func TestCloseAssignmentStaysIdempotent(t *testing.T) {
	h := testutil.New(t)
	tenant := h.SeedTenant()
	h.SeedAdmin(tenant.ID, "owner@idem", models.PlatformOwner)
	session := loginAs(t, h, "owner@idem")

	assignmentID := queuedAssignment(t, h, tenant.ID)

	for i := 0; i < 2; i++ {
		w := h.Request("PATCH", "/v1/assignments/"+assignmentID).
			Cookie("sild_admin", session).JSON(map[string]any{"status": "closed"}).Do()
		if w.Code != http.StatusOK {
			t.Fatalf("close #%d: %d %s", i+1, w.Code, w.Body)
		}
	}
}
