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

// mkPeer creates a peer conversation (open, no assignment) between two users.
func mkPeer(t *testing.T, h *testutil.Harness, tenantID, ref string) *models.Conversation {
	t.Helper()
	conv, err := h.Svc.CreateConversation(context.Background(), tenantID, domain.CreateConversationInput{
		Reference: ref, OpenAssignment: false,
		Members: []domain.MemberInput{
			{UserID: "u_rider_" + ref, ConvRole: models.ConvRole("rider"), Metadata: json.RawMessage(`{"name":"Rider"}`)},
			{UserID: "u_driver_" + ref, ConvRole: models.ConvRole("driver"), Metadata: json.RawMessage(`{"name":"Driver"}`)},
		},
	})
	if err != nil {
		t.Fatalf("create peer %s: %v", ref, err)
	}
	return conv
}

func mkSupport(t *testing.T, h *testutil.Harness, tenantID, ref string) *models.Conversation {
	t.Helper()
	conv, err := h.Svc.CreateConversation(context.Background(), tenantID, domain.CreateConversationInput{
		Reference: ref, OpenAssignment: true,
		Members: []domain.MemberInput{{UserID: "u_" + ref, ConvRole: models.RoleClient}},
	})
	if err != nil {
		t.Fatalf("create support %s: %v", ref, err)
	}
	return conv
}

// A peer conversation is listed only by the dedicated peer endpoint — never in
// the assignment queue — and it does not inflate the open-conversation badge.
func TestPeerConversationsExcludedFromQueue(t *testing.T) {
	h := testutil.New(t)
	tenant := h.SeedTenant()
	admin := h.SeedAdmin(tenant.ID, "owner@test", models.PlatformOwner)
	if err := h.Svc.SetPeerAccess(context.Background(), tenant.ID, admin.ID, true); err != nil {
		t.Fatalf("peer access: %v", err)
	}
	owner := loginAs(t, h, "owner@test")

	mkSupport(t, h, tenant.ID, "support")
	peer := mkPeer(t, h, tenant.ID, "trip_1")

	// Queue: only the support conversation, open_count = 1 (peer excluded).
	var q struct {
		Items     []map[string]any `json:"items"`
		OpenCount int              `json:"open_count"`
	}
	w := h.Request("GET", "/v1/admin/assignments?limit=50").Cookie("sild_admin", owner).Do()
	testutil.DecodeJSON(t, w, &q)
	if len(q.Items) != 1 || q.OpenCount != 1 {
		t.Fatalf("queue items=%d open_count=%d, want 1/1 (peer excluded)", len(q.Items), q.OpenCount)
	}

	// Peer list: only the peer conversation.
	var p struct {
		Conversations []map[string]any `json:"conversations"`
	}
	w = h.Request("GET", "/v1/admin/peer-conversations").Cookie("sild_admin", owner).Do()
	if w.Code != http.StatusOK {
		t.Fatalf("peer list: %d %s", w.Code, w.Body)
	}
	testutil.DecodeJSON(t, w, &p)
	if len(p.Conversations) != 1 || p.Conversations[0]["id"] != peer.ID {
		t.Fatalf("peer list = %+v, want just %s", p.Conversations, peer.ID)
	}
	if len(p.Conversations[0]["members"].([]any)) != 2 {
		t.Fatalf("peer row should carry its 2 participants")
	}
}

// Peer access is per-user: a plain agent without it is denied the list and any
// peer conversation; granting it admits both. Owner/admin retain tenant-wide
// conversation access regardless, but the list endpoint is still their opt-in.
func TestPeerAccessGating(t *testing.T) {
	h := testutil.New(t)
	tenant := h.SeedTenant()
	h.SeedAdmin(tenant.ID, "owner@test", models.PlatformOwner)
	agent := h.SeedAdmin(tenant.ID, "agent@test", models.PlatformAgent)
	ctx := context.Background()
	peer := mkPeer(t, h, tenant.ID, "trip_1")

	agentCookie := loginAs(t, h, "agent@test")

	// No peer access → 403 on list and on the peer conversation itself.
	if w := h.Request("GET", "/v1/admin/peer-conversations").Cookie("sild_admin", agentCookie).Do(); w.Code != http.StatusForbidden {
		t.Fatalf("agent without peer access: list = %d, want 403", w.Code)
	}
	if w := h.Request("GET", "/v1/conversations/"+peer.ID+"/messages").Cookie("sild_admin", agentCookie).Do(); w.Code != http.StatusForbidden {
		t.Fatalf("agent without peer access: peer messages = %d, want 403", w.Code)
	}

	// Grant it → list + peer conversation now allowed. (Re-login: peer_access is
	// resolved onto the principal at session load.)
	if err := h.Svc.SetPeerAccess(ctx, tenant.ID, agent.ID, true); err != nil {
		t.Fatalf("grant: %v", err)
	}
	agentCookie = loginAs(t, h, "agent@test")
	if w := h.Request("GET", "/v1/admin/peer-conversations").Cookie("sild_admin", agentCookie).Do(); w.Code != http.StatusOK {
		t.Fatalf("agent with peer access: list = %d, want 200", w.Code)
	}
	if w := h.Request("GET", "/v1/conversations/"+peer.ID+"/messages").Cookie("sild_admin", agentCookie).Do(); w.Code != http.StatusOK {
		t.Fatalf("agent with peer access: peer messages = %d, want 200", w.Code)
	}
}

