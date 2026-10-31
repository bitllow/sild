package api_test

import (
	"context"
	"github.com/bitllow/sild/backend/internal/archive"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/bitllow/sild/backend/internal/realtime"
	"github.com/bitllow/sild/backend/internal/store/models"
	"github.com/bitllow/sild/backend/internal/testutil"
)

var stepsWithInternal = []map[string]any{{
	"type": "steps",
	"steps": []map[string]any{{
		"label": "Checked his phone registration", "state": "running",
		"internal": map[string]any{"source": "driver.devices", "params": map[string]any{"driver": "drv_2207"}},
	}},
}}

func (f *requesterFixture) botMessage(t *testing.T, parts any) string {
	t.Helper()
	w := f.h.Request("POST", "/v1/conversations/"+f.own+"/messages").Bearer(f.key).
		JSON(map[string]any{"body": "Checking your account…", "parts": parts}).Do()
	if w.Code != http.StatusCreated {
		t.Fatalf("assistant parts message: %d %s", w.Code, w.Body)
	}
	var m struct {
		ID string `json:"id"`
	}
	testutil.DecodeJSON(t, w, &m)
	return m.ID
}

func TestRequesterNeverReceivesInternalStepFields(t *testing.T) {
	f := newRequesterFixture(t)
	f.h.Pub.Reset()
	f.botMessage(t, stepsWithInternal)

	w := f.h.Request("GET", "/v1/conversations/"+f.own+"/messages").Cookie("sild_admin", f.marko).Do()
	if strings.Contains(w.Body.String(), "drv_2207") || !strings.Contains(w.Body.String(), "Checked his phone registration") {
		t.Fatalf("requester messages: %s", w.Body)
	}
	f.h.SeedAdmin(f.tenant.ID, "owner@test", models.PlatformOwner)
	owner := loginAs(t, f.h, "owner@test")
	if w := f.h.Request("GET", "/v1/conversations/"+f.own+"/messages").Cookie("sild_admin", owner).Do(); !strings.Contains(w.Body.String(), "drv_2207") {
		t.Fatalf("owner lost internal step fields: %s", w.Body)
	}

	for _, c := range f.h.Pub.OfType(realtime.EventMessageCreated) {
		full := strings.Contains(testutil.MustJSON(t, c.Env.Data), "drv_2207")
		if c.Target.Conversation != "" && !c.Target.Internal && full {
			t.Fatalf("internal step fields published to the conversation channel: %+v", c.Target)
		}
		if c.Target.Queue.Key == "triage" && !full {
			t.Fatalf("operators got a stripped copy: %+v", c.Target)
		}
	}
}

func TestRequesterNeverReceivesEvidenceReferences(t *testing.T) {
	f := newRequesterFixture(t)
	f.botMessage(t, []map[string]any{{
		"type": "diagnosis", "cause": "Notifications are blocked", "fix": []string{"Allow notifications"},
		"evidence": []map[string]any{{
			"finding": "Alerts fail since 08:01", "source": "your attachment",
			"internal": map[string]any{"ref": "data/m_1/1.csv"},
		}},
	}})

	w := f.h.Request("GET", "/v1/conversations/"+f.own+"/messages").Cookie("sild_admin", f.marko).Do()
	if strings.Contains(w.Body.String(), "data/m_1/1.csv") || !strings.Contains(w.Body.String(), "Alerts fail since 08:01") {
		t.Fatalf("requester messages: %s", w.Body)
	}
	f.h.SeedAdmin(f.tenant.ID, "owner@test", models.PlatformOwner)
	owner := loginAs(t, f.h, "owner@test")
	if w := f.h.Request("GET", "/v1/conversations/"+f.own+"/messages").Cookie("sild_admin", owner).Do(); !strings.Contains(w.Body.String(), "data/m_1/1.csv") {
		t.Fatalf("owner lost the evidence reference: %s", w.Body)
	}
}

func TestPartLimitsCountCharactersNotBytes(t *testing.T) {
	f := newRequesterFixture(t)
	f.botMessage(t, []map[string]any{{"type": "diagnosis", "cause": strings.Repeat("я", 1000), "fix": []string{"x"}}})
}

func TestOnlyTheAssistantSendsParts(t *testing.T) {
	f := newRequesterFixture(t)
	w := f.h.Request("POST", "/v1/conversations/"+f.own+"/messages").Cookie("sild_admin", f.marko).
		JSON(map[string]any{"body": "fake", "parts": []map[string]any{{"type": "diagnosis", "cause": "x", "fix": []string{"y"}}}}).Do()
	if w.Code != http.StatusBadRequest && w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("requester sent parts: %d %s", w.Code, w.Body)
	}
	bad := []any{
		[]map[string]any{{"type": "html", "body": "<b>"}},
		[]map[string]any{{"type": "steps", "steps": []map[string]any{{"label": "x", "state": "exploded"}}}},
		[]map[string]any{{"type": "diagnosis", "cause": strings.Repeat("x", 2000)}},
		[]map[string]any{{"type": "diagnosis", "cause": "x", "evidence": []map[string]any{{"finding": "y"}}}},
		[]map[string]any{{"type": "steps", "steps": []map[string]any{{"label": "x", "state": "done"}}, "evidence": []map[string]any{{"finding": "y", "source": "z"}}}},
	}
	for _, parts := range bad {
		w := f.h.Request("POST", "/v1/conversations/"+f.own+"/messages").Bearer(f.key).JSON(map[string]any{"body": "x", "parts": parts}).Do()
		if w.Code < 400 || w.Code >= 500 {
			t.Errorf("invalid parts accepted: %d %s", w.Code, w.Body)
		}
	}
}

