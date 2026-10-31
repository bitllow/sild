package api_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/bitllow/sild/backend/internal/realtime"
	"github.com/bitllow/sild/backend/internal/store/models"
	"github.com/bitllow/sild/backend/internal/testutil"
)

type queueFixture struct {
	h       *testutil.Harness
	tenant  *models.Tenant
	key     string
	support string
	triage  string
}

func newQueueFixture(t *testing.T, webhooks bool) *queueFixture {
	t.Helper()
	h := testutil.New(t)
	tenant := h.SeedTenant()
	f := &queueFixture{h: h, tenant: tenant, key: h.SeedAPIKey(tenant.ID)}
	if err := h.Store.Queues().Upsert(context.Background(), &models.Queue{
		TenantID: tenant.ID, Key: "triage", Label: "Tech support", RequeueOnReply: true, Webhooks: webhooks,
	}); err != nil {
		t.Fatal(err)
	}
	f.support = f.create(t, "")
	f.triage = f.create(t, "triage")
	return f
}

func (f *queueFixture) create(t *testing.T, queue string) string {
	t.Helper()
	body := map[string]any{
		"members":         []map[string]any{{"user_id": "u_client", "conv_role": "client"}},
		"open_assignment": true,
	}
	if queue != "" {
		body["queue"] = queue
	}
	var conv struct {
		ID string `json:"id"`
	}
	w := f.h.Request("POST", "/v1/conversations").Bearer(f.key).JSON(body).Do()
	if w.Code != http.StatusCreated {
		t.Fatalf("create in %q: %d %s", queue, w.Code, w.Body)
	}
	testutil.DecodeJSON(t, w, &conv)
	return conv.ID
}

func (f *queueFixture) list(t *testing.T, cookie, query string) []string {
	t.Helper()
	var page struct {
		Items []struct {
			ID string `json:"id"`
		} `json:"items"`
	}
	w := f.h.Request("GET", "/v1/conversations?kind=support"+query).Cookie("sild_admin", cookie).Do()
	if w.Code != http.StatusOK {
		t.Fatalf("list %s: %d %s", query, w.Code, w.Body)
	}
	testutil.DecodeJSON(t, w, &page)
	ids := make([]string, 0, len(page.Items))
	for _, it := range page.Items {
		ids = append(ids, it.ID)
	}
	return ids
}

func (f *queueFixture) read(cookie, convID string) int {
	return f.h.Request("GET", "/v1/conversations/"+convID).Cookie("sild_admin", cookie).Do().Code
}

func only(t *testing.T, got []string, want string) {
	t.Helper()
	if len(got) != 1 || got[0] != want {
		t.Fatalf("want only %s, got %v", want, got)
	}
}

func TestDefaultListsStaySupportOnly(t *testing.T) {
	f := newQueueFixture(t, true)
	f.h.SeedAdmin(f.tenant.ID, "agent@test", models.PlatformAgent)
	f.h.SeedAdmin(f.tenant.ID, "owner@test", models.PlatformOwner)
	agent, owner := loginAs(t, f.h, "agent@test"), loginAs(t, f.h, "owner@test")

	only(t, f.list(t, agent, ""), f.support)
	only(t, f.list(t, owner, ""), f.support)
	only(t, f.list(t, owner, "&queue=triage"), f.triage)
}

func TestSupportCountsExcludeOtherQueues(t *testing.T) {
	f := newQueueFixture(t, true)
	f.h.SeedAdmin(f.tenant.ID, "owner@test", models.PlatformOwner)
	owner := loginAs(t, f.h, "owner@test")
	var page struct {
		Counts struct {
			Open       int `json:"open"`
			Unassigned int `json:"unassigned"`
		} `json:"counts"`
	}
	w := f.h.Request("GET", "/v1/conversations?kind=support").Cookie("sild_admin", owner).Do()
	testutil.DecodeJSON(t, w, &page)
	if page.Counts.Open != 1 || page.Counts.Unassigned != 1 {
		t.Fatalf("counts must cover the support queue only, got %+v", page.Counts)
	}
}

