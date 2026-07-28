package domain_test

import (
	"context"
	"testing"

	"github.com/bitllow/sild/backend/internal/domain"
	"github.com/bitllow/sild/backend/internal/realtime"
	"github.com/bitllow/sild/backend/internal/store/models"
	"github.com/bitllow/sild/backend/internal/testutil"
)

// send posts one participant message and returns the targets it fanned out to.
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

func newConversation(t *testing.T, h *testutil.Harness, tenantID string, assigned bool) string {
	t.Helper()
	conv, err := h.Svc.CreateConversation(context.Background(), tenantID, domain.CreateConversationInput{
		OpenAssignment: assigned,
		Members:        []domain.MemberInput{{UserID: "u_rider", ConvRole: models.RoleClient}},
	})
	if err != nil {
		t.Fatalf("create conversation: %v", err)
	}
	return conv.ID
}

// Agents observe background conversations on the tenant channel rather than one
// subscription per conversation, so a message in an assigned support conversation
// has to fan out there — otherwise an inbox that is not looking at that thread
// never learns it moved.
func TestSupportMessageReachesTheAgentsChannel(t *testing.T) {
	h := testutil.New(t)
	tenant := h.SeedTenant()
	convID := newConversation(t, h, tenant.ID, true)

	ext := "u_rider"
	targets := sendAndCapture(t, h, tenant.ID, convID, domain.SendInput{
		SenderKind: models.SenderUser, External: &ext, Body: "where is my driver",
	})

	agents := false
	for _, tg := range targets {
		if tg.Tenant == tenant.ID {
			agents = true
		}
	}
	if !agents {
		t.Fatalf("no agents-channel fan-out for an assigned support conversation: %+v", targets)
	}
}

// The agents channel reaches EVERY operator, and an unassigned support
// conversation is one no agent may read (policy: support_only needs an
// assignment). Publishing it there would hand every operator a conversation REST
// refuses them.
func TestUnassignedSupportConversationDoesNotReachAgents(t *testing.T) {
	h := testutil.New(t)
	tenant := h.SeedTenant()
	convID := newConversation(t, h, tenant.ID, false)

	ext := "u_rider"
	targets := sendAndCapture(t, h, tenant.ID, convID, domain.SendInput{
		SenderKind: models.SenderUser, External: &ext, Body: "nobody owns this yet",
	})

	for _, tg := range targets {
		if tg.Tenant != "" {
			t.Fatalf("an unassigned support conversation fanned out to every operator: %+v", tg)
		}
		if tg.Conversation != convID {
			t.Fatalf("message left its own conversation channel: %+v", tg)
		}
	}
}

// Internal notes are agent-only (§5.6). They must reach the agents channel now
// that agents no longer subscribe per conversation — and must never appear on the
// participants channel, which end users are subscribed to.
func TestInternalNoteReachesAgentsAndNotParticipants(t *testing.T) {
	h := testutil.New(t)
	tenant := h.SeedTenant()
	convID := newConversation(t, h, tenant.ID, true)

	actor := "adm_1"
	targets := sendAndCapture(t, h, tenant.ID, convID, domain.SendInput{
		SenderKind: models.SenderAgent, Internal: &actor, Body: "customer called before",
		Visibility: models.VisibilityInternal, AllowInternal: true,
	})

	agents := false
	for _, tg := range targets {
		if !tg.Internal {
			t.Fatalf("an internal note was published on a participants target: %+v", tg)
		}
		if tg.Tenant == tenant.ID {
			agents = true
		}
	}
	if !agents {
		t.Fatal("internal note never reached the agents channel, so no inbox sees it")
	}
}
