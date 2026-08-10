package api_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/bitllow/sild/backend/internal/store/models"
	"github.com/bitllow/sild/backend/internal/testutil"
)

// Peer access is a per-operator opt-in for EVERY role: an owner or admin's
// tenant-wide scope covers support conversations, not peer ones.
func TestPeerAccessGatesEveryRole(t *testing.T) {
	h := testutil.New(t)
	tenant := h.SeedTenant()
	h.SeedAdmin(tenant.ID, "owner@test", models.PlatformOwner)
	admin := h.SeedAdmin(tenant.ID, "admin@test", models.PlatformAdmin)
	ctx := context.Background()

	peer := mkPeer(t, h, tenant.ID, "trip_roles")
	support := mkSupport(t, h, tenant.ID, "support_roles")

	for _, tc := range []struct{ who, email string }{
		{"owner", "owner@test"},
		{"admin", "admin@test"},
	} {
		cookie := loginAs(t, h, tc.email)
		for _, req := range []struct {
			method, path string
		}{
			{"GET", "/v1/conversations/" + peer.ID},
			{"GET", "/v1/conversations/" + peer.ID + "/messages"},
			{"POST", "/v1/conversations/" + peer.ID + "/close"},
			{"POST", "/v1/conversations/" + peer.ID + "/read"},
			{"POST", "/v1/conversations/" + peer.ID + "/typing"},
			{"POST", "/v1/conversations/" + peer.ID + "/messages"},
		} {
			w := h.Request(req.method, req.path).Cookie("sild_admin", cookie).JSON(map[string]any{}).Do()
			if w.Code != http.StatusForbidden {
				t.Fatalf("%s without peer access: %s %s = %d %s, want 403", tc.who, req.method, req.path, w.Code, w.Body)
			}
		}
		// The close was rejected, not applied.
		if conv, err := h.Store.Conversations().Get(ctx, tenant.ID, peer.ID); err != nil || conv.Status != models.ConversationOpen {
			t.Fatalf("%s: peer conversation must still be open (status=%v err=%v)", tc.who, conv.Status, err)
		}
		// Support conversations are unaffected.
		if w := h.Request("GET", "/v1/conversations/"+support.ID).Cookie("sild_admin", cookie).Do(); w.Code != http.StatusOK {
			t.Fatalf("%s support read = %d %s, want 200", tc.who, w.Code, w.Body)
		}
	}

	// Re-login: peer_access is resolved onto the principal at session load.
	h.GrantPeer(tenant.ID, admin.ID)
	adminCookie := loginAs(t, h, "admin@test")
	if w := h.Request("GET", "/v1/conversations/"+peer.ID).Cookie("sild_admin", adminCookie).Do(); w.Code != http.StatusOK {
		t.Fatalf("admin with peer access: peer read = %d %s, want 200", w.Code, w.Body)
	}
}

