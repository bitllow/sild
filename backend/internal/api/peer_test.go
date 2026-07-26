package api_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"

	"github.com/bitllow/sild/backend/internal/domain"
	"github.com/bitllow/sild/backend/internal/policy"
	"github.com/bitllow/sild/backend/internal/principal"
	"github.com/bitllow/sild/backend/internal/realtime"
	"github.com/bitllow/sild/backend/internal/store"
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
		Items  []map[string]any `json:"items"`
		Counts struct {
			Open       int `json:"open"`
			You        int `json:"you"`
			Unassigned int `json:"unassigned"`
			Closed     int `json:"closed"`
		} `json:"counts"`
	}
	w := h.Request("GET", "/v1/conversations?kind=support&limit=50").Cookie("sild_admin", owner).Do()
	testutil.DecodeJSON(t, w, &q)
	if len(q.Items) != 1 || q.Counts.Open != 1 {
		t.Fatalf("queue items=%d open_count=%d, want 1/1 (peer excluded)", len(q.Items), q.Counts.Open)
	}

	// Peer list: only the peer conversation.
	var p struct {
		Items []map[string]any `json:"items"`
	}
	w = h.Request("GET", "/v1/conversations?kind=peer").Cookie("sild_admin", owner).Do()
	if w.Code != http.StatusOK {
		t.Fatalf("peer list: %d %s", w.Code, w.Body)
	}
	testutil.DecodeJSON(t, w, &p)
	if len(p.Items) != 1 || p.Items[0]["id"] != peer.ID {
		t.Fatalf("peer list = %+v, want just %s", p.Items, peer.ID)
	}
	if len(p.Items[0]["members"].([]any)) != 2 {
		t.Fatalf("peer row should carry its 2 participants")
	}
}