func TestPartsEditedOnlyByTheirAuthorWhileOpen(t *testing.T) {
	f := newRequesterFixture(t)
	id := f.botMessage(t, stepsWithInternal)
	done := []map[string]any{{"type": "steps", "steps": []map[string]any{{"label": "Checked his phone registration", "state": "done", "result": "Token expired 07:42"}}}}
	path := "/v1/conversations/" + f.own + "/messages/" + id

	if code := f.h.Request("PATCH", path).Cookie("sild_admin", f.marko).JSON(map[string]any{"parts": done}).Do().Code; code != http.StatusForbidden {
		t.Fatalf("requester edited the assistant's parts: %d", code)
	}
	other, err := f.h.Svc.CreateServiceAccount(context.Background(), f.tenant.ID, "other", "", models.PlatformAgent, models.RoleScope{Queues: []string{"triage"}})
	if err != nil {
		t.Fatal(err)
	}
	otherKey, _, _ := f.h.Svc.CreateServiceKey(context.Background(), f.tenant.ID, other.ID, "x")
	if code := f.h.Request("PATCH", path).Bearer(otherKey).JSON(map[string]any{"parts": done}).Do().Code; code != http.StatusForbidden {
		t.Fatalf("another assistant edited the parts: %d", code)
	}

	f.h.Pub.Reset()
	w := f.h.Request("PATCH", path).Bearer(f.key).JSON(map[string]any{"parts": done}).Do()
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "Token expired 07:42") {
		t.Fatalf("author edit: %d %s", w.Code, w.Body)
	}
	if len(f.h.Pub.OfType(realtime.EventMessageUpdated)) == 0 {
		t.Fatal("no message.updated event")
	}

	f.h.Request("POST", "/v1/conversations/"+f.own+"/close").Cookie("sild_admin", f.marko).Do()
	if code := f.h.Request("PATCH", path).Bearer(f.key).JSON(map[string]any{"parts": done}).Do().Code; code != http.StatusConflict {
		t.Fatalf("edit after close: %d", code)
	}
}

func TestArchivedRequestNeverShowsInternalStepFields(t *testing.T) {
	f := newRequesterFixture(t)
	f.botMessage(t, stepsWithInternal)
	if code := f.h.Request("POST", "/v1/conversations/"+f.own+"/close").Cookie("sild_admin", f.marko).Do().Code; code != http.StatusOK {
		t.Fatalf("close: %d", code)
	}
	job := archive.NewJob(f.h.Store, f.h.Sink, f.h.Cfg)
	job.SetClock(func() time.Time { return time.Now().Add(60 * 24 * time.Hour) })
	if _, err := job.RunOnce(context.Background(), f.tenant.ID, 100); err != nil {
		t.Fatal(err)
	}

	w := f.h.Request("GET", "/v1/conversations/"+f.own+"/messages").Cookie("sild_admin", f.marko).Do()
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "Checked his phone registration") {
		t.Fatalf("archived read: %d %s", w.Code, w.Body)
	}
	if strings.Contains(w.Body.String(), "drv_2207") {
		t.Fatalf("archived read leaked internal step fields to the requester: %s", w.Body)
	}
}

func TestInternalStepFieldsStayOffListsAndWebhooks(t *testing.T) {
	f := newRequesterFixture(t)
	ctx := context.Background()
	if err := f.h.Store.Queues().Upsert(ctx, &models.Queue{
		TenantID: f.tenant.ID, Key: "triage", Label: "Tech support", RequeueOnReply: true, Webhooks: true,
	}); err != nil {
		t.Fatal(err)
	}
	f.botMessage(t, stepsWithInternal)
	w := f.h.Request("POST", "/v1/conversations/"+f.triage+"/messages").Bearer(f.key).
		JSON(map[string]any{"body": "Checking…", "parts": stepsWithInternal}).Do()
	if w.Code != http.StatusCreated || !strings.Contains(w.Body.String(), "drv_2207") {
		t.Fatalf("assistant's own send lost its internal fields: %d %s", w.Code, w.Body)
	}

	rows, err := f.h.Svc.ListUserConversations(ctx, f.tenant.ID, "u_client")
	if err != nil {
		t.Fatal(err)
	}
	if got := testutil.MustJSON(t, rows); !strings.Contains(got, "Checked his phone registration") || strings.Contains(got, "drv_2207") {
		t.Fatalf("client list rows: %s", got)
	}
	if w := f.h.Request("GET", "/v1/conversations?queue=triage").Cookie("sild_admin", f.marko).Do(); strings.Contains(w.Body.String(), "drv_2207") {
		t.Fatalf("requester list rows leaked internal step fields: %s", w.Body)
	}

	claimed, _, err := f.h.Store.Outbox().ClaimDue(ctx, 100)
	if err != nil {
		t.Fatal(err)
	}
	sent := 0
	for _, e := range claimed {
		if e.EventType != "message.created" {
			continue
		}
		sent++
		if strings.Contains(string(e.Payload), "drv_2207") {
			t.Fatalf("webhook carried internal step fields: %s", e.Payload)
		}
	}
	if sent != 2 {
		t.Fatalf("want 2 message.created webhooks, got %d", sent)
	}
}