// An admin manages the team but must not hand peer history to themselves or a
// colleague — that would make withholding it meaningless.
func TestOnlyOwnerMayGrantPeerAccess(t *testing.T) {
	h := testutil.New(t)
	tenant := h.SeedTenant()
	h.SeedAdmin(tenant.ID, "owner@test", models.PlatformOwner)
	admin := h.SeedAdmin(tenant.ID, "admin@test", models.PlatformAdmin)
	agent := h.SeedAdmin(tenant.ID, "agent@test", models.PlatformAgent)
	adminCookie := loginAs(t, h, "admin@test")

	for _, target := range []struct {
		who, id, method, path string
	}{
		{"self", admin.ID, "POST", "/v1/team/" + admin.ID + "/roles"},
		{"a colleague", agent.ID, "PUT", "/v1/team/" + agent.ID + "/roles/agent"},
	} {
		body := map[string]any{"scope": map[string]any{"peer": true}}
		if target.method == "POST" {
			body["role"] = models.PlatformAgent
		}
		w := h.Request(target.method, target.path).Cookie("sild_admin", adminCookie).JSON(body).Do()
		if w.Code != http.StatusForbidden {
			t.Fatalf("admin granting peer access to %s = %d %s, want 403", target.who, w.Code, w.Body)
		}
	}
	// Nothing was persisted by the rejected attempts.
	for _, id := range []string{admin.ID, agent.ID} {
		if peerAccess(t, h, tenant.ID, id) {
			t.Fatalf("peer access must still be off for %s", id)
		}
	}

	// Roles are still an admin's to hand out.
	w := h.Request("POST", "/v1/team/"+agent.ID+"/roles").
		Cookie("sild_admin", adminCookie).JSON(map[string]any{"role": models.PlatformAdmin}).Do()
	if w.Code != http.StatusNoContent {
		t.Fatalf("admin granting a role = %d %s, want 204", w.Code, w.Body)
	}

	// The agent already holds the role, so the owner rescopes it rather than adding.
	owner := loginAs(t, h, "owner@test")
	w = h.Request("PUT", "/v1/team/"+agent.ID+"/roles/agent").
		Cookie("sild_admin", owner).JSON(map[string]any{"scope": map[string]any{"peer": true}}).Do()
	if w.Code != http.StatusNoContent {
		t.Fatalf("owner granting peer access = %d %s, want 204", w.Code, w.Body)
	}
	if !peerAccess(t, h, tenant.ID, agent.ID) {
		t.Fatal("peer access should be on after the owner's grant")
	}
}

// peerAccess reports whether a member's agent assignment reaches peer conversations.
func peerAccess(t *testing.T, h *testutil.Harness, tenantID, adminID string) bool {
	t.Helper()
	held, err := h.Svc.RoleAssignments(context.Background(), tenantID, adminID)
	if err != nil {
		t.Fatalf("assignments for %s: %v", adminID, err)
	}
	for _, a := range held {
		if a.Role == models.PlatformAgent && a.Scope.Peer {
			return true
		}
	}
	return false
}

// The owner-only grant is worthless unless the owner ROLE is equally protected. This
// walks the whole bypass — promote self, re-login, self-grant, read — not just the
// direct flip, which passes either way.
func TestAdminCannotEscalateToOwnerForPeerAccess(t *testing.T) {
	h := testutil.New(t)
	tenant := h.SeedTenant()
	ownerRec := h.SeedAdmin(tenant.ID, "owner@test", models.PlatformOwner)
	admin := h.SeedAdmin(tenant.ID, "admin@test", models.PlatformAdmin)
	ctx := context.Background()
	peer := mkPeer(t, h, tenant.ID, "trip_esc")
	adminCookie := loginAs(t, h, "admin@test")

	// Self-promotion is refused.
	w := h.Request("POST", "/v1/team/"+admin.ID+"/roles").
		Cookie("sild_admin", adminCookie).JSON(map[string]any{"role": "owner"}).Do()
	if w.Code != http.StatusForbidden {
		t.Fatalf("admin self-promoting to owner = %d %s, want 403", w.Code, w.Body)
	}
	// Nor promoting a colleague.
	agent := h.SeedAdmin(tenant.ID, "agent@test", models.PlatformAgent)
	w = h.Request("POST", "/v1/team/"+agent.ID+"/roles").
		Cookie("sild_admin", adminCookie).JSON(map[string]any{"role": "owner"}).Do()
	if w.Code != http.StatusForbidden {
		t.Fatalf("admin appointing another owner = %d %s, want 403", w.Code, w.Body)
	}
	// Nor taking the real owner's role away.
	w = h.Request("DELETE", "/v1/team/"+ownerRec.ID+"/roles/owner").
		Cookie("sild_admin", adminCookie).Do()
	if w.Code != http.StatusForbidden {
		t.Fatalf("admin demoting the owner = %d %s, want 403", w.Code, w.Body)
	}

	// Nothing moved: still an admin, still no peer access.
	held, err := h.Svc.RoleAssignments(ctx, tenant.ID, admin.ID)
	if err != nil {
		t.Fatalf("reload admin: %v", err)
	}
	if len(held) != 1 || held[0].Role != models.PlatformAdmin {
		t.Fatalf("admin assignments must be untouched, got %v", held)
	}
	// A fresh session is still an admin without peer access.
	adminCookie = loginAs(t, h, "admin@test")
	if w := h.Request("GET", "/v1/conversations/"+peer.ID+"/messages").Cookie("sild_admin", adminCookie).Do(); w.Code != http.StatusForbidden {
		t.Fatalf("peer history must stay closed, got %d %s", w.Code, w.Body)
	}
}

