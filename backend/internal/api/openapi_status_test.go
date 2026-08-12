package api_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/bitllow/sild/backend/internal/api"
	"github.com/bitllow/sild/backend/internal/store/models"
	"github.com/bitllow/sild/backend/internal/testutil"
)

// The document's success status comes from the manifest, so a wrong declaration
// publishes a lie a generated client would branch on. This drives real requests
// through the satisfied path and compares.
//
// Coverage is the routes reachable with fixture state, which is most of the
// mutating surface; the OAuth redirects and signed-URL paths are exercised by
// their own tests.
func TestDeclaredSuccessStatusMatchesReality(t *testing.T) {
	h := testutil.New(t)
	tenant := h.SeedTenant()
	key := h.SeedAPIKey(tenant.ID)
	jwt := h.MintToken(tenant.ID, "u_probe")
	h.SeedAdmin(tenant.ID, "owner@probe", models.PlatformOwner)
	owner := loginAs(t, h, "owner@probe")

	var conv struct {
		ID string `json:"id"`
	}
	testutil.DecodeJSON(t, h.Request("POST", "/v1/conversations").
		Bearer(key).JSON(map[string]any{"members": []map[string]any{{"user_id": "u_probe"}}}).Do(), &conv)

	var webhook, apiKey, agent struct {
		ID string `json:"id"`
	}
	testutil.DecodeJSON(t, h.Request("POST", "/v1/webhooks").Cookie("sild_admin", owner).
		JSON(map[string]any{"url": "https://example.test/hook", "events": []string{"message.created"}}).Do(), &webhook)
	testutil.DecodeJSON(t, h.Request("POST", "/v1/api-keys").Cookie("sild_admin", owner).
		JSON(map[string]any{"label": "probe"}).Do(), &apiKey)
	testutil.DecodeJSON(t, h.Request("POST", "/v1/team").Cookie("sild_admin", owner).
		JSON(map[string]any{"email": "probe-agent@test", "role": "agent"}).Do(), &agent)

	var msg struct {
		ID string `json:"id"`
	}
	testutil.DecodeJSON(t, h.Request("POST", "/v1/conversations/"+conv.ID+"/messages").
		Bearer(jwt).JSON(map[string]any{"body": "seed"}).Do(), &msg)

	brandsRes := h.Request("GET", "/v1/brands").Cookie("sild_admin", owner).Do()
	etag := brandsRes.Header().Get("ETag")
	var brands struct {
		Brands        []map[string]any `json:"brands"`
		ActiveBrandID string           `json:"active_brand_id"`
	}
	testutil.DecodeJSON(t, brandsRes, &brands)

	// Each entry is a route from the manifest driven down its satisfied path.
	cases := []struct {
		method, path string
		req          func(*testutil.Req) *testutil.Req
	}{
		{"GET", "/v1/conversations", func(r *testutil.Req) *testutil.Req { return r.Bearer(jwt) }},
		{"POST", "/v1/conversations", func(r *testutil.Req) *testutil.Req {
			return r.Bearer(jwt).JSON(map[string]any{})
		}},
		{"GET", "/v1/conversations/:id", func(r *testutil.Req) *testutil.Req { return r.Bearer(jwt) }},
		{"GET", "/v1/conversations/:id/messages", func(r *testutil.Req) *testutil.Req { return r.Bearer(jwt) }},
		{"POST", "/v1/conversations/:id/messages", func(r *testutil.Req) *testutil.Req {
			return r.Bearer(jwt).JSON(map[string]any{"body": "hi"})
		}},
		{"POST", "/v1/conversations/:id/read", func(r *testutil.Req) *testutil.Req {
			return r.Bearer(jwt).JSON(map[string]any{"last_read_message_id": msg.ID})
		}},
		{"POST", "/v1/conversations/:id/typing", func(r *testutil.Req) *testutil.Req {
			return r.Bearer(jwt).JSON(map[string]any{})
		}},
		{"POST", "/v1/conversations/:id/members", func(r *testutil.Req) *testutil.Req {
			return r.Bearer(key).JSON(map[string]any{"user_id": "u_second"})
		}},
		{"POST", "/v1/conversations/:id/members/remap", func(r *testutil.Req) *testutil.Req {
			return r.Bearer(key).JSON(map[string]any{"from_user_id": "u_second", "to_user_id": "u_third"})
		}},
		{"DELETE", "/v1/conversations/:id/members/:user_id", func(r *testutil.Req) *testutil.Req { return r.Bearer(key) }},
		{"POST", "/v1/conversations/:id/close", func(r *testutil.Req) *testutil.Req {
			return r.Bearer(key).JSON(map[string]any{})
		}},
		{"GET", "/v1/principal", func(r *testutil.Req) *testutil.Req { return r.Bearer(jwt) }},
		{"GET", "/v1/realtime/token", func(r *testutil.Req) *testutil.Req { return r.Cookie("sild_admin", owner) }},
		{"GET", "/v1/contacts", func(r *testutil.Req) *testutil.Req { return r.Cookie("sild_admin", owner) }},
		{"POST", "/v1/uploads", func(r *testutil.Req) *testutil.Req {
			return r.Bearer(jwt).JSON(map[string]any{"mime_type": "image/png", "size_bytes": 10, "filename": "a.png"})
		}},
		{"POST", "/v1/tokens", func(r *testutil.Req) *testutil.Req {
			return r.Bearer(key).JSON(map[string]any{"user_id": "u_minted"})
		}},
		{"POST", "/v1/push-tokens", func(r *testutil.Req) *testutil.Req {
			return r.Bearer(jwt).JSON(map[string]any{"token": "t", "platform": "ios"})
		}},
		{"DELETE", "/v1/push-tokens", func(r *testutil.Req) *testutil.Req {
			return r.Bearer(jwt).JSON(map[string]any{"token": "t"})
		}},
		{"GET", "/v1/brands/active", func(r *testutil.Req) *testutil.Req { return r.Bearer(jwt) }},
		{"GET", "/v1/brands", func(r *testutil.Req) *testutil.Req { return r.Cookie("sild_admin", owner) }},
		{"GET", "/v1/channels/email", func(r *testutil.Req) *testutil.Req { return r.Cookie("sild_admin", owner) }},
		{"PATCH", "/v1/channels/email", func(r *testutil.Req) *testutil.Req {
			return r.Cookie("sild_admin", owner).Header("If-Match", "*").JSON(map[string]any{})
		}},
		{"GET", "/v1/api-keys", func(r *testutil.Req) *testutil.Req { return r.Cookie("sild_admin", owner) }},
		{"POST", "/v1/api-keys", func(r *testutil.Req) *testutil.Req {
			return r.Cookie("sild_admin", owner).JSON(map[string]any{"label": "another"})
		}},
		{"GET", "/v1/webhooks", func(r *testutil.Req) *testutil.Req { return r.Cookie("sild_admin", owner) }},
		{"GET", "/v1/webhooks/:id/deliveries", func(r *testutil.Req) *testutil.Req { return r.Cookie("sild_admin", owner) }},
		{"PATCH", "/v1/webhooks/:id", func(r *testutil.Req) *testutil.Req {
			return r.Cookie("sild_admin", owner).JSON(map[string]any{"active": false})
		}},
		{"GET", "/v1/team", func(r *testutil.Req) *testutil.Req { return r.Cookie("sild_admin", owner) }},
		{"GET", "/v1/roles", func(r *testutil.Req) *testutil.Req { return r.Cookie("sild_admin", owner) }},
		{"POST", "/v1/team/:id/roles", func(r *testutil.Req) *testutil.Req {
			return r.Cookie("sild_admin", owner).JSON(map[string]any{"role": "admin"})
		}},
		{"PUT", "/v1/team/:id/roles/:role", func(r *testutil.Req) *testutil.Req {
			return r.Cookie("sild_admin", owner).JSON(map[string]any{"scope": map[string]any{}})
		}},
		{"DELETE", "/v1/team/:id/roles/:role", func(r *testutil.Req) *testutil.Req {
			return r.Cookie("sild_admin", owner)
		}},
		{"POST", "/v1/team/:id/password", func(r *testutil.Req) *testutil.Req {
			return r.Cookie("sild_admin", owner).JSON(map[string]any{"password": "correct horse battery staple"})
		}},
		{"PUT", "/v1/brands", func(r *testutil.Req) *testutil.Req {
			return r.Cookie("sild_admin", owner).Header("If-Match", etag).
				JSON(map[string]any{"brands": brands.Brands, "active_brand_id": brands.ActiveBrandID})
		}},
		// Destructive last, so the ids above stay usable.
		{"DELETE", "/v1/webhooks/:id", func(r *testutil.Req) *testutil.Req { return r.Cookie("sild_admin", owner) }},
		{"DELETE", "/v1/api-keys/:id", func(r *testutil.Req) *testutil.Req { return r.Cookie("sild_admin", owner) }},
		{"POST", "/v1/admin/auth/logout", func(r *testutil.Req) *testutil.Req { return r.Cookie("sild_admin", owner) }},
	}

	declared := map[string]int{}
	for _, s := range api.RouteSuccessStatuses() {
		declared[s.Method+" "+s.Path] = s.Status
	}

	subst := map[string]string{
		":id": conv.ID, ":user_id": "u_third", ":external_user_id": "u_probe",
	}

	for _, tc := range cases {
		key := tc.method + " " + tc.path
		want, ok := declared[key]
		if !ok {
			t.Errorf("%s is exercised here but absent from the manifest", key)
			continue
		}
		path := tc.path
		switch {
		case tc.path == "/v1/webhooks/:id" || tc.path == "/v1/webhooks/:id/deliveries":
			path = replaceParam(tc.path, ":id", webhook.ID)
		case tc.path == "/v1/api-keys/:id":
			path = replaceParam(tc.path, ":id", apiKey.ID)
		case strings.HasPrefix(tc.path, "/v1/team/:id"):
			path = replaceParam(tc.path, ":id", agent.ID)
			path = replaceParam(path, ":role", "agent")
		default:
			for param, value := range subst {
				path = replaceParam(path, param, value)
			}
		}

		w := tc.req(h.Request(tc.method, path)).Do()
		if w.Code != want {
			t.Errorf("%s answered %d but the manifest declares %d (body: %s)",
				key, w.Code, want, truncate(w.Body.String()))
		}
	}
}

func replaceParam(path, param, value string) string {
	if value == "" {
		return path
	}
	out := ""
	for _, seg := range splitPath(path) {
		if seg == param {
			seg = value
		}
		out += "/" + seg
	}
	return out
}

func splitPath(p string) []string {
	var out []string
	for _, seg := range []byte(p) {
		_ = seg
		break
	}
	start := 0
	for i := 0; i <= len(p); i++ {
		if i == len(p) || p[i] == '/' {
			if i > start {
				out = append(out, p[start:i])
			}
			start = i + 1
		}
	}
	return out
}

func truncate(s string) string {
	if len(s) > 160 {
		return s[:160] + "…"
	}
	return s
}

var _ = http.StatusOK