// The first agent message into a peer conversation implicitly joins the operator:
// an agent participant is added and a system join-note is appended before the
// message. Subsequent messages don't repeat the join.
func TestPeerImplicitJoin(t *testing.T) {
	h := testutil.New(t)
	tenant := h.SeedTenant()
	admin := h.SeedAdmin(tenant.ID, "owner@test", models.PlatformOwner)
	ctx := context.Background()
	if err := h.Svc.SetPeerAccess(ctx, tenant.ID, admin.ID, true); err != nil {
		t.Fatalf("peer access: %v", err)
	}
	owner := loginAs(t, h, "owner@test")
	peer := mkPeer(t, h, tenant.ID, "trip_1")

	post := func(body string) {
		w := h.Request("POST", "/v1/admin/peer-conversations/"+peer.ID+"/messages").
			Cookie("sild_admin", owner).JSON(map[string]any{"body": body}).Do()
		if w.Code != http.StatusCreated {
			t.Fatalf("peer send: %d %s", w.Code, w.Body)
		}
	}
	post("Stepping in to help.")

	// Members now include the agent (member_kind=agent).
	var conv struct {
		Members []map[string]any `json:"members"`
	}
	w := h.Request("GET", "/v1/conversations/"+peer.ID).Cookie("sild_admin", owner).Do()
	testutil.DecodeJSON(t, w, &conv)
	agents := 0
	for _, m := range conv.Members {
		if m["member_kind"] == string(models.MemberAgent) {
			agents++
		}
	}
	if len(conv.Members) != 3 || agents != 1 {
		t.Fatalf("after join: members=%d agents=%d, want 3/1", len(conv.Members), agents)
	}

	// Messages: one system join-note + the agent message.
	countKinds := func() (system, agent int) {
		var msgs struct {
			Messages []map[string]any `json:"messages"`
		}
		w := h.Request("GET", "/v1/conversations/"+peer.ID+"/messages?limit=50").Cookie("sild_admin", owner).Do()
		testutil.DecodeJSON(t, w, &msgs)
		for _, m := range msgs.Messages {
			switch m["sender_kind"] {
			case string(models.SenderSystem):
				system++
			case string(models.SenderAgent):
				agent++
			}
		}
		return
	}
	if sys, ag := countKinds(); sys != 1 || ag != 1 {
		t.Fatalf("after first send: system=%d agent=%d, want 1/1", sys, ag)
	}

	// A second message must not add another join-note or another agent member.
	post("One more thing.")
	if sys, ag := countKinds(); sys != 1 || ag != 2 {
		t.Fatalf("after second send: system=%d agent=%d, want 1/2 (no repeat join)", sys, ag)
	}
	w = h.Request("GET", "/v1/conversations/"+peer.ID).Cookie("sild_admin", owner).Do()
	testutil.DecodeJSON(t, w, &conv)
	if len(conv.Members) != 3 {
		t.Fatalf("second send added a member: members=%d, want 3", len(conv.Members))
	}
}

// The per-user peer_access flag persists through the team PATCH and surfaces in
// both the team list and /admin/me.
func TestPeerAccessTogglePersists(t *testing.T) {
	h := testutil.New(t)
	tenant := h.SeedTenant()
	h.SeedAdmin(tenant.ID, "owner@test", models.PlatformOwner)
	agent := h.SeedAdmin(tenant.ID, "agent@test", models.PlatformAgent)
	owner := loginAs(t, h, "owner@test")

	w := h.Request("PATCH", "/v1/admin/team/"+agent.ID).
		Cookie("sild_admin", owner).JSON(map[string]any{"peer_access": true}).Do()
	if w.Code != http.StatusNoContent {
		t.Fatalf("toggle: %d %s", w.Code, w.Body)
	}

	var team []map[string]any
	w = h.Request("GET", "/v1/admin/team").Cookie("sild_admin", owner).Do()
	testutil.DecodeJSON(t, w, &team)
	found := false
	for _, m := range team {
		if m["id"] == agent.ID {
			found = true
			if m["peer_access"] != true {
				t.Fatalf("team list peer_access = %v, want true", m["peer_access"])
			}
		}
	}
	if !found {
		t.Fatalf("agent missing from team list")
	}

	// /admin/me reflects the signed-in operator's own flag.
	agentCookie := loginAs(t, h, "agent@test")
	var me map[string]any
	w = h.Request("GET", "/v1/admin/me").Cookie("sild_admin", agentCookie).Do()
	testutil.DecodeJSON(t, w, &me)
	if me["peer_access"] != true {
		t.Fatalf("/admin/me peer_access = %v, want true", me["peer_access"])
	}
}
