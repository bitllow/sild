package api_test

import (
	"net/http"
	"testing"

	"github.com/bitllow/sild/backend/internal/store/models"
	"github.com/bitllow/sild/backend/internal/testutil"
)

// Strict decoding turns a field the server forgot to model into a 400, so the
// bodies the shipped clients actually send are pinned here. A shared URL that
// dispatches by principal or conversation kind must accept ONE body shape on every
// branch — the peer branch dropped `visibility` and broke the inbox's peer replies
// while every unit test still passed.
func TestSharedSendAcceptsTheClientBodyOnEveryBranch(t *testing.T) {
	h := testutil.New(t)
	tenant := h.SeedTenant()
	key := h.SeedAPIKey(tenant.ID)
	jwt := h.MintToken(tenant.ID, "u_rider")
	admin := h.SeedAdmin(tenant.ID, "op@body", models.PlatformOwner)
	h.GrantPeer(tenant.ID, admin.ID)
	owner := loginAs(t, h, "op@body")

	newConv := func(peer bool) string {
		t.Helper()
		body := map[string]any{}
		if peer {
			body["open_assignment"] = false
			body["members"] = []map[string]any{
				{"user_id": "u_rider", "conv_role": "rider"},
				{"user_id": "u_driver", "conv_role": "driver"},
			}
		} else {
			// Explicit: for an API key an ABSENT open_assignment means peer, which
			// would send both operator cases down the same branch.
			body["open_assignment"] = true
			body["members"] = []map[string]any{{"user_id": "u_rider", "conv_role": "client"}}
		}
		var conv struct {
			ID string `json:"id"`
		}
		testutil.DecodeJSON(t, h.Request("POST", "/v1/conversations").Bearer(key).JSON(body).Do(), &conv)
		return conv.ID
	}

	// Exactly what inbox/src/api/admin.ts postMessage() puts on the wire.
	inboxBody := map[string]any{"body": "hello", "visibility": "participants", "attachments": []any{}}

	asOwner := func(r *testutil.Req) *testutil.Req { return r.Cookie("sild_admin", owner) }
	supportConv, peerConv := newConv(false), newConv(true)

	for _, tc := range []struct {
		name string
		conv string
		as   func(*testutil.Req) *testutil.Req
	}{
		{"operator into a support conversation", supportConv, asOwner},
		// The branch that regressed: an operator sending into a peer conversation is
		// routed to a different handler by the DATA, not the URL.
		{"operator into a peer conversation", peerConv, asOwner},
		{"end user into their own conversation", newConv(false),
			func(r *testutil.Req) *testutil.Req { return r.Bearer(jwt) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := tc.as(h.Request("POST", "/v1/conversations/"+tc.conv+"/messages").JSON(inboxBody)).Do()
			if w.Code != http.StatusCreated {
				t.Fatalf("expected 201, got %d %s", w.Code, w.Body)
			}
		})
	}

	// Prove the two operator cases really took different branches rather than both
	// landing on the peer handler: only the support path accepts an internal note.
	t.Run("the operator cases dispatch to different handlers", func(t *testing.T) {
		note := map[string]any{"body": "for the team", "visibility": "internal"}
		if w := asOwner(h.Request("POST", "/v1/conversations/"+supportConv+"/messages").JSON(note)).Do(); w.Code != http.StatusCreated {
			t.Errorf("support conversation must accept an internal note, got %d %s", w.Code, w.Body)
		}
		if w := asOwner(h.Request("POST", "/v1/conversations/"+peerConv+"/messages").JSON(note)).Do(); w.Code != http.StatusBadRequest {
			t.Errorf("peer conversation must refuse an internal note, got %d %s", w.Code, w.Body)
		}
	})
}

