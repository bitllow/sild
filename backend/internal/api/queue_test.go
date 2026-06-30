package api_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/bitllow/sild/backend/internal/domain"
	"github.com/bitllow/sild/backend/internal/store/models"
	"github.com/bitllow/sild/backend/internal/testutil"
)

// The inbox queue endpoint accepts the waiting_since sort and reports the
// tenant's open-conversation count (§8).
func TestListAssignmentsSortAndOpenCount(t *testing.T) {
	h := testutil.New(t)
	tenant := h.SeedTenant()
	h.SeedAdmin(tenant.ID, "owner@test", models.PlatformOwner)
	owner := loginAs(t, h, "owner@test")
	ctx := context.Background()

	mk := func(ref string) *models.Conversation {
		conv, err := h.Svc.CreateConversation(ctx, tenant.ID, domain.CreateConversationInput{
			Reference: ref, OpenAssignment: true,
			Members: []domain.MemberInput{{UserID: "u_" + ref, ConvRole: models.RoleClient}},
		})
		if err != nil {
			t.Fatalf("create %s: %v", ref, err)
		}
		return conv
	}
	mk("a")
	mk("b")
	closed := mk("c")
	if err := h.Svc.CloseConversation(ctx, tenant.ID, closed.ID); err != nil {
		t.Fatalf("close: %v", err)
	}

	var resp struct {
		Items     []map[string]any `json:"items"`
		OpenCount int              `json:"open_count"`
	}
	w := h.Request("GET", "/v1/admin/assignments?sort=waiting_since&order=asc&limit=50").
		Cookie("sild_admin", owner).Do()
	if w.Code != http.StatusOK {
		t.Fatalf("list: %d %s", w.Code, w.Body)
	}
	testutil.DecodeJSON(t, w, &resp)
	if resp.OpenCount != 2 {
		t.Fatalf("open_count = %d, want 2 (closed conversation excluded)", resp.OpenCount)
	}
	// All three conversations carry an assignment, so the queue lists all three.
	if len(resp.Items) != 3 {
		t.Fatalf("queue returned %d items, want 3", len(resp.Items))
	}
}
