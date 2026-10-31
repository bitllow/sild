package api_test

import (
	"net/http"
	"testing"

	"github.com/bitllow/sild/backend/internal/realtime"
	"github.com/bitllow/sild/backend/internal/testutil"
)

type convState struct {
	Status      string `json:"status"`
	CloseReason string `json:"close_reason"`
	Assignment  struct {
		ID     string `json:"id"`
		Status string `json:"status"`
	} `json:"assignment"`
}

func (f *requesterFixture) state(t *testing.T, conv string) convState {
	t.Helper()
	var s convState
	testutil.DecodeJSON(t, f.h.Request("GET", "/v1/conversations/"+conv).Bearer(f.key).Do(), &s)
	return s
}

func (f *requesterFixture) say(t *testing.T, conv, body string) {
	t.Helper()
	if w := f.h.Request("POST", "/v1/conversations/"+conv+"/messages").Cookie("sild_admin", f.marko).JSON(map[string]any{"body": body}).Do(); w.Code != http.StatusCreated {
		t.Fatalf("requester message: %d %s", w.Code, w.Body)
	}
}

func TestReplyAfterIdleReleaseRequeues(t *testing.T) {
	f := newRequesterFixture(t)
	first := f.state(t, f.own).Assignment.ID
	f.h.Request("PATCH", "/v1/assignments/"+first).Bearer(f.key).JSON(map[string]any{"assignee_actor_id": "me"}).Do()
	f.h.Request("PATCH", "/v1/assignments/"+first).Bearer(f.key).JSON(map[string]any{"status": "closed"}).Do()
	if got := f.state(t, f.own).Assignment.Status; got != "closed" {
		t.Fatalf("assignment after release: %s", got)
	}

	f.say(t, f.own, "It still fails.")
	s := f.state(t, f.own)
	if s.Assignment.Status != "queued" || s.Assignment.ID == first {
		t.Fatalf("reply did not requeue: %+v", s.Assignment)
	}

	// A second reply while queued does not stack another assignment.
	f.say(t, f.own, "Any news?")
	if again := f.state(t, f.own).Assignment.ID; again != s.Assignment.ID {
		t.Fatalf("second reply opened another assignment: %s", again)
	}
}

func TestAssistantMessageDoesNotRequeue(t *testing.T) {
	f := newRequesterFixture(t)
	a := f.state(t, f.own).Assignment.ID
	f.h.Request("PATCH", "/v1/assignments/"+a).Bearer(f.key).JSON(map[string]any{"assignee_actor_id": "me"}).Do()
	f.h.Request("PATCH", "/v1/assignments/"+a).Bearer(f.key).JSON(map[string]any{"status": "closed"}).Do()
	f.h.Request("POST", "/v1/conversations/"+f.own+"/messages").Bearer(f.key).JSON(map[string]any{"body": "Data arrived."}).Do()
	if got := f.state(t, f.own).Assignment.Status; got != "closed" {
		t.Fatalf("assistant message requeued: %s", got)
	}
}

func TestSupportReplyDoesNotRequeue(t *testing.T) {
	f := newRequesterFixture(t)
	f.h.SeedAdmin(f.tenant.ID, "agent@test", "agent")
	agent := loginAs(t, f.h, "agent@test")
	var s convState
	testutil.DecodeJSON(t, f.h.Request("GET", "/v1/conversations/"+f.support).Cookie("sild_admin", agent).Do(), &s)
	f.h.Request("PATCH", "/v1/assignments/"+s.Assignment.ID).Cookie("sild_admin", agent).JSON(map[string]any{"assignee_actor_id": "me"}).Do()
	f.h.Request("PATCH", "/v1/assignments/"+s.Assignment.ID).Cookie("sild_admin", agent).JSON(map[string]any{"status": "closed"}).Do()

	user := f.h.MintToken(f.tenant.ID, "u_client")
	f.h.Request("POST", "/v1/conversations/"+f.support+"/messages").Bearer(user).JSON(map[string]any{"body": "hello again"}).Do()
	testutil.DecodeJSON(t, f.h.Request("GET", "/v1/conversations/"+f.support).Cookie("sild_admin", agent).Do(), &s)
	if s.Assignment.Status != "closed" {
		t.Fatalf("support reply requeued: %+v", s.Assignment)
	}
}

func TestCloseCarriesReasonAndNeverRequeues(t *testing.T) {
	f := newRequesterFixture(t)
	f.h.Pub.Reset()
	if code := f.h.Request("POST", "/v1/conversations/"+f.own+"/close").Cookie("sild_admin", f.marko).JSON(map[string]any{"reason": "resolved"}).Do().Code; code != http.StatusOK {
		t.Fatalf("close resolved: %d", code)
	}
	s := f.state(t, f.own)
	if s.Status != "closed" || s.CloseReason != "resolved" {
		t.Fatalf("closed state: %+v", s)
	}
	ev := f.h.Pub.OfType(realtime.EventConversationClosed)
	if len(ev) == 0 || testutil.MustJSON(t, ev[0].Env.Data) != `{"reason":"resolved"}` {
		t.Fatalf("closed event: %+v", ev)
	}
	if w := f.h.Request("POST", "/v1/conversations/"+f.own+"/close").Cookie("sild_admin", f.marko).JSON(map[string]any{"reason": "bogus"}).Do(); w.Code < 400 {
		t.Fatalf("bogus reason accepted: %d", w.Code)
	}
}
