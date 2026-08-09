package api_test

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/bitllow/sild/backend/internal/archive"

	"github.com/bitllow/sild/backend/internal/realtime"
	"github.com/bitllow/sild/backend/internal/store/models"
	"github.com/bitllow/sild/backend/internal/testutil"
)

// putProfile writes a profile over one credential and returns the recorder.
func putProfile(h *testutil.Harness, path, bearer string, metadata map[string]any) *http.Response {
	w := h.Request("PUT", path).Bearer(bearer).JSON(map[string]any{"metadata": metadata}).Do()
	return w.Result()
}

// A person's profile is written once, by the device that knows who is signed in.
func TestUserWritesOwnProfile(t *testing.T) {
	h := testutil.New(t)
	tenant := h.SeedTenant()
	h.SeedAdmin(tenant.ID, "owner@test", models.PlatformOwner)
	owner := loginAs(t, h, "owner@test")
	tok := h.MintToken(tenant.ID, "u_mari")

	if res := putProfile(h, "/v1/contacts/me", tok, map[string]any{"name": "Mari Tamm"}); res.StatusCode != http.StatusNoContent {
		t.Fatalf("PUT /contacts/me = %d", res.StatusCode)
	}

	// The token subject decides whose profile it is, so naming someone else is
	// not a route a user can reach at all.
	if res := putProfile(h, "/v1/contacts/u_jaan", tok, map[string]any{"name": "Not Mari"}); res.StatusCode == http.StatusNoContent {
		t.Fatal("a user token wrote another person's profile")
	}

	newConversation(t, h, tenant.ID, "u_mari")
	if got := contactMeta(t, h, owner, "u_mari")["name"]; got != "Mari Tamm" {
		t.Fatalf("contact name = %v, want Mari Tamm", got)
	}
}

// The host's own backend is the other writer: any of its users, talked to or not.
func TestAPIKeyWritesAnyProfile(t *testing.T) {
	h := testutil.New(t)
	tenant := h.SeedTenant()
	h.SeedAdmin(tenant.ID, "owner@test", models.PlatformOwner)
	owner := loginAs(t, h, "owner@test")
	key := h.SeedAPIKey(tenant.ID)

	if res := putProfile(h, "/v1/contacts/u_silent", key, map[string]any{"name": "Never Wrote In"}); res.StatusCode != http.StatusNoContent {
		t.Fatalf("PUT /contacts/:id = %d", res.StatusCode)
	}

	// A profile is not a directory entry: existence still comes from membership.
	if w := h.Request("GET", "/v1/contacts/u_silent").Cookie("sild_admin", owner).Do(); w.Code != http.StatusNotFound {
		t.Fatalf("a profile with no conversation is visible in the directory: %d", w.Code)
	}
	newConversation(t, h, tenant.ID, "u_silent")
	if got := contactMeta(t, h, owner, "u_silent")["name"]; got != "Never Wrote In" {
		t.Fatalf("contact appeared without their profile: %v", got)
	}
}

// Agents are excluded: a correction made in the inbox would be overwritten by the
// person's next app launch, so it needs its own storage and its own decision.
func TestAgentCannotWriteAProfile(t *testing.T) {
	h := testutil.New(t)
	tenant := h.SeedTenant()
	h.SeedAdmin(tenant.ID, "owner@test", models.PlatformOwner)
	owner := loginAs(t, h, "owner@test")

	w := h.Request("PUT", "/v1/contacts/u_mari").Cookie("sild_admin", owner).
		JSON(map[string]any{"metadata": map[string]any{"name": "Agent Wrote This"}}).Do()
	if w.Code == http.StatusNoContent {
		t.Fatal("an owner session wrote a contact profile")
	}
}

// The configured profile IS the profile: a key dropped from the second write is
// gone, not merged forward.
func TestProfileWriteReplacesRatherThanMerges(t *testing.T) {
	h := testutil.New(t)
	tenant := h.SeedTenant()
	h.SeedAdmin(tenant.ID, "owner@test", models.PlatformOwner)
	owner := loginAs(t, h, "owner@test")
	key := h.SeedAPIKey(tenant.ID)
	newConversation(t, h, tenant.ID, "u_mari")

	putProfile(h, "/v1/contacts/u_mari", key, map[string]any{"name": "Mari Tamm", "plan": "gold"})
	putProfile(h, "/v1/contacts/u_mari", key, map[string]any{"name": "Mari Tamm"})

	meta := contactMeta(t, h, owner, "u_mari")
	if _, ok := meta["plan"]; ok {
		t.Fatalf("a removed key survived the replace: %v", meta)
	}
	if meta["name"] != "Mari Tamm" {
		t.Fatalf("name = %v", meta["name"])
	}
}

