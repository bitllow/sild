package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/bitllow/sild/backend/internal/domain"
	"github.com/bitllow/sild/backend/internal/store/models"
	"github.com/bitllow/sild/backend/internal/testutil"
)

// The queue endpoint reports the three scope counters (assigned-to-me /
// unassigned / closed) alongside the page, and honors exclude_closed.
func TestListAssignmentsScopeCounts(t *testing.T) {
	h := testutil.New(t)
	tenant := h.SeedTenant()
	admin := h.SeedAdmin(tenant.ID, "owner@test", models.PlatformOwner)
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
	mk("unassigned") // stays queued
	mine := mk("mine")
	closed := mk("closed")

	// Claim `mine` as the logged-in admin → counts toward "you".
	if _, err := h.Svc.ClaimAssignment(ctx, tenant.ID, mine.Assignment.ID, admin.ID); err != nil {
		t.Fatalf("claim: %v", err)
	}
	// Close the `closed` CONVERSATION (the inbox "Close conversation" action) →
	// counts toward "closed", never toward you/unassigned even though its
	// assignment is still queued.
	if err := h.Svc.CloseConversation(ctx, tenant.ID, closed.ID); err != nil {
		t.Fatalf("close conversation: %v", err)
	}

	var resp struct {
		Items           []map[string]any `json:"items"`
		YouCount        int              `json:"you_count"`
		UnassignedCount int              `json:"unassigned_count"`
		ClosedCount     int              `json:"closed_count"`
	}
	w := h.Request("GET", "/v1/admin/assignments?limit=50").Cookie("sild_admin", owner).Do()
	if w.Code != http.StatusOK {
		t.Fatalf("list: %d %s", w.Code, w.Body)
	}
	testutil.DecodeJSON(t, w, &resp)
	if resp.YouCount != 1 || resp.UnassignedCount != 1 || resp.ClosedCount != 1 {
		t.Fatalf("counts you=%d unassigned=%d closed=%d, want 1/1/1", resp.YouCount, resp.UnassignedCount, resp.ClosedCount)
	}

	// exclude_closed drops the closed assignment from the page (all three still
	// carry an assignment, so without it the page has 3).
	w = h.Request("GET", "/v1/admin/assignments?limit=50&exclude_closed=true").Cookie("sild_admin", owner).Do()
	testutil.DecodeJSON(t, w, &resp)
	if len(resp.Items) != 2 {
		t.Fatalf("exclude_closed page = %d items, want 2", len(resp.Items))
	}
}

// The contact-history endpoint returns every thread a contact takes part in.
func TestListContactConversations(t *testing.T) {
	h := testutil.New(t)
	tenant := h.SeedTenant()
	h.SeedAdmin(tenant.ID, "owner@test", models.PlatformOwner)
	owner := loginAs(t, h, "owner@test")
	ctx := context.Background()

	meta := json.RawMessage(`{"name":"Mari Tamm"}`)
	for _, ref := range []string{"trip_1", "trip_2"} {
		if _, err := h.Svc.CreateConversation(ctx, tenant.ID, domain.CreateConversationInput{
			Reference: ref, OpenAssignment: true,
			Members: []domain.MemberInput{{UserID: "u_mari", ConvRole: models.RoleClient, Metadata: meta}},
		}); err != nil {
			t.Fatalf("create %s: %v", ref, err)
		}
	}

	var resp struct {
		Conversations []map[string]any `json:"conversations"`
	}
	w := h.Request("GET", "/v1/admin/contacts/conversations?external_user_id=u_mari").Cookie("sild_admin", owner).Do()
	if w.Code != http.StatusOK {
		t.Fatalf("contact history: %d %s", w.Code, w.Body)
	}
	testutil.DecodeJSON(t, w, &resp)
	if len(resp.Conversations) != 2 {
		t.Fatalf("contact history = %d threads, want 2", len(resp.Conversations))
	}

	// Missing external_user_id is a 400.
	w = h.Request("GET", "/v1/admin/contacts/conversations").Cookie("sild_admin", owner).Do()
	if w.Code != http.StatusBadRequest {
		t.Fatalf("missing external_user_id = %d, want 400", w.Code)
	}
}

// Agent-authored messages carry the operator's first name (author_name) so the
// widget can render it in place of "Support".
func TestMessageCarriesAgentName(t *testing.T) {
	h := testutil.New(t)
	tenant := h.SeedTenant()
	admin, err := h.Svc.InviteAgent(context.Background(), tenant.ID, "eva@test", "Eva", "Marleen", models.PlatformOwner)
	if err != nil {
		t.Fatalf("invite: %v", err)
	}
	ctx := context.Background()
	conv, err := h.Svc.CreateConversation(ctx, tenant.ID, domain.CreateConversationInput{
		Reference: "trip_x", OpenAssignment: true,
		Members: []domain.MemberInput{{UserID: "u_x", ConvRole: models.RoleClient}},
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	internal := admin.ID
	if _, err := h.Svc.SendMessage(ctx, tenant.ID, conv.ID, domain.SendInput{
		SenderKind: models.SenderAgent, Internal: &internal, Body: "hello there",
	}); err != nil {
		t.Fatalf("send: %v", err)
	}

	if got := h.Svc.AgentDisplayName(ctx, tenant.ID, admin.ID); got != "Eva" {
		t.Fatalf("AgentDisplayName = %q, want Eva", got)
	}
}
