package api_test

import (
	"net/http"
	"testing"

	"github.com/bitllow/sild/backend/internal/store/models"
	"github.com/bitllow/sild/backend/internal/testutil"
)

// GET returns a forwarding address (token@inbound-domain) and the toggles; PATCH
// updates them and persists. (§6.2, §8 Settings → Channels.)
func TestEmailChannelGetAndUpdate(t *testing.T) {
	h := testutil.New(t)
	tenant := h.SeedTenant()
	h.SeedAdmin(tenant.ID, "owner@test", models.PlatformOwner)
	owner := loginAs(t, h, "owner@test")

	var got struct {
		Channel           string `json:"channel"`
		ForwardingAddress string `json:"forwarding_address"`
		InboundDomain     string `json:"inbound_domain"`
		Verified          bool   `json:"verified"`
		AutoReply         bool   `json:"auto_reply"`
		SpamFilter        bool   `json:"spam_filter"`
	}
	w := h.Request("GET", "/v1/admin/channels/email").Cookie("sild_admin", owner).Do()
	if w.Code != http.StatusOK {
		t.Fatalf("get: %d %s", w.Code, w.Body)
	}
	testutil.DecodeJSON(t, w, &got)
	if got.Channel != "email" || got.InboundDomain != "inbound.test" {
		t.Fatalf("unexpected channel view: %+v", got)
	}
	// A forwarding token is minted on first access: "<token>@inbound.test".
	if got.ForwardingAddress == "" || got.ForwardingAddress[len(got.ForwardingAddress)-len("@inbound.test"):] != "@inbound.test" {
		t.Fatalf("forwarding_address = %q", got.ForwardingAddress)
	}
	if !got.SpamFilter || got.AutoReply || got.Verified {
		t.Fatalf("defaults wrong: spam=%v auto=%v verified=%v", got.SpamFilter, got.AutoReply, got.Verified)
	}
	firstAddr := got.ForwardingAddress

	// PATCH the toggles.
	w = h.Request("PATCH", "/v1/admin/channels/email").Cookie("sild_admin", owner).
		JSON(map[string]any{"auto_reply": true, "spam_filter": false}).Do()
	if w.Code != http.StatusOK {
		t.Fatalf("patch: %d %s", w.Code, w.Body)
	}

	// Re-GET: changes persisted and the forwarding address is stable.
	w = h.Request("GET", "/v1/admin/channels/email").Cookie("sild_admin", owner).Do()
	testutil.DecodeJSON(t, w, &got)
	if !got.AutoReply || got.SpamFilter {
		t.Fatalf("patch did not persist: auto=%v spam=%v", got.AutoReply, got.SpamFilter)
	}
	if got.ForwardingAddress != firstAddr {
		t.Fatalf("forwarding address changed: %q != %q", got.ForwardingAddress, firstAddr)
	}

	// Toggling a bool back off must persist too — booleans carry DB defaults, and
	// a naive upsert would omit the false value and leave the old value in place.
	w = h.Request("PATCH", "/v1/admin/channels/email").Cookie("sild_admin", owner).
		JSON(map[string]any{"auto_reply": false}).Do()
	if w.Code != http.StatusOK {
		t.Fatalf("patch off: %d %s", w.Code, w.Body)
	}
	w = h.Request("GET", "/v1/admin/channels/email").Cookie("sild_admin", owner).Do()
	testutil.DecodeJSON(t, w, &got)
	if got.AutoReply || got.SpamFilter {
		t.Fatalf("toggles should both be off, got auto=%v spam=%v", got.AutoReply, got.SpamFilter)
	}
}

// Channels settings are owner/admin only; an agent is forbidden, no session 401.
func TestEmailChannelRBAC(t *testing.T) {
	h := testutil.New(t)
	tenant := h.SeedTenant()
	h.SeedAdmin(tenant.ID, "agent@test", models.PlatformAgent)
	agent := loginAs(t, h, "agent@test")

	if w := h.Request("GET", "/v1/admin/channels/email").Cookie("sild_admin", agent).Do(); w.Code != http.StatusForbidden {
		t.Fatalf("agent must not read channels, got %d %s", w.Code, w.Body)
	}
	if w := h.Request("GET", "/v1/admin/channels/email").Do(); w.Code != http.StatusUnauthorized {
		t.Fatalf("no session must be 401, got %d", w.Code)
	}
}