// One row, so one correction reaches every thread — closed and archived alike.
// Archival moves the message bulk but keeps the members, so an archived thread
// renders the person's CURRENT profile like any other.
func TestProfileAppliesToEveryConversation(t *testing.T) {
	h := testutil.New(t)
	tenant := h.SeedTenant()
	h.SeedAdmin(tenant.ID, "owner@test", models.PlatformOwner)
	owner := loginAs(t, h, "owner@test")
	key := h.SeedAPIKey(tenant.ID)
	ctx := context.Background()

	open := newConversation(t, h, tenant.ID, "u_mari")
	closed := newConversation(t, h, tenant.ID, "u_mari")
	archived := newConversation(t, h, tenant.ID, "u_mari")
	for _, id := range []string{closed, archived} {
		if w := h.Request("POST", "/v1/conversations/"+id+"/close").Cookie("sild_admin", owner).Do(); w.Code != http.StatusOK {
			t.Fatalf("close: %d %s", w.Code, w.Body)
		}
	}
	job := archive.NewJob(h.Store, h.Sink, h.Cfg)
	job.SetClock(func() time.Time { return time.Now().Add(60 * 24 * time.Hour) })
	if _, err := job.RunOnce(ctx, tenant.ID, 100); err != nil {
		t.Fatalf("archive: %v", err)
	}

	putProfile(h, "/v1/contacts/u_mari", key, map[string]any{"name": "Corrected Name"})

	for _, id := range []string{open, closed, archived} {
		var conv map[string]any
		w := h.Request("GET", "/v1/conversations/"+id).Cookie("sild_admin", owner).Do()
		testutil.DecodeJSON(t, w, &conv)
		if got := memberName(conv, "u_mari"); got != "Corrected Name" {
			t.Fatalf("conversation %s renders %q", id, got)
		}
	}
	var arch map[string]any
	testutil.DecodeJSON(t, h.Request("GET", "/v1/conversations/"+archived).Cookie("sild_admin", owner).Do(), &arch)
	if arch["archived"] != true {
		t.Fatal("the archive job did not run, so the archived case proved nothing")
	}
	// The directory still lists her — archival must not forget a contact.
	if got := contactMeta(t, h, owner, "u_mari")["name"]; got != "Corrected Name" {
		t.Fatalf("directory renders %v", got)
	}
}

func TestLocaleOnlyWriteKeepsTheStoredProfile(t *testing.T) {
	h := testutil.New(t)
	tenant := h.SeedTenant()
	h.SeedAdmin(tenant.ID, "owner@test", models.PlatformOwner)
	owner := loginAs(t, h, "owner@test")
	key := h.SeedAPIKey(tenant.ID)
	newConversation(t, h, tenant.ID, "u_mari")
	putProfile(h, "/v1/contacts/u_mari", key, map[string]any{"name": "Mari Tamm"})

	tok := h.MintToken(tenant.ID, "u_mari")
	if w := h.Request("PUT", "/v1/contacts/me").Bearer(tok).
		JSON(map[string]any{"locale": "lv"}).Do(); w.Code != http.StatusNoContent {
		t.Fatalf("locale write = %d", w.Code)
	}

	if got := contactMeta(t, h, owner, "u_mari")["name"]; got != "Mari Tamm" {
		t.Fatalf("profile after a locale write = %v", got)
	}
	locales, err := h.Store.Contacts().Locales(context.Background(), tenant.ID, []string{"u_mari"})
	if err != nil {
		t.Fatalf("locales: %v", err)
	}
	if locales["u_mari"] != "lv" {
		t.Fatalf("stored locale = %q, want lv", locales["u_mari"])
	}
}