// Every route that can create or take over an owner, in one place — gating them one
// at a time is how a bypass survives. Each case is a complete escalation on its own.
func TestOwnerAccountIsProtectedOnEveryTeamRoute(t *testing.T) {
	h := testutil.New(t)
	tenant := h.SeedTenant()
	owner := h.SeedAdmin(tenant.ID, "owner@test", models.PlatformOwner)
	h.SeedAdmin(tenant.ID, "admin@test", models.PlatformAdmin)
	agent := h.SeedAdmin(tenant.ID, "agent@test", models.PlatformAgent)
	ctx := context.Background()
	if err := h.Svc.SetAdminPassword(ctx, tenant.ID, owner.ID, "owner-real-password"); err != nil {
		t.Fatalf("seed owner password: %v", err)
	}
	adminCookie := loginAs(t, h, "admin@test")

	// POST /team — no minting an owner you control.
	w := h.Request("POST", "/v1/team").Cookie("sild_admin", adminCookie).
		JSON(map[string]any{"email": "attacker@test", "role": "owner"}).Do()
	if w.Code != http.StatusForbidden {
		t.Fatalf("admin inviting an owner = %d %s, want 403", w.Code, w.Body)
	}
	for _, a := range mustListAdmins(t, h, tenant.ID) {
		if a.Email == "attacker@test" {
			t.Fatal("the rejected invite must not have created an account")
		}
	}

	// POST /team/:id/password — no taking over the owner's login.
	w = h.Request("POST", "/v1/team/"+owner.ID+"/password").Cookie("sild_admin", adminCookie).
		JSON(map[string]any{"password": "attacker-chosen-password"}).Do()
	if w.Code != http.StatusForbidden {
		t.Fatalf("admin resetting the owner's password = %d %s, want 403", w.Code, w.Body)
	}
	w = h.Request("POST", "/v1/admin/auth/password").
		JSON(map[string]any{"email": "owner@test", "password": "attacker-chosen-password"}).Do()
	if w.Code == http.StatusOK {
		t.Fatal("the owner's password was changed — takeover succeeded")
	}
	if w := h.Request("POST", "/v1/admin/auth/password").
		JSON(map[string]any{"email": "owner@test", "password": "owner-real-password"}).Do(); w.Code != http.StatusOK {
		t.Fatalf("the owner's own password must still work, got %d %s", w.Code, w.Body)
	}

	// Legitimate team management still works.
	if w := h.Request("POST", "/v1/team").Cookie("sild_admin", adminCookie).
		JSON(map[string]any{"email": "helper@test", "role": "agent"}).Do(); w.Code != http.StatusCreated {
		t.Fatalf("admin inviting an agent = %d %s, want 201", w.Code, w.Body)
	}
	if w := h.Request("POST", "/v1/team/"+agent.ID+"/password").Cookie("sild_admin", adminCookie).
		JSON(map[string]any{"password": "a-fine-password"}).Do(); w.Code != http.StatusNoContent {
		t.Fatalf("admin resetting an agent's password = %d %s, want 204", w.Code, w.Body)
	}

	// The owner may do all of it.
	ownerCookie := loginAs(t, h, "owner@test")
	if w := h.Request("POST", "/v1/team").Cookie("sild_admin", ownerCookie).
		JSON(map[string]any{"email": "co-owner@test", "role": "owner"}).Do(); w.Code != http.StatusCreated {
		t.Fatalf("owner inviting an owner = %d %s, want 201", w.Code, w.Body)
	}
	if w := h.Request("POST", "/v1/team/"+owner.ID+"/password").Cookie("sild_admin", ownerCookie).
		JSON(map[string]any{"password": "owner-new-password"}).Do(); w.Code != http.StatusNoContent {
		t.Fatalf("owner resetting their own password = %d %s, want 204", w.Code, w.Body)
	}
}

