package api_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/bitllow/sild/backend/internal/archive"
	"github.com/bitllow/sild/backend/internal/domain"
	"github.com/bitllow/sild/backend/internal/store"
	"github.com/bitllow/sild/backend/internal/store/models"
	"github.com/bitllow/sild/backend/internal/testutil"
)

// Review fix: the blanket "archived conversations are readable" grant in
// AuthorizeConversation must NOT bypass peer gating. Once a peer conversation is
// closed and archived (hot row purged), a plain agent WITHOUT peer access must
// still be denied its history; the tombstone preserves the kind so the boundary
// holds after the kind column is gone. Granting peer access admits the read.
func TestArchivedPeerConversationStillGatedOnPeerAccess(t *testing.T) {
	h := testutil.New(t)
	tenant := h.SeedTenant()
	h.SeedAdmin(tenant.ID, "owner@test", models.PlatformOwner)
	agent := h.SeedAdmin(tenant.ID, "agent@test", models.PlatformAgent)
	ctx := context.Background()

	peer := mkPeer(t, h, tenant.ID, "trip_1")
	if err := h.Svc.CloseConversation(ctx, tenant.ID, peer.ID); err != nil {
		t.Fatalf("close: %v", err)
	}
	// Archive it (idle clock far in the future so it's eligible) — purges the hot
	// conversation row, leaving only the tombstone (which now carries the kind).
	sink, _ := archive.New(h.Cfg)
	job := archive.NewJob(h.Store, sink, h.Cfg)
	job.SetClock(func() time.Time { return time.Now().Add(60 * 24 * time.Hour) })
	if n, err := job.RunOnce(ctx, tenant.ID, 100); err != nil || n != 1 {
		t.Fatalf("archive: n=%d err=%v", n, err)
	}

	// Plain agent, no peer access → 403 on the archived peer conversation.
	agentCookie := loginAs(t, h, "agent@test")
	if w := h.Request("GET", "/v1/conversations/"+peer.ID+"/messages").Cookie("sild_admin", agentCookie).Do(); w.Code != http.StatusForbidden {
		t.Fatalf("archived peer read without peer access = %d, want 403 (bypass!)", w.Code)
	}

	// Grant peer access, re-login (resolved onto the principal at session load) →
	// the archived read is now admitted and falls back to the sink.
	if err := h.Svc.SetPeerAccess(ctx, tenant.ID, agent.ID, true); err != nil {
		t.Fatalf("grant: %v", err)
	}
	agentCookie = loginAs(t, h, "agent@test")
	if w := h.Request("GET", "/v1/conversations/"+peer.ID+"/messages").Cookie("sild_admin", agentCookie).Do(); w.Code != http.StatusOK {
		t.Fatalf("archived peer read with peer access = %d, want 200", w.Code)
	}
}

// Review fix: an implicit join is user-visible (a "joined to help" note webhooked
// and emailed to both parties), so an empty submit must be rejected BEFORE it
// fires — no member added, no join note, no empty message bubble.
func TestPeerEmptySendRejectedWithoutJoin(t *testing.T) {
	h := testutil.New(t)
	tenant := h.SeedTenant()
	admin := h.SeedAdmin(tenant.ID, "owner@test", models.PlatformOwner)
	ctx := context.Background()
	peer := mkPeer(t, h, tenant.ID, "trip_1")

	if _, err := h.Svc.PeerAgentSend(ctx, tenant.ID, peer.ID, admin.ID, "   ", nil, ""); err == nil {
		t.Fatalf("empty send: err = nil, want a validation error")
	}

	members, err := h.Store.Members().ListActive(ctx, tenant.ID, peer.ID)
	if err != nil {
		t.Fatalf("list members: %v", err)
	}
	if len(members) != 2 {
		t.Fatalf("empty send added a member (phantom join): members = %d, want 2", len(members))
	}
	page, err := h.Svc.ListMessagesBefore(ctx, tenant.ID, peer.ID, "", 50, true)
	if err != nil {
		t.Fatalf("list messages: %v", err)
	}
	if len(page.Messages) != 0 {
		t.Fatalf("empty send created messages (incl. join note): %d, want 0", len(page.Messages))
	}
}