// The SDK writes on every launch, so an unchanged profile must cost the tenant
// channel nothing.
func TestProfileWriteEmitsOnlyOnChange(t *testing.T) {
	h := testutil.New(t)
	tenant := h.SeedTenant()
	key := h.SeedAPIKey(tenant.ID)

	putProfile(h, "/v1/contacts/u_mari", key, map[string]any{"name": "Mari Tamm"})
	if n := len(h.Pub.OfType(realtime.EventContactUpdated)); n != 1 {
		t.Fatalf("first write emitted %d contact.updated events, want 1", n)
	}
	putProfile(h, "/v1/contacts/u_mari", key, map[string]any{"name": "Mari Tamm"})
	if n := len(h.Pub.OfType(realtime.EventContactUpdated)); n != 1 {
		t.Fatalf("an unchanged write emitted an event (%d total)", n)
	}
	putProfile(h, "/v1/contacts/u_mari", key, map[string]any{"name": "Mari Kask"})
	if n := len(h.Pub.OfType(realtime.EventContactUpdated)); n != 2 {
		t.Fatalf("a changed write emitted %d events, want 2", n)
	}
}

// Sild's own state lives in a typed column, so no number of app launches can
// un-suppress someone the tenant silenced.
func TestProfileWriteLeavesAPushOptOutIntact(t *testing.T) {
	f := newPushFixture(t)
	key := f.h.SeedAPIKey(f.tenant.ID)

	if w := f.h.Request("PUT", "/v1/contacts/u_bob/push").Bearer(key).
		JSON(map[string]any{"enabled": false}).Do(); w.Code != http.StatusNoContent {
		t.Fatalf("opt out = %d %s", w.Code, w.Body)
	}
	putProfile(f.h, "/v1/contacts/u_bob", key, map[string]any{"name": "Bob", "plan": "gold"})

	f.send(t, "u_alice", "hi")
	if got := f.h.Notifier.Nudges(); len(got) != 0 {
		t.Fatalf("a profile write undid the opt-out: %d nudges", len(got))
	}
}

// Creating a conversation means naming participants by id. Per-member metadata
// is not silently ignored — it is not a field of the request.
func TestConversationCreateRejectsMemberMetadata(t *testing.T) {
	h := testutil.New(t)
	tenant := h.SeedTenant()
	key := h.SeedAPIKey(tenant.ID)

	w := h.Request("POST", "/v1/conversations").Bearer(key).JSON(map[string]any{
		"members": []map[string]any{
			{"user_id": "u_mari", "conv_role": "client", "metadata": map[string]any{"name": "Mari"}},
		},
		"open_assignment": true,
	}).Do()
	if w.Code != http.StatusBadRequest {
		t.Fatalf("per-member metadata = %d %s, want 400", w.Code, w.Body)
	}
}

// ── helpers ─────────────────────────────────────────────────────────────────

// newConversation opens a support conversation for one end user and returns its id.
func newConversation(t *testing.T, h *testutil.Harness, tenantID, userID string) string {
	t.Helper()
	key := h.SeedAPIKey(tenantID)
	var conv struct {
		ID string `json:"id"`
	}
	w := h.Request("POST", "/v1/conversations").Bearer(key).JSON(map[string]any{
		"members":         []map[string]any{{"user_id": userID, "conv_role": "client"}},
		"open_assignment": true,
	}).Do()
	if w.Code != http.StatusCreated {
		t.Fatalf("create conversation: %d %s", w.Code, w.Body)
	}
	testutil.DecodeJSON(t, w, &conv)
	return conv.ID
}

// contactMeta reads one directory entry's profile.
func contactMeta(t *testing.T, h *testutil.Harness, adminCookie, externalUserID string) map[string]any {
	t.Helper()
	w := h.Request("GET", "/v1/contacts/"+externalUserID).Cookie("sild_admin", adminCookie).Do()
	if w.Code != http.StatusOK {
		t.Fatalf("GET /contacts/%s = %d %s", externalUserID, w.Code, w.Body)
	}
	var body struct {
		Metadata map[string]any `json:"metadata"`
	}
	testutil.DecodeJSON(t, w, &body)
	return body.Metadata
}

