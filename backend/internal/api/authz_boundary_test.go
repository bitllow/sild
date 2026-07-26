package api_test

import (
	"net/http"
	"testing"

	"github.com/bitllow/sild/backend/internal/store/models"
	"github.com/bitllow/sild/backend/internal/testutil"
)

// The manifest is the single source of truth for guards, which means the
// conformance tests cannot catch it being WIDENED: relax `adminOnly` to
// `anyPrincipal` and router, document and guard test all agree again.
//
// So the credential boundaries that matter are pinned here too, deliberately
// duplicated and stated as behaviour rather than as a reading of the manifest.
// Widening one now takes an edit in two places, one of which says why.
func TestCredentialBoundariesAreClosed(t *testing.T) {
	h := testutil.New(t)
	tenant := h.SeedTenant()
	key := h.SeedAPIKey(tenant.ID)
	jwt := h.MintToken(tenant.ID, "u_probe")
	h.SeedAdmin(tenant.ID, "agent@probe", models.PlatformAgent)
	agent := loginAs(t, h, "agent@probe")

	cases := []struct {
		name   string
		method string
		path   string
		// why the credential must not reach it
		reason string
		// which credentials must be refused
		refuseKey, refuseJWT, refuseAgent bool
	}{
		{
			name: "contacts list", method: "GET", path: "/v1/contacts",
			reason:    "the tenant's whole contact directory is an operator surface",
			refuseKey: true, refuseJWT: true,
		},
		{
			name: "contact read", method: "GET", path: "/v1/contacts/u_probe",
			reason:    "a contact is derived from membership the caller may not see",
			refuseKey: true, refuseJWT: true,
		},
		{
			name: "assignment transition", method: "PATCH", path: "/v1/assignments/a_probe",
			reason:    "claiming is an operator act tied to an operator identity",
			refuseKey: true, refuseJWT: true,
		},
		{
			name: "realtime token", method: "GET", path: "/v1/realtime/token",
			reason:    "mints a socket credential carrying the operator's channel set",
			refuseKey: true, refuseJWT: true,
		},
		{
			name: "team list", method: "GET", path: "/v1/team",
			reason:    "operator roster, owner/admin only",
			refuseKey: true, refuseJWT: true, refuseAgent: true,
		},
		{
			name: "api keys list", method: "GET", path: "/v1/api-keys",
			reason:    "server-to-server credentials, owner/admin only",
			refuseKey: true, refuseJWT: true, refuseAgent: true,
		},
		{
			name: "brands write", method: "PUT", path: "/v1/brands",
			reason:    "tenant-wide appearance, owner/admin only",
			refuseKey: true, refuseJWT: true, refuseAgent: true,
		},
		{
			name: "email channel read", method: "GET", path: "/v1/channels/email",
			reason:    "inbound mail configuration, owner/admin only",
			refuseKey: true, refuseJWT: true, refuseAgent: true,
		},
		{
			name: "add member", method: "POST", path: "/v1/conversations/c_probe/members",
			reason:    "membership is server-to-server; an operator adding people would bypass the host's identity namespace",
			refuseJWT: true, refuseAgent: true,
		},
		{
			name: "remap guest", method: "POST", path: "/v1/conversations/c_probe/members/remap",
			reason:    "reassigns a guest's history to a real user id",
			refuseJWT: true, refuseAgent: true,
		},
		{
			name: "register push token", method: "POST", path: "/v1/push-tokens",
			reason:    "a push token belongs to an end user's device, not to an operator or a key",
			refuseKey: true, refuseAgent: true,
		},
	}

	refused := func(code int) bool {
		return code == http.StatusUnauthorized || code == http.StatusForbidden
	}

	for _, tc := range cases {
		probe := func(label string, apply func(*testutil.Req) *testutil.Req) {
			t.Helper()
			code := apply(h.Request(tc.method, tc.path).JSON(map[string]any{})).Do().Code
			if !refused(code) {
				t.Errorf("%s: %s %s admitted a %s credential (%d) — %s",
					tc.name, tc.method, tc.path, label, code, tc.reason)
			}
		}
		if tc.refuseKey {
			probe("api key", func(r *testutil.Req) *testutil.Req { return r.Bearer(key) })
		}
		if tc.refuseJWT {
			probe("user JWT", func(r *testutil.Req) *testutil.Req { return r.Bearer(jwt) })
		}
		if tc.refuseAgent {
			probe("agent session", func(r *testutil.Req) *testutil.Req { return r.Cookie("sild_admin", agent) })
		}
	}
}
