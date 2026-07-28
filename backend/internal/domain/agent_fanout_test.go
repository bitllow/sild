package domain_test

import (
	"context"
	"testing"

	"github.com/bitllow/sild/backend/internal/domain"
	"github.com/bitllow/sild/backend/internal/realtime"
	"github.com/bitllow/sild/backend/internal/store/models"
	"github.com/bitllow/sild/backend/internal/testutil"
)

func sendAndCapture(t *testing.T, h *testutil.Harness, tenantID, convID string, in domain.SendInput) []realtime.Target {
	t.Helper()
	h.Pub.Reset()
	if _, err := h.Svc.SendMessage(context.Background(), tenantID, convID, in); err != nil {
		t.Fatalf("send: %v", err)
	}
	var out []realtime.Target
	for _, e := range h.Pub.OfType(realtime.EventMessageCreated) {
		out = append(out, e.Target)
	}
	if len(out) == 0 {
		t.Fatal("no message.created was published")
	}
	return out
}

// support opens an assignment, which is what classifies the conversation (§1).
func newConversation(t *testing.T, h *testutil.Harness, tenantID string, support bool) string {
	t.Helper()
	conv, err := h.Svc.CreateConversation(context.Background(), tenantID, domain.CreateConversationInput{
		OpenAssignment: support,
		Members:        []domain.MemberInput{{UserID: "u_rider", ConvRole: models.RoleClient}},
	})
	if err != nil {
		t.Fatalf("create conversation: %v", err)
	}
	return conv.ID
}

func reachedAgents(targets []realtime.Target, tenantID string) bool {
	for _, tg := range targets {
		if tg.Tenant == tenantID {
			return true
		}
	}
	return false
}

// Otherwise an inbox not looking at that thread never learns it moved.
func TestSupportMessageReachesTheAgentsChannel(t *testing.T) {
	h := testutil.New(t)
	tenant := h.SeedTenant()
	convID := newConversation(t, h, tenant.ID, true)

	ext := "u_rider"
	targets := sendAndCapture(t, h, tenant.ID, convID, domain.SendInput{
		SenderKind: models.SenderUser, External: &ext, Body: "where is my driver",
	})

	if !reachedAgents(targets, tenant.ID) {
		t.Fatalf("no agents-channel fan-out for an assigned support conversation: %+v", targets)
	}
}

// The agents channel reaches every operator; peer conversations are gated on
// peer_access, so they belong on the peer channel and nowhere else.
func TestPeerConversationDoesNotReachAgents(t *testing.T) {
	h := testutil.New(t)
	tenant := h.SeedTenant()
	convID := newConversation(t, h, tenant.ID, false)

	ext := "u_rider"
	targets := sendAndCapture(t, h, tenant.ID, convID, domain.SendInput{
		SenderKind: models.SenderUser, External: &ext, Body: "nobody owns this yet",
	})

	if reachedAgents(targets, tenant.ID) {
		t.Fatalf("a peer conversation fanned out to every operator: %+v", targets)
	}
	for _, tg := range targets {
		if tg.Peer != tenant.ID {
			t.Fatalf("peer conversation missed the peer channel: %+v", tg)
		}
	}
}

func TestInternalNoteReachesAgentsAndNotParticipants(t *testing.T) {
	h := testutil.New(t)
	tenant := h.SeedTenant()
	convID := newConversation(t, h, tenant.ID, true)

	actor := "adm_1"
	targets := sendAndCapture(t, h, tenant.ID, convID, domain.SendInput{
		SenderKind: models.SenderAgent, Internal: &actor, Body: "customer called before",
		Visibility: models.VisibilityInternal, AllowInternal: true,
	})

	for _, tg := range targets {
		if !tg.Internal {
			t.Fatalf("an internal note was published on a participants target: %+v", tg)
		}
	}
	if !reachedAgents(targets, tenant.ID) {
		t.Fatal("internal note never reached the agents channel, so no inbox sees it")
	}
}