func mustListAdmins(t *testing.T, h *testutil.Harness, tenantID string) []models.AdminUser {
	t.Helper()
	admins, err := h.Store.Admins().List(context.Background(), tenantID)
	if err != nil {
		t.Fatalf("list admins: %v", err)
	}
	return admins
}

// Only owners can grant peer access or appoint owners, so demoting the last one
// would lock the tenant out with nobody left to undo it.
func TestLastOwnerCannotBeDemoted(t *testing.T) {
	h := testutil.New(t)
	tenant := h.SeedTenant()
	owner := h.SeedAdmin(tenant.ID, "owner@test", models.PlatformOwner)
	ctx := context.Background()
	cookie := loginAs(t, h, "owner@test")

	w := h.Request("DELETE", "/v1/team/"+owner.ID+"/roles/owner").Cookie("sild_admin", cookie).Do()
	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("sole owner stepping down = %d %s, want 422", w.Code, w.Body)
	}
	held, err := h.Svc.RoleAssignments(ctx, tenant.ID, owner.ID)
	if err != nil || len(held) == 0 || held[0].Role != models.PlatformOwner {
		t.Fatalf("owner must still be owner (err=%v, held=%v)", err, held)
	}

	// With a co-owner, stepping down is allowed.
	second := h.SeedAdmin(tenant.ID, "second@test", models.PlatformAgent)
	if w := h.Request("POST", "/v1/team/"+second.ID+"/roles").
		Cookie("sild_admin", cookie).JSON(map[string]any{"role": "owner"}).Do(); w.Code != http.StatusNoContent {
		t.Fatalf("owner appointing a second owner = %d %s, want 204", w.Code, w.Body)
	}
	if w := h.Request("DELETE", "/v1/team/"+owner.ID+"/roles/owner").
		Cookie("sild_admin", cookie).Do(); w.Code != http.StatusNoContent {
		t.Fatalf("owner stepping down with a co-owner = %d %s, want 204", w.Code, w.Body)
	}
}

// Closing needs the same scope check as reading: being *an* agent used to be enough
// to close any conversation by id.
func TestCloseRequiresConversationAccess(t *testing.T) {
	h := testutil.New(t)
	tenant := h.SeedTenant()
	h.SeedAdmin(tenant.ID, "owner@test", models.PlatformOwner)
	agent := h.SeedAdmin(tenant.ID, "agent@test", models.PlatformAgent)
	ctx := context.Background()

	peer := mkPeer(t, h, tenant.ID, "trip_close")
	support := mkSupport(t, h, tenant.ID, "support_close")
	agentCookie := loginAs(t, h, "agent@test")

	if w := h.Request("POST", "/v1/conversations/"+peer.ID+"/close").Cookie("sild_admin", agentCookie).Do(); w.Code != http.StatusForbidden {
		t.Fatalf("agent without peer access closing a peer conversation: %d %s, want 403", w.Code, w.Body)
	}
	if conv, err := h.Store.Conversations().Get(ctx, tenant.ID, peer.ID); err != nil || conv.Status != models.ConversationOpen {
		t.Fatalf("peer conversation must still be open (status=%v err=%v)", conv.Status, err)
	}

	// The support flow is untouched.
	if w := h.Request("POST", "/v1/conversations/"+support.ID+"/close").Cookie("sild_admin", agentCookie).Do(); w.Code != http.StatusOK {
		t.Fatalf("agent closing a support conversation: %d %s, want 200", w.Code, w.Body)
	}

	h.GrantPeer(tenant.ID, agent.ID)
	agentCookie = loginAs(t, h, "agent@test")
	if w := h.Request("POST", "/v1/conversations/"+peer.ID+"/close").Cookie("sild_admin", agentCookie).Do(); w.Code != http.StatusOK {
		t.Fatalf("agent with peer access closing a peer conversation: %d %s, want 200", w.Code, w.Body)
	}

	// A support conversation on purpose — peer access is another test's subject.
	owner := loginAs(t, h, "owner@test")
	ownerTarget := mkSupport(t, h, tenant.ID, "support_close_owner")
	if w := h.Request("POST", "/v1/conversations/"+ownerTarget.ID+"/close").Cookie("sild_admin", owner).Do(); w.Code != http.StatusOK {
		t.Fatalf("owner close: %d %s, want 200", w.Code, w.Body)
	}
}

