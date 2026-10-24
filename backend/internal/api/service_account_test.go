package api_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/bitllow/sild/backend/internal/store/models"
	"github.com/bitllow/sild/backend/internal/testutil"
)

type serviceFixture struct {
	*queueFixture
	admin *models.AdminUser
	key   string
}

func newServiceFixture(t *testing.T) *serviceFixture {
	t.Helper()
	f := newQueueFixture(t, false)
	ctx := context.Background()
	admin, err := f.h.Svc.CreateServiceAccount(ctx, f.tenant.ID, "assistant", "Triage assistant",
		models.PlatformAgent, models.RoleScope{Queues: []string{"triage"}})
	if err != nil {
		t.Fatal(err)
	}
	key, _, err := f.h.Svc.CreateServiceKey(ctx, f.tenant.ID, admin.ID, "bridge")
	if err != nil {
		t.Fatal(err)
	}
	return &serviceFixture{queueFixture: f, admin: admin, key: key}
}

func TestServiceKeyIsRefusedOnServerRoutes(t *testing.T) {
	f := newServiceFixture(t)
	routes := []struct{ method, path string }{
		{"POST", "/v1/tokens"},
		{"POST", "/v1/conversations/" + f.triage + "/members"},
		{"DELETE", "/v1/conversations/" + f.triage + "/members/u_client"},
		{"POST", "/v1/conversations/" + f.triage + "/members/remap"},
		{"PUT", "/v1/contacts/u_client"},
		{"PUT", "/v1/contacts/u_client/push"},
		{"DELETE", "/v1/contacts/u_client/push-tokens"},
	}
	for _, r := range routes {
		w := f.h.Request(r.method, r.path).Bearer(f.key).JSON(map[string]any{"user_id": "u_x"}).Do()
		if w.Code != http.StatusUnauthorized {
			t.Errorf("%s %s with a service key: want 401, got %d %s", r.method, r.path, w.Code, w.Body)
		}
	}
}

func TestServiceKeyActsAsItsAccount(t *testing.T) {
	f := newServiceFixture(t)
	var principal struct {
		Kind    string `json:"kind"`
		Subject struct {
			ID string `json:"id"`
		} `json:"subject"`
	}
	w := f.h.Request("GET", "/v1/principal").Bearer(f.key).Do()
	testutil.DecodeJSON(t, w, &principal)
	if principal.Kind != "admin" || principal.Subject.ID != f.admin.ID {
		t.Fatalf("service key resolved to %+v", principal)
	}

	if code := f.h.Request("GET", "/v1/conversations/"+f.support).Bearer(f.key).Do().Code; code != http.StatusForbidden {
		t.Fatalf("triage service account read a support conversation: %d", code)
	}
	var page struct {
		Items []struct {
			ID         string `json:"id"`
			Assignment struct {
				ID string `json:"id"`
			} `json:"assignment"`
		} `json:"items"`
	}
	w = f.h.Request("GET", "/v1/conversations?kind=support&queue=triage").Bearer(f.key).Do()
	testutil.DecodeJSON(t, w, &page)
	if len(page.Items) != 1 || page.Items[0].ID != f.triage {
		t.Fatalf("service account triage list: %+v", page.Items)
	}

	claim := f.h.Request("PATCH", "/v1/assignments/"+page.Items[0].Assignment.ID).Bearer(f.key).
		JSON(map[string]any{"assignee_actor_id": "me"}).Do()
	if claim.Code != http.StatusOK {
		t.Fatalf("service account claim: %d %s", claim.Code, claim.Body)
	}

	var msg struct {
		SenderKind      string `json:"sender_kind"`
		InternalActorID string `json:"internal_actor_id"`
	}
	w = f.h.Request("POST", "/v1/conversations/"+f.triage+"/messages").Bearer(f.key).JSON(map[string]any{"body": "Looking into it."}).Do()
	if w.Code != http.StatusCreated {
		t.Fatalf("service account send: %d %s", w.Code, w.Body)
	}
	testutil.DecodeJSON(t, w, &msg)
	if msg.SenderKind != "bot" || msg.InternalActorID != f.admin.ID {
		t.Fatalf("service account message: %+v", msg)
	}

	if code := f.h.Request("GET", "/v1/realtime/token").Bearer(f.key).Do().Code; code != http.StatusOK {
		t.Fatalf("service account realtime token: %d", code)
	}
}

func TestServiceAccountCannotSignIn(t *testing.T) {
	f := newServiceFixture(t)
	if w := f.h.Request("GET", "/v1/admin/auth/google/dev?email=assistant").Do(); w.Code == http.StatusOK {
		t.Fatalf("service account signed in: %d", w.Code)
	}
}

func TestPlainKeyStillServerWide(t *testing.T) {
	f := newServiceFixture(t)
	if w := f.h.Request("POST", "/v1/tokens").Bearer(f.queueFixture.key).JSON(map[string]any{"user_id": "u1"}).Do(); w.Code != http.StatusOK && w.Code != http.StatusCreated {
		t.Fatalf("plain server key lost token minting: %d %s", w.Code, w.Body)
	}
}

func TestTriageOnlyAccountReachesNoSupportContactsOrConversations(t *testing.T) {
	f := newServiceFixture(t)
	w := f.h.Request("POST", "/v1/conversations").Bearer(f.queueFixture.key).JSON(map[string]any{
		"members": []map[string]any{{"user_id": "u_support_only", "conv_role": "client"}}, "open_assignment": true,
	}).Do()
	if w.Code != http.StatusCreated {
		t.Fatalf("support conversation: %d %s", w.Code, w.Body)
	}
	var page struct {
		Items []struct {
			ExternalUserID string `json:"external_user_id"`
		} `json:"items"`
	}
	w = f.h.Request("GET", "/v1/contacts").Bearer(f.key).Do()
	testutil.DecodeJSON(t, w, &page)
	for _, c := range page.Items {
		if c.ExternalUserID == "u_support_only" {
			t.Fatalf("triage-only account listed a support contact: %+v", page.Items)
		}
	}
	if code := f.h.Request("GET", "/v1/contacts/u_support_only").Bearer(f.key).Do().Code; code != http.StatusNotFound && code != http.StatusForbidden {
		t.Fatalf("triage-only account read a support contact: %d", code)
	}
	w = f.h.Request("POST", "/v1/conversations").Bearer(f.key).JSON(map[string]any{"external_user_id": "u_new", "open_assignment": true}).Do()
	if w.Code != http.StatusForbidden {
		t.Fatalf("triage-only account opened a support conversation: %d %s", w.Code, w.Body)
	}
}
