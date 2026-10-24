package api_test

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/bitllow/sild/backend/internal/store/models"
	"github.com/bitllow/sild/backend/internal/testutil"
)

type requesterFixture struct {
	*serviceFixture
	marko, liis     string
	markoID, liisID string
	own             string
}

func newRequesterFixture(t *testing.T) *requesterFixture {
	t.Helper()
	f := &requesterFixture{serviceFixture: newServiceFixture(t)}
	scope := models.RoleScope{Queues: []string{"triage"}}
	f.markoID = f.h.SeedAdminScoped(f.tenant.ID, "marko@test", models.PlatformRequester, scope).ID
	f.liisID = f.h.SeedAdminScoped(f.tenant.ID, "liis@test", models.PlatformRequester, scope).ID
	f.marko, f.liis = loginAs(t, f.h, "marko@test"), loginAs(t, f.h, "liis@test")
	f.own = f.open(t, f.marko, http.StatusCreated)
	return f
}

func (f *requesterFixture) open(t *testing.T, cookie string, want int) string {
	t.Helper()
	w := f.h.Request("POST", "/v1/conversations").Cookie("sild_admin", cookie).
		JSON(map[string]any{"queue": "triage", "open_assignment": true}).Do()
	if w.Code != want {
		t.Fatalf("requester open: want %d, got %d %s", want, w.Code, w.Body)
	}
	var conv struct {
		ID string `json:"id"`
	}
	if w.Code == http.StatusCreated {
		testutil.DecodeJSON(t, w, &conv)
	}
	return conv.ID
}

func TestRequesterReachesOnlyOwnConversations(t *testing.T) {
	f := newRequesterFixture(t)
	only(t, f.list(t, f.marko, "&queue=triage"), f.own)
	if got := f.list(t, f.liis, "&queue=triage"); len(got) != 0 {
		t.Fatalf("liis listed marko's request: %v", got)
	}
	if got := f.list(t, f.marko, ""); len(got) != 0 {
		t.Fatalf("requester listed the support queue: %v", got)
	}

	for _, conv := range []string{f.support, f.triage} {
		paths := []struct{ method, path string }{
			{"GET", "/v1/conversations/" + conv},
			{"GET", "/v1/conversations/" + conv + "/messages"},
			{"POST", "/v1/conversations/" + conv + "/messages"},
			{"POST", "/v1/conversations/" + conv + "/close"},
		}
		for _, p := range paths {
			w := f.h.Request(p.method, p.path).Cookie("sild_admin", f.marko).JSON(map[string]any{"body": "hi"}).Do()
			if w.Code != http.StatusForbidden {
				t.Errorf("%s %s on someone else's conversation: want 403, got %d", p.method, p.path, w.Code)
			}
		}
	}
	if code := f.read(f.liis, f.own); code != http.StatusForbidden {
		t.Fatalf("liis read marko's request: %d", code)
	}
	if code := f.read(f.marko, f.own); code != http.StatusOK {
		t.Fatalf("marko cannot read his request: %d", code)
	}
	w := f.h.Request("POST", "/v1/conversations/"+f.own+"/messages").Cookie("sild_admin", f.marko).JSON(map[string]any{"body": "VIN rejected"}).Do()
	if w.Code != http.StatusCreated {
		t.Fatalf("marko cannot write to his request: %d %s", w.Code, w.Body)
	}
}

func TestRequesterOpensOnlyInOwnQueuesAndOnce(t *testing.T) {
	f := newRequesterFixture(t)
	f.open(t, f.marko, http.StatusConflict)
	w := f.h.Request("POST", "/v1/conversations").Cookie("sild_admin", f.marko).
		JSON(map[string]any{"external_user_id": "u_x", "open_assignment": true}).Do()
	if w.Code != http.StatusForbidden {
		t.Fatalf("requester opened a support conversation: %d %s", w.Code, w.Body)
	}
	if code := f.h.Request("POST", "/v1/conversations/"+f.own+"/close").Cookie("sild_admin", f.marko).Do().Code; code != http.StatusOK && code != http.StatusNoContent {
		t.Fatalf("requester close own: %d", code)
	}
	f.open(t, f.marko, http.StatusCreated)
}

func TestRequesterNeverSeesInternalNotes(t *testing.T) {
	f := newRequesterFixture(t)
	w := f.h.Request("POST", "/v1/conversations/"+f.own+"/messages").Bearer(f.key).
		JSON(map[string]any{"body": "provider ids: drv_2207", "visibility": "internal"}).Do()
	if w.Code != http.StatusCreated {
		t.Fatalf("assistant internal note: %d %s", w.Code, w.Body)
	}
	var page struct {
		Items []struct {
			Body       string `json:"body"`
			Visibility string `json:"visibility"`
		} `json:"items"`
	}
	w = f.h.Request("GET", "/v1/conversations/"+f.own+"/messages").Cookie("sild_admin", f.marko).Do()
	testutil.DecodeJSON(t, w, &page)
	for _, m := range page.Items {
		if m.Visibility == "internal" {
			t.Fatalf("requester received an internal note: %q", m.Body)
		}
	}
	w = f.h.Request("POST", "/v1/conversations/"+f.own+"/messages").Cookie("sild_admin", f.marko).
		JSON(map[string]any{"body": "x", "visibility": "internal"}).Do()
	if w.Code == http.StatusCreated {
		var m struct {
			Visibility string `json:"visibility"`
		}
		testutil.DecodeJSON(t, w, &m)
		if m.Visibility == "internal" {
			t.Fatal("requester posted an internal note")
		}
	}
}

