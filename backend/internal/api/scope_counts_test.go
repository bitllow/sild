package api_test

import (
	"context"
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
		Items  []map[string]any `json:"items"`
		Counts struct {
			Open       int `json:"open"`
			You        int `json:"you"`
			Unassigned int `json:"unassigned"`
			Closed     int `json:"closed"`
		} `json:"counts"`
	}
	w := h.Request("GET", "/v1/conversations?kind=support&limit=50").Cookie("sild_admin", owner).Do()
	if w.Code != http.StatusOK {
		t.Fatalf("list: %d %s", w.Code, w.Body)
	}
	testutil.DecodeJSON(t, w, &resp)
	if resp.Counts.You != 1 || resp.Counts.Unassigned != 1 || resp.Counts.Closed != 1 {
		t.Fatalf("counts you=%d unassigned=%d closed=%d, want 1/1/1", resp.Counts.You, resp.Counts.Unassigned, resp.Counts.Closed)
	}

	// exclude_closed drops the closed assignment from the page (all three still
	// carry an assignment, so without it the page has 3).
	w = h.Request("GET", "/v1/conversations?kind=support&limit=50&status=open").Cookie("sild_admin", owner).Do()
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

	h.SeedContact(tenant.ID, "u_mari", `{"name":"Mari Tamm"}`)
	for _, ref := range []string{"trip_1", "trip_2"} {
		if _, err := h.Svc.CreateConversation(ctx, tenant.ID, domain.CreateConversationInput{
			Reference: ref, OpenAssignment: true,
			Members: []domain.MemberInput{{UserID: "u_mari", ConvRole: models.RoleClient}},
		}); err != nil {
			t.Fatalf("create %s: %v", ref, err)
		}
	}

	var resp struct {
		Items []map[string]any `json:"items"`
	}
	w := h.Request("GET", "/v1/conversations?participant=u_mari").Cookie("sild_admin", owner).Do()
	if w.Code != http.StatusOK {
		t.Fatalf("contact history: %d %s", w.Code, w.Body)
	}
	testutil.DecodeJSON(t, w, &resp)
	if len(resp.Items) != 2 {
		t.Fatalf("contact history = %d threads, want 2", len(resp.Items))
	}

	// Missing external_user_id is a 400.
	// Omitting the participant filter is no longer an error — it just widens the
	// list to the whole scope.
	w = h.Request("GET", "/v1/conversations").Cookie("sild_admin", owner).Do()
	if w.Code != http.StatusOK {
		t.Fatalf("unfiltered list = %d, want 200", w.Code)
	}
}

// Agent-authored messages carry the operator's first name (author_name) so the
// widget can render it in place of "Support".
func TestMessageCarriesAgentName(t *testing.T) {
	h := testutil.New(t)
	tenant := h.SeedTenant()
	admin, err := h.Svc.InviteAgent(context.Background(), tenant.ID, "eva@test", "Eva", "Marleen", models.PlatformOwner, models.RoleScope{})
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
