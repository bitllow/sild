package api_test

import (
	"net/http"
	"testing"

	"github.com/bitllow/sild/backend/internal/store/models"
	"github.com/bitllow/sild/backend/internal/testutil"
)

func TestOwnerManagesQueuesAndServiceAccounts(t *testing.T) {
	h := testutil.New(t)
	tenant := h.SeedTenant()
	h.SeedAdmin(tenant.ID, "owner@test", models.PlatformOwner)
	owner := loginAs(t, h, "owner@test")

	if w := h.Request("PUT", "/v1/queues/triage").Cookie("sild_admin", owner).JSON(map[string]any{"label": "Tech support", "requeue_on_reply": true, "webhooks": false}).Do(); w.Code != http.StatusOK {
		t.Fatalf("put queue: %d %s", w.Code, w.Body)
	}
	if w := h.Request("PUT", "/v1/queues/support").Cookie("sild_admin", owner).JSON(map[string]any{}).Do(); w.Code < 400 {
		t.Fatalf("the built-in queue was rewritten: %d", w.Code)
	}
	var queues struct {
		Items []struct {
			Key      string `json:"key"`
			BuiltIn  bool   `json:"built_in"`
			Webhooks bool   `json:"webhooks"`
		} `json:"items"`
	}
	testutil.DecodeJSON(t, h.Request("GET", "/v1/queues").Cookie("sild_admin", owner).Do(), &queues)
	if len(queues.Items) != 2 || queues.Items[0].Key != "support" || !queues.Items[0].BuiltIn || queues.Items[1].Key != "triage" || queues.Items[1].Webhooks {
		t.Fatalf("queues: %+v", queues.Items)
	}

	var acct struct {
		ID   string `json:"id"`
		Kind string `json:"kind"`
	}
	w := h.Request("POST", "/v1/service-accounts").Cookie("sild_admin", owner).
		JSON(map[string]any{"name": "assistant", "display_name": "Triage assistant", "role": "agent", "scope": map[string]any{"queues": []string{"triage"}}}).Do()
	if w.Code != http.StatusCreated {
		t.Fatalf("create service account: %d %s", w.Code, w.Body)
	}
	testutil.DecodeJSON(t, w, &acct)
	if acct.Kind != "service" {
		t.Fatalf("service account kind: %+v", acct)
	}
	var key struct {
		Key     string `json:"key"`
		ActorID string `json:"actor_id"`
	}
	w = h.Request("POST", "/v1/service-accounts/"+acct.ID+"/keys").Cookie("sild_admin", owner).JSON(map[string]any{"label": "bridge"}).Do()
	testutil.DecodeJSON(t, w, &key)
	if key.Key == "" || key.ActorID != acct.ID {
		t.Fatalf("service key: %d %+v", w.Code, key)
	}
	if code := h.Request("GET", "/v1/principal").Bearer(key.Key).Do().Code; code != http.StatusOK {
		t.Fatalf("issued key does not authenticate: %d", code)
	}

	h.SeedAdmin(tenant.ID, "agent@test", models.PlatformAgent)
	agent := loginAs(t, h, "agent@test")
	if code := h.Request("POST", "/v1/service-accounts").Cookie("sild_admin", agent).JSON(map[string]any{"name": "x"}).Do().Code; code != http.StatusForbidden {
		t.Fatalf("agent created a service account: %d", code)
	}
	if code := h.Request("POST", "/v1/service-accounts/"+h.SeedAdmin(tenant.ID, "human@test", models.PlatformAgent).ID+"/keys").Cookie("sild_admin", owner).Do().Code; code < 400 {
		t.Fatalf("a person got an actor key: %d", code)
	}
}