func TestOperatorsStillSeeInternalNotes(t *testing.T) {
	f := newRequesterFixture(t)
	f.h.Request("POST", "/v1/conversations/"+f.own+"/messages").Bearer(f.key).
		JSON(map[string]any{"body": "note", "visibility": "internal"}).Do()
	f.h.SeedAdmin(f.tenant.ID, "owner@test", models.PlatformOwner)
	owner := loginAs(t, f.h, "owner@test")
	var page struct {
		Items []struct {
			Visibility string `json:"visibility"`
		} `json:"items"`
	}
	w := f.h.Request("GET", "/v1/conversations/"+f.own+"/messages").Cookie("sild_admin", owner).Do()
	testutil.DecodeJSON(t, w, &page)
	for _, m := range page.Items {
		if m.Visibility == "internal" {
			return
		}
	}
	t.Fatal("owner lost internal notes")
}

func TestRequesterWithAllQueuesReachesOwnRequests(t *testing.T) {
	f := newServiceFixture(t)
	f.h.SeedAdminScoped(f.tenant.ID, "eva@test", models.PlatformRequester, models.RoleScope{Queues: []string{models.ScopeAll}})
	eva := loginAs(t, f.h, "eva@test")
	w := f.h.Request("POST", "/v1/conversations").Cookie("sild_admin", eva).JSON(map[string]any{"queue": "triage", "open_assignment": true}).Do()
	if w.Code != http.StatusCreated {
		t.Fatalf("requester with all queues could not open: %d %s", w.Code, w.Body)
	}
	var conv struct {
		ID string `json:"id"`
	}
	testutil.DecodeJSON(t, w, &conv)
	only(t, f.list(t, eva, "&queue=triage"), conv.ID)
	if code := f.read(eva, f.triage); code != http.StatusForbidden {
		t.Fatalf("requester with all queues read someone else's request: %d", code)
	}
}

func TestAgentWhoIsAlsoARequesterReachesOwnRequests(t *testing.T) {
	f := newServiceFixture(t)
	liis := f.h.SeedAdmin(f.tenant.ID, "liis@test", models.PlatformAgent)
	if err := f.h.Svc.AssignRole(context.Background(), f.tenant.ID, liis.ID, models.PlatformRequester, models.RoleScope{Queues: []string{"triage"}}); err != nil {
		t.Fatal(err)
	}
	cookie := loginAs(t, f.h, "liis@test")
	w := f.h.Request("POST", "/v1/conversations").Cookie("sild_admin", cookie).JSON(map[string]any{"queue": "triage", "open_assignment": true}).Do()
	if w.Code != http.StatusCreated {
		t.Fatalf("agent+requester could not open: %d %s", w.Code, w.Body)
	}
	var own struct {
		ID string `json:"id"`
	}
	testutil.DecodeJSON(t, w, &own)

	if code := f.read(cookie, own.ID); code != http.StatusOK {
		t.Fatalf("agent+requester cannot read own request: %d", code)
	}
	if w := f.h.Request("POST", "/v1/conversations/"+own.ID+"/messages").Cookie("sild_admin", cookie).JSON(map[string]any{"body": "hi"}).Do(); w.Code != http.StatusCreated {
		t.Fatalf("agent+requester cannot write own request: %d %s", w.Code, w.Body)
	}
	only(t, f.list(t, cookie, "&queue=triage"), own.ID)
	only(t, f.list(t, cookie, ""), f.support)
	if code := f.read(cookie, f.triage); code != http.StatusForbidden {
		t.Fatalf("agent+requester read someone else's triage request: %d", code)
	}

	// Their agent role works support, not triage: on their own request they are only its requester.
	f.h.Request("POST", "/v1/conversations/"+own.ID+"/messages").Bearer(f.key).
		JSON(map[string]any{"body": "provider ids: drv_2207", "visibility": "internal"}).Do()
	if body := f.h.Request("GET", "/v1/conversations/"+own.ID+"/messages").Cookie("sild_admin", cookie).Do().Body.String(); strings.Contains(body, "drv_2207") {
		t.Fatalf("agent+requester read an internal note on their own request: %s", body)
	}
	w = f.h.Request("POST", "/v1/conversations/"+own.ID+"/messages").Cookie("sild_admin", cookie).JSON(map[string]any{"body": "x", "visibility": "internal"}).Do()
	if w.Code == http.StatusCreated && strings.Contains(w.Body.String(), `"visibility":"internal"`) {
		t.Fatal("agent+requester wrote an internal note on their own request")
	}
	var conv struct {
		Assignment struct {
			ID string `json:"id"`
		} `json:"assignment"`
	}
	testutil.DecodeJSON(t, f.h.Request("GET", "/v1/conversations/"+own.ID).Cookie("sild_admin", cookie).Do(), &conv)
	if w := f.h.Request("PATCH", "/v1/assignments/"+conv.Assignment.ID).Cookie("sild_admin", cookie).
		JSON(map[string]any{"assignee_actor_id": "me"}).Do(); w.Code < 400 {
		t.Fatalf("agent+requester claimed the assistant's assignment on their own request: %d %s", w.Code, w.Body)
	}
}