// Peer access is per-user: a plain agent without it is denied the list and any
// peer conversation; granting it admits both. Owner/admin are not exempt — their
// tenant-wide scope covers support conversations only (TestPeerAccessGatesEveryRole).
func TestPeerAccessGating(t *testing.T) {
	h := testutil.New(t)
	tenant := h.SeedTenant()
	h.SeedAdmin(tenant.ID, "owner@test", models.PlatformOwner)
	agent := h.SeedAdmin(tenant.ID, "agent@test", models.PlatformAgent)
	ctx := context.Background()
	peer := mkPeer(t, h, tenant.ID, "trip_1")

	agentCookie := loginAs(t, h, "agent@test")

	// No peer access → the LIST returns an empty page (the scope is a ceiling, so
	// a filter it excludes yields nothing), while the named conversation 403s:
	// there the caller identified something specific and silence is worse.
	if w := h.Request("GET", "/v1/conversations?kind=peer").Cookie("sild_admin", agentCookie).Do(); w.Code != http.StatusOK {
		t.Fatalf("agent without peer access: list = %d, want 200", w.Code)
	} else {
		var page struct {
			Items []map[string]any `json:"items"`
		}
		testutil.DecodeJSON(t, w, &page)
		if len(page.Items) != 0 {
			t.Fatalf("peer rows leaked to an agent without peer access: %v", page.Items)
		}
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
	if w := h.Request("GET", "/v1/conversations?kind=peer").Cookie("sild_admin", agentCookie).Do(); w.Code != http.StatusOK {
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
		w := h.Request("POST", "/v1/conversations/"+peer.ID+"/messages").
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
			Messages []map[string]any `json:"items"`
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

// Peer search (GET /admin/search?peer=true, via the shared search backend) finds
// a peer conversation by a participant's id and by any metadata value, and never
// returns support (assignment-carrying) conversations.
func TestPeerSearchByIdAndMetadata(t *testing.T) {
	h := testutil.New(t)
	tenant := h.SeedTenant()
	ctx := context.Background()

	// A peer conversation whose rider has a distinctive id + metadata value.
	_, err := h.Svc.CreateConversation(ctx, tenant.ID, domain.CreateConversationInput{
		Reference: "trip_find", OpenAssignment: false,
		Members: []domain.MemberInput{
			{UserID: "p_zorro", ConvRole: models.ConvRole("rider"), Metadata: json.RawMessage(`{"name":"Zelda Xylophone","plan":"platinum"}`)},
			{UserID: "p_dd", ConvRole: models.ConvRole("driver"), Metadata: json.RawMessage(`{"name":"Dan Driver"}`)},
		},
	})
	if err != nil {
		t.Fatalf("create peer: %v", err)
	}
	// A support conversation that must NOT appear in peer search.
	mkSupport(t, h, tenant.ID, "support")

	search := func(q string, peerOnly bool) []string {
		scope, in := searchAs(peerOnly)
		in.Query = q
		res, err := h.Search.Search(ctx, tenant.ID, scope, in)
		if err != nil {
			t.Fatalf("search %q: %v", q, err)
		}
		ids := make([]string, 0, len(res.Conversations))
		for _, c := range res.Conversations {
			ids = append(ids, c.ConversationID)
		}
		return ids
	}

	if got := search("p_zorro", true); len(got) != 1 { // by participant id
		t.Fatalf("search by id: got %d hits, want 1 (%v)", len(got), got)
	}
	if got := search("platinum", true); len(got) != 1 { // by metadata value (no configured keys)
		t.Fatalf("search by metadata: got %d hits, want 1 (%v)", len(got), got)
	}
	// A term that only matches the support conversation returns nothing under peer scope.
	if got := search("u_support", true); len(got) != 0 {
		t.Fatalf("peer search leaked a support conversation: %v", got)
	}
	// And critically: the DEFAULT (non-peer) search must NOT surface the peer
	// conversation — otherwise a non-peer-access agent could recover peer content.
	if got := search("platinum", false); len(got) != 0 {
		t.Fatalf("default search leaked a peer conversation by metadata: %v", got)
	}
	if got := search("p_zorro", false); len(got) != 0 {
		t.Fatalf("default search leaked a peer conversation by id: %v", got)
	}
}

// Revoking peer access reconciles the operator's LIVE realtime subscription to
// the tenant peer channel, so a still-open inbox stops receiving peer
// publications without waiting for a reconnect. Granting subscribes them. It is
// a single channel — not one per peer conversation — so the number of broker
// calls does not grow with how many peer conversations exist.
func TestPeerAccessRevokeUnsubscribesRealtime(t *testing.T) {
	h := testutil.New(t)
	tenant := h.SeedTenant()
	agent := h.SeedAdmin(tenant.ID, "agent@test", models.PlatformAgent)
	ctx := context.Background()
	// Several peer conversations: the reconcile must still be exactly one call.
	mkPeer(t, h, tenant.ID, "trip_1")
	mkPeer(t, h, tenant.ID, "trip_2")
	mkPeer(t, h, tenant.ID, "trip_3")

	h.Pub.Reset()
	if err := h.Svc.SetPeerAccess(ctx, tenant.ID, agent.ID, true); err != nil {
		t.Fatalf("grant: %v", err)
	}
	wantChan := agent.ID + "→" + "peer:" + tenant.ID
	if len(h.Pub.Subscribed) != 1 || h.Pub.Subscribed[0] != wantChan {
		t.Fatalf("grant should subscribe once to %s, got %v", wantChan, h.Pub.Subscribed)
	}

	if err := h.Svc.SetPeerAccess(ctx, tenant.ID, agent.ID, false); err != nil {
		t.Fatalf("revoke: %v", err)
	}
	if len(h.Pub.Unsubscribed) != 1 || h.Pub.Unsubscribed[0] != wantChan {
		t.Fatalf("revoke should unsubscribe once from %s, got %v", wantChan, h.Pub.Unsubscribed)
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

	w := h.Request("PATCH", "/v1/team/"+agent.ID).
		Cookie("sild_admin", owner).JSON(map[string]any{"peer_access": true}).Do()
	if w.Code != http.StatusNoContent {
		t.Fatalf("toggle: %d %s", w.Code, w.Body)
	}

	var teamPage struct {
		Items []map[string]any `json:"items"`
	}
	w = h.Request("GET", "/v1/team").Cookie("sild_admin", owner).Do()
	testutil.DecodeJSON(t, w, &teamPage)
	found := false
	for _, m := range teamPage.Items {
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

	// GET /v1/principal reports peer visibility as the SCOPE the operator holds
	// conversations.list over, not a raw flag — so the inbox renders the peer
	// surface from the same decision the backend enforces.
	agentCookie := loginAs(t, h, "agent@test")
	var me struct {
		Grants []struct {
			Action string `json:"action"`
			Scope  *struct {
				Kinds []string `json:"kinds"`
			} `json:"scope"`
		} `json:"grants"`
	}
	w = h.Request("GET", "/v1/principal").Cookie("sild_admin", agentCookie).Do()
	testutil.DecodeJSON(t, w, &me)
	unrestricted := false
	for _, g := range me.Grants {
		if g.Action == "conversations.list" && g.Scope != nil && len(g.Scope.Kinds) == 0 {
			unrestricted = true
		}
	}
	if !unrestricted {
		t.Fatalf("peer_access operator should hold conversations.list unrestricted by kind, got %+v", me.Grants)
	}
}

// A peer conversation that has been closed is read-only: an operator can no
// longer implicitly join and post into it. Reading its history stays allowed
// (that gate is separate); only the write path enforces open status.
func TestPeerSendRejectedOnClosedConversation(t *testing.T) {
	h := testutil.New(t)
	tenant := h.SeedTenant()
	admin := h.SeedAdmin(tenant.ID, "owner@test", models.PlatformOwner)
	ctx := context.Background()
	peer := mkPeer(t, h, tenant.ID, "trip_1")

	if err := h.Svc.CloseConversation(ctx, tenant.ID, peer.ID); err != nil {
		t.Fatalf("close: %v", err)
	}
	_, err := h.Svc.PeerAgentSend(ctx, tenant.ID, peer.ID, admin.ID, "stepping in", nil, "")
	if !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("send into closed peer conversation: err = %v, want ErrForbidden", err)
	}
}

// A rejected peer send must not commit its attachment uploads. The uploads are
// finalized only after PeerAgentSend's guards pass, so a send rejected because
// the conversation is closed leaves the upload untouched (still pending) rather
// than orphaned in completed state with no message referencing it.
func TestPeerSendDoesNotCompleteUploadsWhenRejected(t *testing.T) {
	h := testutil.New(t)
	tenant := h.SeedTenant()
	admin := h.SeedAdmin(tenant.ID, "owner@test", models.PlatformOwner)
	ctx := context.Background()
	peer := mkPeer(t, h, tenant.ID, "trip_1")
	if err := h.Svc.CloseConversation(ctx, tenant.ID, peer.ID); err != nil {
		t.Fatalf("close: %v", err)
	}

	up, err := h.Svc.IssueUpload(ctx, tenant.ID, domain.IssueUploadInput{
		MimeType: "image/png", SizeBytes: 4, Filename: "a.png",
		Uploader: store.Participant{Kind: models.MemberAgent, InternalActorID: &admin.ID},
	})
	if err != nil {
		t.Fatalf("issue upload: %v", err)
	}

	_, err = h.Svc.PeerAgentSend(ctx, tenant.ID, peer.ID, admin.ID, "with attachment",
		[]domain.AttachmentInput{{ObjectKey: up.ObjectKey}}, "")
	if !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("send should be rejected: err = %v, want ErrForbidden", err)
	}

	stored, err := h.Store.Uploads().GetByObjectKey(ctx, tenant.ID, up.ObjectKey)
	if err != nil {
		t.Fatalf("load upload: %v", err)
	}
	if stored.Status == models.UploadCompleted {
		t.Fatalf("rejected send left an orphaned completed upload: %s", up.ObjectKey)
	}
}

// Peer-conversation events fan out to the tenant PEER channel (peer:<tenant>),
// which only peer_access operators observe — never the tenant agents channel
// (which every operator subscribes to). This covers both the new-conversation
// nudge and message.created, so an operator who connected before the
// conversation existed still receives its messages via the peer channel.
func TestPeerEventsFanOutToPeerChannelOnly(t *testing.T) {
	h := testutil.New(t)
	tenant := h.SeedTenant()
	ctx := context.Background()

	h.Pub.Reset()
	peer := mkPeer(t, h, tenant.ID, "trip_1")

	// The new-peer-conversation nudge targets the peer channel, not the tenant
	// agents channel (which would leak the id to non-peer operators).
	nudges := h.Pub.OfType(realtime.EventMemberAdded)
	if len(nudges) != 1 {
		t.Fatalf("want 1 member.added nudge on create, got %d", len(nudges))
	}
	if nudges[0].Target.Peer != tenant.ID || nudges[0].Target.Tenant != "" {
		t.Fatalf("create nudge target = %+v, want Peer=%s Tenant empty", nudges[0].Target, tenant.ID)
	}

	// An end-user message in the peer conversation reaches operators via the peer
	// channel (and the conv channel for the participants).
	h.Pub.Reset()
	rider := "u_rider_trip_1"
	if _, err := h.Svc.SendMessage(ctx, tenant.ID, peer.ID, domain.SendInput{
		SenderKind: models.SenderUser, External: &rider, Body: "hello",
	}); err != nil {
		t.Fatalf("send: %v", err)
	}
	msgs := h.Pub.OfType(realtime.EventMessageCreated)
	if len(msgs) != 1 {
		t.Fatalf("want 1 message.created, got %d", len(msgs))
	}
	if msgs[0].Target.Peer != tenant.ID {
		t.Fatalf("peer message target = %+v, want Peer=%s", msgs[0].Target, tenant.ID)
	}
	if msgs[0].Target.Conversation != peer.ID {
		t.Fatalf("peer message must still target the conv channel for participants, got %+v", msgs[0].Target)
	}
}

// A support (assignment-carrying) message must NOT fan out to the peer channel —
// peer_access operators observe only peer conversations.
func TestSupportMessageDoesNotFanOutToPeerChannel(t *testing.T) {
	h := testutil.New(t)
	tenant := h.SeedTenant()
	ctx := context.Background()
	conv := mkSupport(t, h, tenant.ID, "support")

	h.Pub.Reset()
	ext := "u_support"
	if _, err := h.Svc.SendMessage(ctx, tenant.ID, conv.ID, domain.SendInput{
		SenderKind: models.SenderUser, External: &ext, Body: "help",
	}); err != nil {
		t.Fatalf("send: %v", err)
	}
	msgs := h.Pub.OfType(realtime.EventMessageCreated)
	if len(msgs) != 1 || msgs[0].Target.Peer != "" {
		t.Fatalf("support message must not target the peer channel, got %+v", msgs)
	}
}

// A closed peer conversation (still assignment-less) must not surface in the
// default (non-peer) search — otherwise a non-peer-access operator could recover
// its content by closing it first, defeating the peer gating.
func TestClosedPeerConversationStaysOutOfSupportSearch(t *testing.T) {
	h := testutil.New(t)
	tenant := h.SeedTenant()
	ctx := context.Background()

	peer, err := h.Svc.CreateConversation(ctx, tenant.ID, domain.CreateConversationInput{
		Reference: "trip_secret", OpenAssignment: false,
		Members: []domain.MemberInput{
			{UserID: "p_needle", ConvRole: models.ConvRole("rider"), Metadata: json.RawMessage(`{"name":"Needle Haystack"}`)},
			{UserID: "p_drv", ConvRole: models.ConvRole("driver"), Metadata: json.RawMessage(`{"name":"Dee Driver"}`)},
		},
	})
	if err != nil {
		t.Fatalf("create peer: %v", err)
	}
	if err := h.Svc.CloseConversation(ctx, tenant.ID, peer.ID); err != nil {
		t.Fatalf("close: %v", err)
	}

	// Default (support) search must not return the now-closed peer conversation.
	scope, in := searchAs(false)
	in.Query = "p_needle"
	res, err := h.Search.Search(ctx, tenant.ID, scope, in)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(res.Conversations) != 0 {
		t.Fatalf("default search leaked a closed peer conversation: %v", res.Conversations)
	}

	// But the PEER surface's search must still surface it (open OR closed): reading
	// a closed peer conversation's history is authorized — only writing is gated on
	// open status — so it must not vanish from search the moment it closes.
	pscope, pin := searchAs(true)
	pin.Query = "p_needle"
	pres, err := h.Search.Search(ctx, tenant.ID, pscope, pin)
	if err != nil {
		t.Fatalf("peer search: %v", err)
	}
	if len(pres.Conversations) != 1 || pres.Conversations[0].ConversationID != peer.ID {
		t.Fatalf("peer search dropped the closed peer conversation: %v, want just %s", pres.Conversations, peer.ID)
	}
}

// A conversation's kind is set once at creation from whether an assignment is
// opened — support when it is, peer otherwise — and persisted as the durable
// classifier every surface reads (rather than re-derived from assignment
// presence, which is what allowed a closed peer conversation to leak).
func TestConversationKindSetAtCreation(t *testing.T) {
	h := testutil.New(t)
	tenant := h.SeedTenant()
	ctx := context.Background()

	peer := mkPeer(t, h, tenant.ID, "trip_1")
	support := mkSupport(t, h, tenant.ID, "support")

	got, err := h.Store.Conversations().Get(ctx, tenant.ID, peer.ID)
	if err != nil {
		t.Fatalf("load peer: %v", err)
	}
	if got.Kind != models.KindPeer {
		t.Fatalf("peer conversation kind = %q, want peer", got.Kind)
	}
	got, err = h.Store.Conversations().Get(ctx, tenant.ID, support.ID)
	if err != nil {
		t.Fatalf("load support: %v", err)
	}
	if got.Kind != models.KindSupport {
		t.Fatalf("support conversation kind = %q, want support", got.Kind)
	}
}

// A conversation's kind is fixed at creation and never re-derived, so a peer
// conversation can never be queued: adding an assignment is rejected. Otherwise
// it would carry an assignment while every kind-based path (queue count, default
// search, realtime fan-out) still treats it as peer — visible on the peer surface
// yet also in the raw queue.
func TestAddAssignmentRejectedOnPeerConversation(t *testing.T) {
	h := testutil.New(t)
	tenant := h.SeedTenant()
	ctx := context.Background()
	peer := mkPeer(t, h, tenant.ID, "trip_1")

	if _, err := h.Svc.AddAssignment(ctx, tenant.ID, peer.ID); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("add assignment to peer conversation: err = %v, want ErrForbidden", err)
	}

	// It stays a peer conversation with no assignment.
	got, err := h.Store.Conversations().Get(ctx, tenant.ID, peer.ID)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if got.Kind != models.KindPeer {
		t.Fatalf("conversation kind = %q, want peer (unchanged)", got.Kind)
	}
}

// searchAs builds the scope+kind pair for an operator with or without peer_access.
func searchAs(peerOnly bool) (policy.ResourceScope, domain.SearchInput) {
	p := &principal.Principal{TenantID: "t", Kind: principal.KindAdmin, AdminID: "a",
		Role: models.PlatformOwner, PeerAccess: peerOnly}
	in := domain.SearchInput{Limit: 25}
	if peerOnly {
		k := models.KindPeer
		in.Kind = &k
	}
	return policy.Scope(p, policy.ConversationsList), in
}