// memberName reads a participant's inline profile name off a conversation view.
func memberName(conv map[string]any, externalUserID string) string {
	members, _ := conv["members"].([]any)
	for _, m := range members {
		mm, _ := m.(map[string]any)
		if mm["external_user_id"] != externalUserID {
			continue
		}
		name, _ := mm["name"].(string)
		return name
	}
	return ""
}

// The directory search reads the narrow table's materialized text, so it matches
// exactly the keys the tenant declared searchable.
func TestContactSearchMatchesDeclaredKeys(t *testing.T) {
	h := testutil.New(t)
	tenant := h.SeedTenant("name") // "plan" is deliberately not declared
	h.SeedAdmin(tenant.ID, "owner@test", models.PlatformOwner)
	owner := loginAs(t, h, "owner@test")
	h.SeedContact(tenant.ID, "u_mari", `{"name":"Mari Tamm","plan":"platinum"}`)
	newConversation(t, h, tenant.ID, "u_mari")

	if n := len(contactSearch(t, h, owner, "Tamm")); n != 1 {
		t.Fatalf("declared key matched %d contacts, want 1", n)
	}
	if n := len(contactSearch(t, h, owner, "platinum")); n != 0 {
		t.Fatalf("an undeclared key leaked into contact search: %d hits", n)
	}
}

// Scope still decides the directory: a contact an agent cannot reach through any
// conversation stays invisible, so it does not become a roster of the user base.
func TestDirectoryStaysScopedToTheAgentsConversations(t *testing.T) {
	h := testutil.New(t)
	tenant := h.SeedTenant()
	h.SeedAdmin(tenant.ID, "agent@test", models.PlatformAgent)
	agent := loginAs(t, h, "agent@test")
	key := h.SeedAPIKey(tenant.ID)

	h.SeedContact(tenant.ID, "u_peer", `{"name":"Peer Only"}`)
	// A peer conversation: no assignment, so a plain agent cannot reach it.
	if w := h.Request("POST", "/v1/conversations").Bearer(key).JSON(map[string]any{
		"members": []map[string]any{
			{"user_id": "u_peer", "conv_role": "rider"},
			{"user_id": "u_peer2", "conv_role": "driver"},
		},
	}).Do(); w.Code != http.StatusCreated {
		t.Fatalf("create peer: %d %s", w.Code, w.Body)
	}

	if n := len(contactSearch(t, h, agent, "")); n != 0 {
		t.Fatalf("an unreachable contact appeared in the directory: %d entries", n)
	}
	newConversation(t, h, tenant.ID, "u_peer")
	if n := len(contactSearch(t, h, agent, "")); n != 1 {
		t.Fatalf("directory has %d entries after a reachable conversation, want 1", n)
	}
}

// contactSearch lists the directory as an admin session, optionally filtered.
func contactSearch(t *testing.T, h *testutil.Harness, adminCookie, q string) []map[string]any {
	t.Helper()
	w := h.Request("GET", "/v1/contacts?q="+q).Cookie("sild_admin", adminCookie).Do()
	if w.Code != http.StatusOK {
		t.Fatalf("GET /contacts?q=%s = %d %s", q, w.Code, w.Body)
	}
	var e envelope
	testutil.DecodeJSON(t, w, &e)
	return e.Items
}

// The blob is opaque: an id past float64's exact range must come back as the
// host wrote it, not rounded by a round-trip through the change detector.
func TestProfileMetadataIsStoredVerbatim(t *testing.T) {
	h := testutil.New(t)
	tenant := h.SeedTenant()
	h.SeedAdmin(tenant.ID, "owner@test", models.PlatformOwner)
	owner := loginAs(t, h, "owner@test")
	key := h.SeedAPIKey(tenant.ID)
	newConversation(t, h, tenant.ID, "u_mari")

	w := h.Request("PUT", "/v1/contacts/u_mari").Bearer(key).
		Raw([]byte(`{"metadata":{"account":9007199254740993}}`), "application/json").Do()
	if w.Code != http.StatusNoContent {
		t.Fatalf("PUT = %d %s", w.Code, w.Body)
	}

	got := h.Request("GET", "/v1/contacts/u_mari").Cookie("sild_admin", owner).Do().Body.String()
	if !strings.Contains(got, "9007199254740993") {
		t.Fatalf("large number was rewritten: %s", got)
	}
}