func TestAdminCannotIssueAKeyForAnOwnerServiceAccount(t *testing.T) {
	f := newServiceFixture(t)
	ownerBot, err := f.h.Svc.CreateServiceAccount(context.Background(), f.tenant.ID, "ownerbot", "", models.PlatformOwner, models.RoleScope{})
	if err != nil {
		t.Fatal(err)
	}
	f.h.SeedAdmin(f.tenant.ID, "admin@test", models.PlatformAdmin)
	admin := loginAs(t, f.h, "admin@test")
	if code := f.h.Request("POST", "/v1/service-accounts/"+ownerBot.ID+"/keys").Cookie("sild_admin", admin).Do().Code; code != http.StatusForbidden {
		t.Fatalf("admin issued a key carrying owner powers: %d", code)
	}
}

func TestRequesterSearchSeesOnlyOwnRequests(t *testing.T) {
	f := newRequesterFixture(t)
	f.h.Request("POST", "/v1/conversations/"+f.own+"/messages").Cookie("sild_admin", f.marko).JSON(map[string]any{"body": "VIN rejected for the van"}).Do()
	liisOwn := f.open(t, f.liis, http.StatusCreated)
	f.h.Request("POST", "/v1/conversations/"+liisOwn+"/messages").Cookie("sild_admin", f.liis).JSON(map[string]any{"body": "VIN rejected again"}).Do()

	var page struct {
		Items []struct {
			ID string `json:"id"`
		} `json:"items"`
		HasMore bool `json:"has_more"`
	}
	w := f.h.Request("GET", "/v1/conversations?queue=triage&q=VIN&limit=1").Cookie("sild_admin", f.marko).Do()
	testutil.DecodeJSON(t, w, &page)
	if len(page.Items) != 1 || page.Items[0].ID != f.own || page.HasMore {
		t.Fatalf("requester search: %+v", page)
	}
}

func TestOwnerOpensOwnTriageRequestButSupportStaysForUsers(t *testing.T) {
	f := newServiceFixture(t)
	f.h.SeedAdmin(f.tenant.ID, "owner@test", models.PlatformOwner)
	owner := loginAs(t, f.h, "owner@test")

	w := f.h.Request("POST", "/v1/conversations").Cookie("sild_admin", owner).JSON(map[string]any{"queue": "triage", "open_assignment": true}).Do()
	if w.Code != http.StatusCreated {
		t.Fatalf("owner could not open a triage request: %d %s", w.Code, w.Body)
	}
	if w := f.h.Request("POST", "/v1/conversations").Cookie("sild_admin", owner).JSON(map[string]any{"queue": "triage", "open_assignment": true}).Do(); w.Code != http.StatusConflict {
		t.Fatalf("owner second open request: %d", w.Code)
	}
	if w := f.h.Request("POST", "/v1/conversations").Cookie("sild_admin", owner).JSON(map[string]any{"open_assignment": true}).Do(); w.Code != http.StatusBadRequest {
		t.Fatalf("owner support create without a user must stay a 400: %d %s", w.Code, w.Body)
	}
	if w := f.h.Request("POST", "/v1/conversations").Cookie("sild_admin", owner).JSON(map[string]any{"external_user_id": "u_x", "open_assignment": true}).Do(); w.Code != http.StatusCreated {
		t.Fatalf("owner support create for a user: %d %s", w.Code, w.Body)
	}
	var me struct {
		Grants []struct {
			Action string `json:"action"`
			Scope  struct {
				OwnQueues []string `json:"own_queues"`
			} `json:"scope"`
		} `json:"grants"`
	}
	testutil.DecodeJSON(t, f.h.Request("GET", "/v1/principal").Cookie("sild_admin", owner).Do(), &me)
	for _, g := range me.Grants {
		if g.Action == "conversations.list" && (len(g.Scope.OwnQueues) != 1 || g.Scope.OwnQueues[0] != models.ScopeAll) {
			t.Fatalf("owner own_queues: %+v", g.Scope)
		}
	}
}
