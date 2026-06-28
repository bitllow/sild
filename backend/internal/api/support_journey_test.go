package api_test

import (
	"net/http"
	"testing"

	"github.com/bitllow/sild/backend/internal/store/models"
	"github.com/bitllow/sild/backend/internal/testutil"
)

// TestSupportAgentInboxJourney is the full support loop from both sides:
//
//	client: mint token → open support request → send message
//	agent:  log in → SEE it queued in the inbox → claim it → answer
//	client: catch up and SEE the answer → mark read
//	agent:  receives the read receipt event
func TestSupportAgentInboxJourney(t *testing.T) {
	h := testutil.New(t)
	tenant := h.SeedTenant()
	apiKey := h.SeedAPIKey(tenant.ID)
	h.SeedAdmin(tenant.ID, "agent@test", models.PlatformAgent)

	// ── client side: mint a token, open a support request, ask a question ──
	var tok struct{ Token string `json:"token"` }
	w := h.Request("POST", "/v1/tokens").Bearer(apiKey).JSON(map[string]any{"user_id": "u_client"}).Do()
	testutil.DecodeJSON(t, w, &tok)

	var conv struct {
		ID         string         `json:"id"`
		Assignment map[string]any `json:"assignment"`
	}
	w = h.Request("POST", "/v1/me/support-requests").Bearer(tok.Token).JSON(map[string]any{}).Do()
	if w.Code != http.StatusCreated {
		t.Fatalf("open support: %d %s", w.Code, w.Body)
	}
	testutil.DecodeJSON(t, w, &conv)
	assignmentID, _ := conv.Assignment["id"].(string)
	if assignmentID == "" {
		t.Fatal("support request should carry an assignment")
	}

	var firstMsg struct{ ID string `json:"id"` }
	w = h.Request("POST", "/v1/conversations/"+conv.ID+"/messages").
		Bearer(tok.Token).JSON(map[string]any{"body": "my card was charged twice"}).Do()
	testutil.DecodeJSON(t, w, &firstMsg)

	// ── agent side: log in, SEE the request queued in the inbox ──
	cookie := loginAs(t, h, "agent@test")

	var queue []map[string]any
	w = h.Request("GET", "/v1/admin/assignments?status=queued").Cookie("sild_admin", cookie).Do()
	if w.Code != http.StatusOK {
		t.Fatalf("inbox: %d %s", w.Code, w.Body)
	}
	testutil.DecodeJSON(t, w, &queue)
	if !containsAssignment(queue, assignmentID) {
		t.Fatalf("agent should see the queued assignment %s, got %+v", assignmentID, queue)
	}

	// agent can read the incoming question
	var convView struct {
		Members []map[string]any `json:"members"`
	}
	w = h.Request("GET", "/v1/conversations/"+conv.ID).Cookie("sild_admin", cookie).Do()
	if w.Code != http.StatusOK {
		t.Fatalf("agent read conv: %d %s", w.Code, w.Body)
	}
	testutil.DecodeJSON(t, w, &convView)

	// ── agent claims the assignment (queued → assigned) ──
	w = h.Request("POST", "/v1/admin/assignments/"+assignmentID+"/claim").Cookie("sild_admin", cookie).Do()
	if w.Code != http.StatusOK {
		t.Fatalf("claim: %d %s", w.Code, w.Body)
	}
	var claimed struct {
		Status          string `json:"status"`
		AssigneeActorID string `json:"assignee_actor_id"`
	}
	testutil.DecodeJSON(t, w, &claimed)
	if claimed.Status != "assigned" || claimed.AssigneeActorID == "" {
		t.Fatalf("after claim expected assigned+assignee, got %+v", claimed)
	}

	// it should no longer appear in the queued list
	w = h.Request("GET", "/v1/admin/assignments?status=queued").Cookie("sild_admin", cookie).Do()
	queue = nil
	testutil.DecodeJSON(t, w, &queue)
	if containsAssignment(queue, assignmentID) {
		t.Fatal("claimed assignment must leave the queued list")
	}

	// ── agent answers ──
	var reply struct{ ID string `json:"id"` }
	w = h.Request("POST", "/v1/conversations/"+conv.ID+"/messages").
		Cookie("sild_admin", cookie).JSON(map[string]any{"body": "I've refunded the duplicate charge"}).Do()
	if w.Code != http.StatusCreated {
		t.Fatalf("agent answer: %d %s", w.Code, w.Body)
	}
	testutil.DecodeJSON(t, w, &reply)

	// ── client SEES the answer via catch-up ──
	var after struct {
		Messages []map[string]any `json:"messages"`
	}
	w = h.Request("GET", "/v1/conversations/"+conv.ID+"/messages?after="+firstMsg.ID).Bearer(tok.Token).Do()
	testutil.DecodeJSON(t, w, &after)
	if len(after.Messages) != 1 || after.Messages[0]["body"] != "I've refunded the duplicate charge" {
		t.Fatalf("client should see the agent's answer, got %+v", after.Messages)
	}
	if after.Messages[0]["sender_kind"] != string(models.SenderAgent) {
		t.Fatalf("answer should be from an agent, got %v", after.Messages[0]["sender_kind"])
	}

	// ── client marks read → agent receives a read receipt event ──
	h.Pub.Reset()
	w = h.Request("POST", "/v1/conversations/"+conv.ID+"/read").
		Bearer(tok.Token).JSON(map[string]any{"last_read_message_id": reply.ID}).Do()
	if w.Code != http.StatusNoContent {
		t.Fatalf("mark read: %d %s", w.Code, w.Body)
	}
	reads := h.Pub.OfType("message.read")
	if len(reads) != 1 {
		t.Fatalf("expected a message.read event, got %d", len(reads))
	}
	if reads[0].Env.Data.(map[string]any)["last_read_message_id"] != reply.ID {
		t.Fatalf("read receipt should reference the agent's message, got %+v", reads[0].Env.Data)
	}
}

func containsAssignment(list []map[string]any, id string) bool {
	for _, a := range list {
		if a["id"] == id {
			return true
		}
	}
	return false
}
