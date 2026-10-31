package realtime_test

import (
	"context"
	"slices"
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

func TestAQueueGrantFollowsALiveSocket(t *testing.T) {
	h := testutil.New(t)
	tenant := h.SeedTenant()
	ctx := context.Background()
	if err := h.Store.Queues().Upsert(ctx, &models.Queue{TenantID: tenant.ID, Key: "triage", Webhooks: true}); err != nil {
		t.Fatal(err)
	}
	agent := h.SeedAdmin(tenant.ID, "a@test", models.PlatformAgent)
	triage := agent.ID + "→" + realtime.QueueChannel(tenant.ID, "triage")

	h.Pub.Reset()
	if err := h.Svc.RescopeRole(ctx, tenant.ID, agent.ID, models.PlatformAgent, models.RoleScope{Queues: []string{"support", "triage"}}); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(h.Pub.Subscribed, []string{triage}) || len(h.Pub.Unsubscribed) != 0 {
		t.Fatalf("grant: subscribed %v, unsubscribed %v", h.Pub.Subscribed, h.Pub.Unsubscribed)
	}

	if err := h.Svc.RescopeRole(ctx, tenant.ID, agent.ID, models.PlatformAgent, models.RoleScope{Queues: []string{"support"}}); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(h.Pub.Unsubscribed, []string{triage}) {
		t.Fatalf("revoke: unsubscribed %v", h.Pub.Unsubscribed)
	}
}

func TestARequesterQueueGrantFollowsALiveSocket(t *testing.T) {
	h := testutil.New(t)
	tenant := h.SeedTenant()
	ctx := context.Background()
	for _, key := range []string{"triage", "billing"} {
		if err := h.Store.Queues().Upsert(ctx, &models.Queue{TenantID: tenant.ID, Key: key, Webhooks: true}); err != nil {
			t.Fatal(err)
		}
	}
	marko := h.SeedAdminScoped(tenant.ID, "m@test", models.PlatformRequester, models.RoleScope{Queues: []string{"billing"}})
	if err := h.DB.Create(&models.Conversation{ID: "c_tri", TenantID: tenant.ID, Queue: "triage", Status: models.ConversationOpen}).Error; err != nil {
		t.Fatal(err)
	}
	if err := h.Store.Members().Add(ctx, &models.ConversationMember{TenantID: tenant.ID, ConversationID: "c_tri",
		MemberKind: models.MemberAgent, InternalActorID: &marko.ID, ConvRole: models.RoleClient}); err != nil {
		t.Fatal(err)
	}
	conv := marko.ID + "→" + realtime.ConvChannel("c_tri")

	h.Pub.Reset()
	if err := h.Svc.RescopeRole(ctx, tenant.ID, marko.ID, models.PlatformRequester, models.RoleScope{Queues: []string{"billing", "triage"}}); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(h.Pub.Subscribed, []string{conv}) {
		t.Fatalf("grant: subscribed %v", h.Pub.Subscribed)
	}

	if err := h.Svc.RescopeRole(ctx, tenant.ID, marko.ID, models.PlatformRequester, models.RoleScope{Queues: []string{"billing"}}); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(h.Pub.Unsubscribed, []string{conv}) {
		t.Fatalf("revoke: unsubscribed %v", h.Pub.Unsubscribed)
	}
}

func TestANewQueueReachesOperatorsOfEveryQueue(t *testing.T) {
	h := testutil.New(t)
	tenant := h.SeedTenant()
	ctx := context.Background()
	if err := h.Svc.SetQueue(ctx, &models.Queue{TenantID: tenant.ID, Key: "billing"}); err != nil {
		t.Fatal(err)
	}
	owner := h.SeedAdmin(tenant.ID, "o@test", models.PlatformOwner)
	h.SeedAdmin(tenant.ID, "a@test", models.PlatformAgent)
	h.SeedAdminScoped(tenant.ID, "m@test", models.PlatformRequester, models.RoleScope{Queues: []string{"billing"}})

	h.Pub.Reset()
	if err := h.Svc.SetQueue(ctx, &models.Queue{TenantID: tenant.ID, Key: "triage"}); err != nil {
		t.Fatal(err)
	}
	if want := []string{owner.ID + "→" + realtime.QueueChannel(tenant.ID, "triage")}; !slices.Equal(h.Pub.Subscribed, want) {
		t.Fatalf("subscribed %v, want %v", h.Pub.Subscribed, want)
	}
}