// Review fix: a send whose attachment key is unknown must be rejected BEFORE the
// implicit join and BEFORE completing any upload — otherwise a bad key leaves a
// phantom join (member + broadcast note) and an orphaned completed upload.
func TestPeerSendBadAttachmentDoesNotJoinOrOrphan(t *testing.T) {
	h := testutil.New(t)
	tenant := h.SeedTenant()
	admin := h.SeedAdmin(tenant.ID, "owner@test", models.PlatformOwner)
	ctx := context.Background()
	peer := mkPeer(t, h, tenant.ID, "trip_1")

	// A valid upload paired with a bad object_key in the same send.
	up, err := h.Svc.IssueUpload(ctx, tenant.ID, domain.IssueUploadInput{
		MimeType: "image/png", SizeBytes: 4, Filename: "a.png",
		Uploader: store.Participant{Kind: models.MemberAgent, InternalActorID: &admin.ID},
	})
	if err != nil {
		t.Fatalf("issue upload: %v", err)
	}

	if _, err := h.Svc.PeerAgentSend(ctx, tenant.ID, peer.ID, admin.ID, "with attachment",
		[]domain.AttachmentInput{{ObjectKey: up.ObjectKey}, {ObjectKey: "nonexistent_key"}}, ""); err == nil {
		t.Fatalf("send with unknown attachment: err = nil, want error")
	}

	// No implicit join: validation ran before it.
	members, err := h.Store.Members().ListActive(ctx, tenant.ID, peer.ID)
	if err != nil {
		t.Fatalf("list members: %v", err)
	}
	if len(members) != 2 {
		t.Fatalf("bad-attachment send joined the agent (phantom join): members = %d, want 2", len(members))
	}
	// The good upload was NOT committed — no orphaned completed upload.
	stored, err := h.Store.Uploads().GetByObjectKey(ctx, tenant.ID, up.ObjectKey)
	if err != nil {
		t.Fatalf("load upload: %v", err)
	}
	if stored.Status == models.UploadCompleted {
		t.Fatalf("rejected send left an orphaned completed upload: %s", up.ObjectKey)
	}
}

// Review fix: the implicit join is idempotent under concurrent/duplicate first
// sends. agentJoinPeer derives a deterministic member id and inserts via
// AddIfAbsent, so a second insert with the same id is a no-op — this is the
// primitive that stops two racing first-sends from double-adding the operator or
// double-posting the join note.
func TestPeerAgentMemberAddIfAbsentIsIdempotent(t *testing.T) {
	h := testutil.New(t)
	tenant := h.SeedTenant()
	admin := h.SeedAdmin(tenant.ID, "owner@test", models.PlatformOwner)
	ctx := context.Background()
	peer := mkPeer(t, h, tenant.ID, "trip_1")

	mk := func() *models.ConversationMember {
		return &models.ConversationMember{
			ID: "m_peer_agent_fixed", TenantID: tenant.ID, ConversationID: peer.ID,
			MemberKind: models.MemberAgent, InternalActorID: &admin.ID, ConvRole: models.ConvRole("support"),
		}
	}

	added, err := h.Store.Members().AddIfAbsent(ctx, mk())
	if err != nil || !added {
		t.Fatalf("first AddIfAbsent: added=%v err=%v, want true/nil", added, err)
	}
	added, err = h.Store.Members().AddIfAbsent(ctx, mk())
	if err != nil {
		t.Fatalf("second AddIfAbsent: err = %v", err)
	}
	if added {
		t.Fatalf("second AddIfAbsent reported added=true, want false (idempotent on the primary key)")
	}

	members, err := h.Store.Members().ListActive(ctx, tenant.ID, peer.ID)
	if err != nil {
		t.Fatalf("list members: %v", err)
	}
	agents := 0
	for i := range members {
		if members[i].MemberKind == models.MemberAgent {
			agents++
		}
	}
	if agents != 1 {
		t.Fatalf("agent members after duplicate join = %d, want 1", agents)
	}
}