// visibility is an enum, not free text. An unknown value used to pass strict
// decoding and reach storage, where the reads disagree about it: thread history
// matches `visibility = participants` and excludes it, while unread counts match
// `visibility <> internal` and count it, and the fan-out treats it as
// participant-visible. The result is a message that chimes and bumps a badge but is
// absent from the thread — and, since the internal-note gate compares against the
// exact value, a non-agent could create one.
func TestUnknownVisibilityIsRefused(t *testing.T) {
	h := testutil.New(t)
	tenant := h.SeedTenant()
	key := h.SeedAPIKey(tenant.ID)
	jwt := h.MintToken(tenant.ID, "u_rider")
	admin := h.SeedAdmin(tenant.ID, "op@vis2", models.PlatformOwner)
	h.GrantPeer(tenant.ID, admin.ID)
	owner := loginAs(t, h, "op@vis2")

	var support, peer struct {
		ID string `json:"id"`
	}
	testutil.DecodeJSON(t, h.Request("POST", "/v1/conversations").Bearer(key).JSON(map[string]any{
		"open_assignment": true,
		"members":         []map[string]any{{"user_id": "u_rider", "conv_role": "client"}},
	}).Do(), &support)
	testutil.DecodeJSON(t, h.Request("POST", "/v1/conversations").Bearer(key).JSON(map[string]any{
		"open_assignment": false,
		"members": []map[string]any{
			{"user_id": "u_rider", "conv_role": "rider"},
			{"user_id": "u_driver", "conv_role": "driver"},
		},
	}).Do(), &peer)

	body := map[string]any{"body": "typo", "visibility": "private"}
	for _, tc := range []struct {
		name string
		conv string
		as   func(*testutil.Req) *testutil.Req
	}{
		{"operator, support", support.ID, func(r *testutil.Req) *testutil.Req { return r.Cookie("sild_admin", owner) }},
		{"operator, peer", peer.ID, func(r *testutil.Req) *testutil.Req { return r.Cookie("sild_admin", owner) }},
		// The bypass: a user may not post an internal note, and an unknown value
		// must not be the way around that gate.
		{"end user", support.ID, func(r *testutil.Req) *testutil.Req { return r.Bearer(jwt) }},
		{"api key", support.ID, func(r *testutil.Req) *testutil.Req { return r.Bearer(key) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := tc.as(h.Request("POST", "/v1/conversations/"+tc.conv+"/messages").JSON(body)).Do()
			if w.Code < 400 {
				t.Fatalf("unknown visibility was accepted (%d %s)", w.Code, w.Body)
			}
		})
	}
}

// A peer conversation has no internal side, so the field is refused rather than
// silently downgraded to participant-visible.
func TestPeerSendRefusesInternalVisibility(t *testing.T) {
	h := testutil.New(t)
	tenant := h.SeedTenant()
	key := h.SeedAPIKey(tenant.ID)
	admin := h.SeedAdmin(tenant.ID, "op@vis", models.PlatformOwner)
	h.GrantPeer(tenant.ID, admin.ID)
	owner := loginAs(t, h, "op@vis")

	var conv struct {
		ID string `json:"id"`
	}
	testutil.DecodeJSON(t, h.Request("POST", "/v1/conversations").Bearer(key).JSON(map[string]any{
		"open_assignment": false,
		"members": []map[string]any{
			{"user_id": "u_rider", "conv_role": "rider"},
			{"user_id": "u_driver", "conv_role": "driver"},
		},
	}).Do(), &conv)

	w := h.Request("POST", "/v1/conversations/"+conv.ID+"/messages").Cookie("sild_admin", owner).
		JSON(map[string]any{"body": "note to self", "visibility": "internal"}).Do()
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for internal visibility in a peer chat, got %d %s", w.Code, w.Body)
	}
}

// channel is the other client-supplied enum on this body, and the same shape of
// hole: the outbound-mail gate suppresses only the exact email channel, so an
// unknown value would have a message emailed out.
func TestUnknownChannelIsRefused(t *testing.T) {
	h := testutil.New(t)
	tenant := h.SeedTenant()
	key := h.SeedAPIKey(tenant.ID)
	tok := h.MintToken(tenant.ID, "u_ch")

	var conv struct {
		ID string `json:"id"`
	}
	testutil.DecodeJSON(t, h.Request("POST", "/v1/conversations").Bearer(key).JSON(map[string]any{
		"open_assignment": true,
		"members":         []map[string]any{{"user_id": "u_ch", "conv_role": "client"}},
	}).Do(), &conv)

	w := h.Request("POST", "/v1/conversations/"+conv.ID+"/messages").Bearer(tok).
		JSON(map[string]any{"body": "hi", "channel": "sms"}).Do()
	if w.Code < 400 {
		t.Fatalf("unknown channel was accepted (%d %s)", w.Code, w.Body)
	}
}