func TestAgentWithoutQueueScopeCannotReachTriage(t *testing.T) {
	f := newQueueFixture(t, true)
	f.h.SeedAdmin(f.tenant.ID, "agent@test", models.PlatformAgent)
	agent := loginAs(t, f.h, "agent@test")

	if got := f.list(t, agent, "&queue=triage"); len(got) != 0 {
		t.Fatalf("support agent listed triage: %v", got)
	}
	if code := f.read(agent, f.triage); code != http.StatusForbidden {
		t.Fatalf("support agent read a triage conversation: %d", code)
	}
	if code := f.read(agent, f.support); code != http.StatusOK {
		t.Fatalf("support agent lost support: %d", code)
	}
}

func TestTriageScopedAgentCannotReachSupport(t *testing.T) {
	f := newQueueFixture(t, true)
	f.h.SeedAdminScoped(f.tenant.ID, "triage@test", models.PlatformAgent, models.RoleScope{Queues: []string{"triage"}})
	agent := loginAs(t, f.h, "triage@test")

	only(t, f.list(t, agent, "&queue=triage"), f.triage)
	if got := f.list(t, agent, ""); len(got) != 0 {
		t.Fatalf("triage agent listed support: %v", got)
	}
	if code := f.read(agent, f.support); code != http.StatusForbidden {
		t.Fatalf("triage agent read a support conversation: %d", code)
	}
	if code := f.read(agent, f.triage); code != http.StatusOK {
		t.Fatalf("triage agent cannot read triage: %d", code)
	}
}

func TestQueueOnCreateIsValidated(t *testing.T) {
	f := newQueueFixture(t, true)
	cases := map[string]map[string]any{
		"unknown queue": {
			"members": []map[string]any{{"user_id": "u1", "conv_role": "client"}}, "open_assignment": true, "queue": "nope",
		},
		"peer with a queue": {
			"members": []map[string]any{{"user_id": "u1", "conv_role": "client"}, {"user_id": "u2", "conv_role": "driver"}}, "queue": "triage",
		},
		"invalid key": {
			"members": []map[string]any{{"user_id": "u1", "conv_role": "client"}}, "open_assignment": true, "queue": "Bad Key",
		},
	}
	for name, body := range cases {
		if w := f.h.Request("POST", "/v1/conversations").Bearer(f.key).JSON(body).Do(); w.Code < 400 || w.Code >= 500 {
			t.Errorf("%s: want 4xx, got %d %s", name, w.Code, w.Body)
		}
	}
	user := f.h.MintToken(f.tenant.ID, "u_self")
	w := f.h.Request("POST", "/v1/conversations").Bearer(user).JSON(map[string]any{"queue": "triage"}).Do()
	if w.Code != http.StatusForbidden {
		t.Fatalf("a user opened into triage: %d %s", w.Code, w.Body)
	}
}

func TestQueueEventsUseTheQueueChannel(t *testing.T) {
	f := newQueueFixture(t, true)
	var supportTarget, triageTarget *realtime.Target
	for _, c := range f.h.Pub.OfType(realtime.EventAssignmentUpdated) {
		c := c
		switch c.Env.ConversationID {
		case f.support:
			supportTarget = &c.Target
		case f.triage:
			triageTarget = &c.Target
		}
	}
	if supportTarget == nil || supportTarget.Tenant != f.tenant.ID || supportTarget.Queue.Key != "" {
		t.Fatalf("support event target: %+v", supportTarget)
	}
	if triageTarget == nil || triageTarget.Tenant != "" || triageTarget.Queue != (realtime.QueueRef{Tenant: f.tenant.ID, Key: "triage"}) {
		t.Fatalf("triage event target: %+v", triageTarget)
	}
}

func TestQueueWithWebhooksOffEnqueuesNothing(t *testing.T) {
	f := newQueueFixture(t, false)
	var rows []models.Outbox
	if err := f.h.DB.Where("tenant_id = ?", f.tenant.ID).Find(&rows).Error; err != nil {
		t.Fatal(err)
	}
	for _, r := range rows {
		if r.ConversationID == f.triage {
			t.Fatalf("triage queue with webhooks off enqueued %s", r.EventType)
		}
	}
	if len(rows) == 0 {
		t.Fatal("support conversation should still enqueue webhooks")
	}
}
