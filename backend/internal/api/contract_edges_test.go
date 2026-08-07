package api_test

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/bitllow/sild/backend/internal/archive"
	"github.com/bitllow/sild/backend/internal/store/models"
	"github.com/bitllow/sild/backend/internal/testutil"
)

// Queueing a conversation is a key/operator action; the shared route accepts any
// credential, so it must authorize rather than trust the route.
func TestUserCannotQueueAConversation(t *testing.T) {
	h := testutil.New(t)
	tenant := h.SeedTenant()
	tok := h.MintToken(tenant.ID, "u_client")

	var conv struct {
		ID string `json:"id"`
	}
	testutil.DecodeJSON(t, h.Request("POST", "/v1/conversations").Bearer(tok).JSON(map[string]any{}).Do(), &conv)

	w := h.Request("POST", "/v1/conversations/"+conv.ID+"/assignments").Bearer(tok).JSON(map[string]any{}).Do()
	if w.Code != http.StatusForbidden {
		t.Fatalf("user must not add an assignment, got %d %s", w.Code, w.Body)
	}
}

// Archival retains the conversation row, so a second run must skip it — otherwise
// it re-archives an empty history over the existing sink object.
func TestArchiveIsNotRepeated(t *testing.T) {
	h := testutil.New(t)
	tenant := h.SeedTenant()
	key := h.SeedAPIKey(tenant.ID)
	tok := h.MintToken(tenant.ID, "u_client")
	ctx := context.Background()

	var conv struct {
		ID string `json:"id"`
	}
	testutil.DecodeJSON(t, h.Request("POST", "/v1/conversations").Bearer(tok).JSON(map[string]any{}).Do(), &conv)
	h.Request("POST", "/v1/conversations/"+conv.ID+"/messages").Bearer(tok).JSON(map[string]any{"body": "keep me"}).Do()
	h.Request("POST", "/v1/conversations/"+conv.ID+"/close").Bearer(key).Do()

	sink, _ := archive.New(h.Cfg, h.Bucket)
	job := archive.NewJob(h.Store, sink, h.Cfg)
	job.SetClock(func() time.Time { return time.Now().Add(60 * 24 * time.Hour) })

	if n, err := job.RunOnce(ctx, tenant.ID, 100); err != nil || n != 1 {
		t.Fatalf("first archive: n=%d err=%v", n, err)
	}
	if n, err := job.RunOnce(ctx, tenant.ID, 100); err != nil || n != 0 {
		t.Fatalf("second archive must be a no-op, got n=%d err=%v", n, err)
	}

	// The history is still readable, so the sink object survived.
	var page struct {
		Items []map[string]any `json:"items"`
	}
	w := h.Request("GET", "/v1/conversations/"+conv.ID+"/messages").Bearer(tok).Do()
	if w.Code != http.StatusOK {
		t.Fatalf("archived read: %d %s", w.Code, w.Body)
	}
	testutil.DecodeJSON(t, w, &page)
	if len(page.Items) != 1 || page.Items[0]["body"] != "keep me" {
		t.Fatalf("archived history was destroyed by the second run: %+v", page.Items)
	}
}

// The declared limit for a route must be the one that applies. 413 is the
// contract, and handlers turn any decode failure into 400, so the cap answers.
func TestJSONBodyCapAnswers413(t *testing.T) {
	h := testutil.New(t)
	tenant := h.SeedTenant()
	tok := h.MintToken(tenant.ID, "u_client")

	var conv struct {
		ID string `json:"id"`
	}
	testutil.DecodeJSON(t, h.Request("POST", "/v1/conversations").Bearer(tok).JSON(map[string]any{}).Do(), &conv)

	w := h.Request("POST", "/v1/conversations/"+conv.ID+"/messages").
		Bearer(tok).JSON(map[string]any{"body": strings.Repeat("x", 300<<10)}).Do()
	if w.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("oversized JSON should be 413, got %d %s", w.Code, w.Body)
	}
}

// A collection whose storage queries one direction must refuse an order override
// rather than mint a cursor labelled with an order it never applied.
func TestFixedOrderCollectionsRejectOrderOverride(t *testing.T) {
	h := testutil.New(t)
	tenant := h.SeedTenant()
	tok := h.MintToken(tenant.ID, "u_client")
	h.SeedAdmin(tenant.ID, "owner@test", models.PlatformOwner)
	owner := loginAs(t, h, "owner@test")

	var conv struct {
		ID string `json:"id"`
	}
	testutil.DecodeJSON(t, h.Request("POST", "/v1/conversations").Bearer(tok).JSON(map[string]any{}).Do(), &conv)

	if w := h.Request("GET", "/v1/conversations/"+conv.ID+"/messages?order=asc").Bearer(tok).Do(); w.Code != http.StatusBadRequest {
		t.Fatalf("messages order override should be 400, got %d %s", w.Code, w.Body)
	}
	if w := h.Request("GET", "/v1/contacts?order=asc").Cookie("sild_admin", owner).Do(); w.Code != http.StatusBadRequest {
		t.Fatalf("contacts order override should be 400, got %d", w.Code)
	}
	if w := h.Request("GET", "/v1/team?order=asc").Cookie("sild_admin", owner).Do(); w.Code != http.StatusBadRequest {
		t.Fatalf("team order override should be 400, got %d", w.Code)
	}
	// The conversation list DOES honour it.
	if w := h.Request("GET", "/v1/conversations?order=asc").Bearer(tok).Do(); w.Code != http.StatusOK {
		t.Fatalf("conversation list should accept order=asc, got %d %s", w.Code, w.Body)
	}
}

// Filters the search query cannot express are refused, not silently applied to an
// already-paginated set of ids (which would thin the page and make has_more lie).
func TestSearchRejectsInexpressibleFilters(t *testing.T) {
	h := testutil.New(t)
	tenant := h.SeedTenant()
	h.SeedAdmin(tenant.ID, "owner@test", models.PlatformOwner)
	owner := loginAs(t, h, "owner@test")

	for _, q := range []string{
		"/v1/conversations?q=x&assignment_status=queued",
		"/v1/conversations?q=x&participant=u_1",
		"/v1/conversations?q=x&assignee=none",
	} {
		if w := h.Request("GET", q).Cookie("sild_admin", owner).Do(); w.Code != http.StatusBadRequest {
			t.Fatalf("%s should be 400, got %d %s", q, w.Code, w.Body)
		}
	}
	// status/assignee/role DO reach the search query, so they are accepted.
	if w := h.Request("GET", "/v1/conversations?q=x&status=open").Cookie("sild_admin", owner).Do(); w.Code != http.StatusOK {
		t.Fatalf("q + status should be accepted, got %d %s", w.Code, w.Body)
	}
}

// The rate-limit bucket must not come from a client-controlled header.
func TestRateLimitIgnoresForwardedFor(t *testing.T) {
	h := testutil.New(t)
	h.SeedTenant()

	over := false
	for i := 0; i < 25; i++ {
		w := h.Request("POST", "/v1/admin/auth/password").
			Header("X-Forwarded-For", "10.0.0."+string(rune('1'+i%9))).
			JSON(map[string]any{"email": "nobody@test", "password": "wrong"}).Do()
		if w.Code == http.StatusTooManyRequests {
			over = true
			break
		}
	}
	if !over {
		t.Fatal("a varying X-Forwarded-For bypassed the auth rate limit")
	}
}