// An operator sending into a peer conversation implicitly joins it. That is a
// property of the data — operator + peer conversation — not of the URL, so it
// fires on the shared send route rather than a separate one.
func TestSharedSendImplicitlyJoinsOperator(t *testing.T) {
	h := testutil.New(t)
	tenant := h.SeedTenant()
	admin := h.SeedAdmin(tenant.ID, "owner@test", models.PlatformOwner)
	ctx := context.Background()
	h.GrantPeer(tenant.ID, admin.ID)
	owner := loginAs(t, h, "owner@test")
	peer := mkPeer(t, h, tenant.ID, "trip_send")

	w := h.Request("POST", "/v1/conversations/"+peer.ID+"/messages").
		Cookie("sild_admin", owner).JSON(map[string]any{"body": "stepping in"}).Do()
	if w.Code != http.StatusCreated {
		t.Fatalf("operator peer write on the shared route: %d %s, want 201", w.Code, w.Body)
	}
	page, err := h.Store.Messages().ListBefore(ctx, tenant.ID, peer.ID, "", 50, true)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	// The agent message plus the system join-note.
	if len(page.Messages) != 2 {
		t.Fatalf("expected the message and a join note, got %d", len(page.Messages))
	}
	members, err := h.Store.Members().ListActive(ctx, tenant.ID, peer.ID)
	if err != nil {
		t.Fatalf("members: %v", err)
	}
	joined := false
	for i := range members {
		if members[i].MemberKind == models.MemberAgent {
			joined = true
		}
	}
	if !joined {
		t.Fatal("the operator should have been joined as an agent participant")
	}

	// Support conversations are unaffected.
	support := mkSupport(t, h, tenant.ID, "support_send")
	w = h.Request("POST", "/v1/conversations/"+support.ID+"/messages").
		Cookie("sild_admin", owner).JSON(map[string]any{"body": "on it"}).Do()
	if w.Code != http.StatusCreated {
		t.Fatalf("support send on the shared route: %d %s, want 201", w.Code, w.Body)
	}
}

// The read-only rule held only on the dedicated route, so a party could keep writing
// past a close via the shared one. Support conversations are unchanged.
func TestClosedPeerConversationIsReadOnly(t *testing.T) {
	h := testutil.New(t)
	tenant := h.SeedTenant()
	ctx := context.Background()
	peer := mkPeer(t, h, tenant.ID, "trip_closed")
	rider := "u_rider_trip_closed"
	tok := h.MintToken(tenant.ID, rider)

	w := h.Request("POST", "/v1/conversations/"+peer.ID+"/messages").
		Bearer(tok).JSON(map[string]any{"body": "before"}).Do()
	if w.Code != http.StatusCreated {
		t.Fatalf("send while open: %d %s, want 201", w.Code, w.Body)
	}

	if err := h.Svc.CloseConversation(ctx, tenant.ID, peer.ID); err != nil {
		t.Fatalf("close: %v", err)
	}

	w = h.Request("POST", "/v1/conversations/"+peer.ID+"/messages").
		Bearer(tok).JSON(map[string]any{"body": "after"}).Do()
	if w.Code != http.StatusForbidden {
		t.Fatalf("send into a closed peer conversation: %d %s, want 403", w.Code, w.Body)
	}
}
