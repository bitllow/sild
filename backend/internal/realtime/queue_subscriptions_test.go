package realtime_test

import (
	"context"
	"testing"

	"github.com/bitllow/sild/backend/internal/realtime"
	"github.com/bitllow/sild/backend/internal/store/models"
	"github.com/bitllow/sild/backend/internal/testutil"
)

func TestOperatorSubscriptionsFollowQueueScope(t *testing.T) {
	h := testutil.New(t)
	tenant := h.SeedTenant()
	ctx := context.Background()
	if err := h.Store.Queues().Upsert(ctx, &models.Queue{TenantID: tenant.ID, Key: "triage", Webhooks: true}); err != nil {
		t.Fatal(err)
	}
	agents, triage := realtime.AgentsChannel(tenant.ID), realtime.QueueChannel(tenant.ID, "triage")

	cases := []struct {
		name         string
		admin        *models.AdminUser
		agents, tria bool
	}{
		{"support agent", h.SeedAdmin(tenant.ID, "a@test", models.PlatformAgent), true, false},
		{"triage agent", h.SeedAdminScoped(tenant.ID, "t@test", models.PlatformAgent, models.RoleScope{Queues: []string{"triage"}}), false, true},
		{"both", h.SeedAdminScoped(tenant.ID, "b@test", models.PlatformAgent, models.RoleScope{Queues: []string{"support", "triage"}}), true, true},
		{"owner", h.SeedAdmin(tenant.ID, "o@test", models.PlatformOwner), true, true},
	}
	for _, c := range cases {
		subs, err := realtime.AgentSubscriptions(ctx, h.Store, tenant.ID, c.admin.ID)
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		_, gotAgents := subs[agents]
		_, gotTriage := subs[triage]
		if gotAgents != c.agents || gotTriage != c.tria {
			t.Errorf("%s: agents=%v triage=%v, want %v %v", c.name, gotAgents, gotTriage, c.agents, c.tria)
		}
	}
}

func TestRequesterSubscribesToOwnConversationsOnly(t *testing.T) {
	h := testutil.New(t)
	tenant := h.SeedTenant()
	ctx := context.Background()
	if err := h.Store.Queues().Upsert(ctx, &models.Queue{TenantID: tenant.ID, Key: "triage", Webhooks: true}); err != nil {
		t.Fatal(err)
	}
	marko := h.SeedAdminScoped(tenant.ID, "m@test", models.PlatformRequester, models.RoleScope{Queues: []string{"triage"}})
	own := &models.ConversationMember{TenantID: tenant.ID, ConversationID: "c_own", MemberKind: models.MemberAgent,
		InternalActorID: &marko.ID, ConvRole: models.RoleClient}
	if err := h.DB.Create(&models.Conversation{ID: "c_own", TenantID: tenant.ID, Queue: "triage", Status: models.ConversationOpen}).Error; err != nil {
		t.Fatal(err)
	}
	if err := h.Store.Members().Add(ctx, own); err != nil {
		t.Fatal(err)
	}
	subs, err := realtime.AgentSubscriptions(ctx, h.Store, tenant.ID, marko.ID)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]bool{realtime.UserChannel(marko.ID): true, realtime.ConvChannel("c_own"): true}
	if len(subs) != len(want) {
		t.Fatalf("requester channels: %v", subs)
	}
	for ch := range subs {
		if !want[ch] {
			t.Fatalf("requester subscribed to %s", ch)
		}
	}
}

func TestRequesterLosesConversationsOfARemovedQueue(t *testing.T) {
	h := testutil.New(t)
	tenant := h.SeedTenant()
	ctx := context.Background()
	for _, key := range []string{"triage", "billing"} {
		if err := h.Store.Queues().Upsert(ctx, &models.Queue{TenantID: tenant.ID, Key: key, Webhooks: true}); err != nil {
			t.Fatal(err)
		}
	}
	marko := h.SeedAdminScoped(tenant.ID, "m@test", models.PlatformRequester, models.RoleScope{Queues: []string{"billing"}})
	if err := h.DB.Create(&models.Conversation{ID: "c_old", TenantID: tenant.ID, Queue: "triage", Status: models.ConversationOpen}).Error; err != nil {
		t.Fatal(err)
	}
	if err := h.Store.Members().Add(ctx, &models.ConversationMember{TenantID: tenant.ID, ConversationID: "c_old",
		MemberKind: models.MemberAgent, InternalActorID: &marko.ID, ConvRole: models.RoleClient}); err != nil {
		t.Fatal(err)
	}
	subs, err := realtime.AgentSubscriptions(ctx, h.Store, tenant.ID, marko.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := subs[realtime.ConvChannel("c_old")]; ok {
		t.Fatal("requester still subscribed to a conversation in a queue they no longer use")
	}
}
