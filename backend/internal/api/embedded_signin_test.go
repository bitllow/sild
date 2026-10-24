package api_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/bitllow/sild/backend/internal/store/models"
	"github.com/bitllow/sild/backend/internal/testutil"
)

const (
	hostOrigin     = "https://fleetline.example"
	validatorToken = "test-validator-token"
)

// fakeValidator implements the validator contract over a fixed table of
// integrator tokens; any other token is inactive.
type fakeValidator struct {
	srv *httptest.Server

	mu      sync.Mutex
	answers map[string]func(w http.ResponseWriter, r *http.Request)
	last    *http.Request
	body    string
}

func (v *fakeValidator) answer(token string, fn func(http.ResponseWriter, *http.Request)) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.answers[token] = fn
}

func newFakeValidator(t *testing.T) *fakeValidator {
	t.Helper()
	v := &fakeValidator{answers: map[string]func(http.ResponseWriter, *http.Request){}}
	v.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		v.mu.Lock()
		v.last, v.body = r, string(raw)
		v.mu.Unlock()
		if r.Header.Get("Authorization") != "Bearer "+validatorToken {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		var req struct {
			Token string `json:"token"`
		}
		_ = json.Unmarshal(raw, &req)
		v.mu.Lock()
		answer, ok := v.answers[req.Token]
		v.mu.Unlock()
		if ok {
			answer(w, r)
			return
		}
		reply(map[string]any{"active": false, "reason": "unknown_token"})(w, r)
	}))
	t.Cleanup(v.srv.Close)
	return v
}

func reply(body any) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(body)
	}
}

func active(sub string, exp time.Time) func(http.ResponseWriter, *http.Request) {
	return reply(map[string]any{"active": true, "sub": sub, "exp": exp.Unix(), "name": "Marko Saar"})
}

type embedFixture struct {
	h         *testutil.Harness
	tenant    *models.Tenant
	owner     string // owner session cookie
	marko     *models.AdminUser
	validator *fakeValidator
	embed     string
}

func newEmbedFixture(t *testing.T) *embedFixture {
	t.Helper()
	h := testutil.New(t)
	tenant := h.SeedTenant()
	h.SeedAdmin(tenant.ID, "owner@test", models.PlatformOwner)
	if err := h.Svc.SetQueue(context.Background(), &models.Queue{TenantID: tenant.ID, Key: "triage", Label: "Tech support"}); err != nil {
		t.Fatal(err)
	}
	marko := h.SeedAdminScoped(tenant.ID, "marko@test", models.PlatformRequester, models.RoleScope{Queues: []string{"triage"}})
	f := &embedFixture{h: h, tenant: tenant, owner: loginAs(t, h, "owner@test"), marko: marko, validator: newFakeValidator(t)}
	f.validator.answer("tok-marko", active("fleetline.7", time.Now().Add(2*time.Hour)))
	f.embed = f.configure(t, map[string]any{
		"validator_url": f.validator.srv.URL + "/validate", "validator_token": validatorToken,
		"allowed_origins": []string{hostOrigin},
	})
	f.link(t, marko.ID, "fleetline.7", http.StatusNoContent)
	return f
}

func (f *embedFixture) put(body map[string]any) *httptest.ResponseRecorder {
	return f.h.Request("PUT", "/v1/identity-provider").Cookie("sild_admin", f.owner).JSON(body).Do()
}

func (f *embedFixture) configure(t *testing.T, body map[string]any) string {
	t.Helper()
	w := f.put(body)
	if w.Code != http.StatusOK {
		t.Fatalf("configure identity provider: %d %s", w.Code, w.Body)
	}
	var v struct {
		EmbedKey string `json:"embed_key"`
	}
	testutil.DecodeJSON(t, w, &v)
	return v.EmbedKey
}

func (f *embedFixture) link(t *testing.T, adminID, sub string, want int) {
	t.Helper()
	w := f.h.Request("PUT", "/v1/team/"+adminID+"/external-sub").Cookie("sild_admin", f.owner).
		JSON(map[string]any{"external_sub": sub}).Do()
	if w.Code != want {
		t.Fatalf("link %s → %s: want %d, got %d %s", adminID, sub, want, w.Code, w.Body)
	}
}

type exchangeResult struct {
	code      int
	token     string
	expiresAt time.Time
	errCode   string
	reason    string
	sub       string
	body      string
}

func (f *embedFixture) exchange(t *testing.T, embedKey, token string) exchangeResult {
	t.Helper()
	w := f.h.Request("POST", "/v1/admin/auth/embedded").JSON(map[string]any{"embed_key": embedKey, "token": token}).Do()
	var body struct {
		Token     string    `json:"token"`
		ExpiresAt time.Time `json:"expires_at"`
		Error     struct {
			Code   string            `json:"code"`
			Fields map[string]string `json:"fields"`
		} `json:"error"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &body)
	return exchangeResult{code: w.Code, token: body.Token, expiresAt: body.ExpiresAt, errCode: body.Error.Code,
		reason: body.Error.Fields["reason"], sub: body.Error.Fields["sub"], body: w.Body.String()}
}

func (f *embedFixture) signIn(t *testing.T, token string) string {
	t.Helper()
	r := f.exchange(t, f.embed, token)
	if r.code != http.StatusOK || !strings.HasPrefix(r.token, "sild_emb_") {
		t.Fatalf("exchange: %d %s", r.code, r.body)
	}
	return r.token
}

func (f *embedFixture) sessions(t *testing.T) int64 {
	t.Helper()
	var n int64
	if err := f.h.DB.Model(&models.EmbedSession{}).Count(&n).Error; err != nil {
		t.Fatal(err)
	}
	return n
}

func TestEmbeddedSessionActsAsTheLinkedMember(t *testing.T) {
	f := newEmbedFixture(t)
	bearer := f.signIn(t, "tok-marko")

	w := f.h.Request("GET", "/v1/principal").Bearer(bearer).Do()
	var me struct {
		Kind     string `json:"kind"`
		TenantID string `json:"tenant_id"`
		Subject  struct {
			ID string `json:"id"`
		} `json:"subject"`
	}
	testutil.DecodeJSON(t, w, &me)
	if w.Code != http.StatusOK || me.Kind != "admin" || me.Subject.ID != f.marko.ID || me.TenantID != f.tenant.ID {
		t.Fatalf("principal: %d %+v", w.Code, me)
	}
	if w := f.h.Request("GET", "/v1/conversations?queue=triage").Bearer(bearer).Do(); w.Code != http.StatusOK {
		t.Fatalf("list triage: %d %s", w.Code, w.Body)
	}
	if w := f.h.Request("POST", "/v1/conversations").Bearer(bearer).
		JSON(map[string]any{"queue": "triage", "open_assignment": true}).Do(); w.Code != http.StatusCreated {
		t.Fatalf("open triage: %d %s", w.Code, w.Body)
	}
	// The member's roles still bind: a requester is no team manager.
	if w := f.h.Request("GET", "/v1/team").Bearer(bearer).Do(); w.Code != http.StatusForbidden {
		t.Fatalf("team as requester: want 403, got %d", w.Code)
	}
}

func TestEmbeddedSessionOpensRealtime(t *testing.T) {
	f := newEmbedFixture(t)
	bearer := f.signIn(t, "tok-marko")
	w := f.h.Request("GET", "/v1/realtime/token").Bearer(bearer).Do()
	var rt struct {
		Token string `json:"token"`
	}
	testutil.DecodeJSON(t, w, &rt)
	if w.Code != http.StatusOK {
		t.Fatalf("realtime token: %d %s", w.Code, w.Body)
	}
	claims, err := f.h.KM.VerifyRealtime(context.Background(), rt.Token)
	if err != nil || claims.Subject != f.marko.ID || claims.Tid != f.tenant.ID {
		t.Fatalf("realtime token does not verify as the member: %v %+v", err, claims)
	}
}

func TestEmbeddedSessionIsNotAUserToken(t *testing.T) {
	f := newEmbedFixture(t)
	bearer := f.signIn(t, "tok-marko")
	if _, err := f.h.KM.Verify(context.Background(), bearer); err == nil {
		t.Fatal("user-JWT verifier accepted an embedded bearer")
	}
	for _, r := range []struct{ method, path string }{
		{"PUT", "/v1/contacts/me"},
		{"POST", "/v1/push-tokens"},
		{"POST", "/v1/tokens"},
	} {
		if w := f.h.Request(r.method, r.path).Bearer(bearer).JSON(map[string]any{}).Do(); w.Code != http.StatusUnauthorized {
			t.Errorf("%s %s with an embedded bearer: want 401, got %d", r.method, r.path, w.Code)
		}
	}
}

func TestValidatorReceivesOurBearerAndTheToken(t *testing.T) {
	f := newEmbedFixture(t)
	f.signIn(t, "tok-marko")
	f.validator.mu.Lock()
	defer f.validator.mu.Unlock()
	r := f.validator.last
	if r.Method != http.MethodPost || r.URL.Path != "/validate" ||
		r.Header.Get("Authorization") != "Bearer "+validatorToken ||
		r.Header.Get("Content-Type") != "application/json" ||
		f.validator.body != `{"token":"tok-marko"}` {
		t.Fatalf("validator got %s %s auth=%q type=%q body=%s", r.Method, r.URL.Path,
			r.Header.Get("Authorization"), r.Header.Get("Content-Type"), f.validator.body)
	}
}

func TestEmbeddedSignInRefusesWithAReason(t *testing.T) {
	f := newEmbedFixture(t)
	bare := &models.AdminUser{TenantID: f.tenant.ID, Email: "noroles@test"}
	if err := f.h.Store.Admins().Create(context.Background(), bare); err != nil {
		t.Fatal(err)
	}
	f.link(t, bare.ID, "fleetline.12", http.StatusNoContent)
	hour := time.Now().Add(time.Hour)
	f.validator.answer("tok-driver", reply(map[string]any{"active": false, "reason": "role_not_allowed", "sub": "fleetline.4411"}))
	f.validator.answer("tok-bare-inactive", reply(map[string]any{"active": false}))
	f.validator.answer("tok-unlinked", active("fleetline.40", hour))
	f.validator.answer("tok-noroles", active("fleetline.12", hour))
	f.validator.answer("tok-expired", active("fleetline.7", time.Now().Add(-time.Minute)))
	f.validator.answer("tok-email", active("marko@test", hour))

	cases := []struct{ token, reason, sub string }{
		{"tok-driver", "role_not_allowed", "fleetline.4411"},
		{"tok-bare-inactive", "inactive", ""},
		{"tok-unlinked", "user_not_provisioned", "fleetline.40"},
		{"tok-noroles", "no_grants", "fleetline.12"},
		{"tok-expired", "token_expired", "fleetline.7"},
		// Never matched by email: a sub that happens to be one is just an unknown identity.
		{"tok-email", "user_not_provisioned", "marko@test"},
	}
	for _, tc := range cases {
		t.Run(tc.token, func(t *testing.T) {
			r := f.exchange(t, f.embed, tc.token)
			if r.code != http.StatusForbidden || r.errCode != "access_denied" || r.reason != tc.reason || r.sub != tc.sub {
				t.Fatalf("want 403 access_denied %s sub=%s, got %d %s", tc.reason, tc.sub, r.code, r.body)
			}
		})
	}
	if n := f.sessions(t); n != 0 {
		t.Fatalf("a refused sign-in left %d sessions", n)
	}
}

func TestEmbeddedSignInFailsClosedWhenTheValidatorFails(t *testing.T) {
	f := newEmbedFixture(t)
	f.h.Svc.SetValidatorTimeout(200 * time.Millisecond)
	raw := func(status int, body string) func(http.ResponseWriter, *http.Request) {
		return func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(status)
			_, _ = io.WriteString(w, body)
		}
	}
	answers := map[string]func(http.ResponseWriter, *http.Request){
		"tok-500":       raw(http.StatusInternalServerError, `{"active":true,"sub":"fleetline.7"}`),
		"tok-bad-json":  raw(http.StatusOK, `active: true`),
		"tok-no-active": reply(map[string]any{"sub": "fleetline.7"}),
		"tok-no-sub":    reply(map[string]any{"active": true}),
		"tok-reason":    reply(map[string]any{"active": false, "reason": "<b>no</b>"}),
		"tok-redirect": func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "/validate", http.StatusTemporaryRedirect)
		},
		"tok-slow": func(w http.ResponseWriter, r *http.Request) {
			select {
			case <-r.Context().Done():
			case <-time.After(2 * time.Second):
			}
			active("fleetline.7", time.Now().Add(time.Hour))(w, r)
		},
	}
	for token, answer := range answers {
		f.validator.answer(token, answer)
		t.Run(token, func(t *testing.T) {
			if r := f.exchange(t, f.embed, token); r.code != http.StatusBadGateway || r.errCode != "validator_failed" || r.token != "" {
				t.Fatalf("want 502 validator_failed, got %d %s", r.code, r.body)
			}
		})
	}
	t.Run("validator refuses our API token", func(t *testing.T) {
		f.configure(t, map[string]any{
			"validator_url": f.validator.srv.URL + "/validate", "validator_token": "stale-token",
			"allowed_origins": []string{hostOrigin},
		})
		if r := f.exchange(t, f.embed, "tok-marko"); r.code != http.StatusBadGateway || r.errCode != "validator_failed" {
			t.Fatalf("want 502 validator_failed, got %d %s", r.code, r.body)
		}
	})
	t.Run("unreachable", func(t *testing.T) {
		f.configure(t, map[string]any{
			"validator_url": "http://127.0.0.1:1/validate", "validator_token": validatorToken,
			"allowed_origins": []string{hostOrigin},
		})
		if r := f.exchange(t, f.embed, "tok-marko"); r.code != http.StatusBadGateway {
			t.Fatalf("want 502, got %d %s", r.code, r.body)
		}
	})
	if n := f.sessions(t); n != 0 {
		t.Fatalf("a failed validator left %d sessions", n)
	}
	if r := f.exchange(t, "emb_unknown", "tok-marko"); r.code != http.StatusUnauthorized {
		t.Errorf("unknown embed key: want 401, got %d", r.code)
	}
}

func TestIdentityProviderConfig(t *testing.T) {
	f := newEmbedFixture(t)
	url := f.validator.srv.URL + "/validate"

	w := f.h.Request("GET", "/v1/identity-provider").Cookie("sild_admin", f.owner).Do()
	if w.Code != http.StatusOK || strings.Contains(w.Body.String(), validatorToken) ||
		!strings.Contains(w.Body.String(), `"has_validator_token":true`) || !strings.Contains(w.Body.String(), url) {
		t.Fatalf("config read leaks or loses the token: %s", w.Body)
	}
	t.Run("omitting the token keeps it for the same URL", func(t *testing.T) {
		f.configure(t, map[string]any{"validator_url": url, "allowed_origins": []string{hostOrigin}})
		f.signIn(t, "tok-marko")
	})
	t.Run("a new URL needs a new token", func(t *testing.T) {
		w := f.put(map[string]any{"validator_url": "https://elsewhere.example/validate", "allowed_origins": []string{hostOrigin}})
		if w.Code != http.StatusUnprocessableEntity {
			t.Fatalf("want 422, got %d %s", w.Code, w.Body)
		}
	})
	t.Run("rejects", func(t *testing.T) {
		for name, body := range map[string]map[string]any{
			"empty token":         {"validator_url": url, "validator_token": "", "allowed_origins": []string{hostOrigin}},
			"token with newline":  {"validator_url": url, "validator_token": "a\nb", "allowed_origins": []string{hostOrigin}},
			"relative url":        {"validator_url": "/validate", "validator_token": "x", "allowed_origins": []string{hostOrigin}},
			"http off loopback":   {"validator_url": "http://fleetline.example/validate", "validator_token": "x", "allowed_origins": []string{hostOrigin}},
			"no origins":          {"validator_url": url, "validator_token": "x"},
			"retired method":      {"method": "jwks", "validator_url": url, "validator_token": "x", "allowed_origins": []string{hostOrigin}},
			"client-set embedkey": {"embed_key": "emb_mine", "validator_url": url, "validator_token": "x", "allowed_origins": []string{hostOrigin}},
		} {
			if w := f.put(body); w.Code != http.StatusUnprocessableEntity && w.Code != http.StatusBadRequest {
				t.Errorf("%s: want 4xx, got %d %s", name, w.Code, w.Body)
			}
		}
	})
}

func TestValidatorURLMustBeHTTPSOutsideDevelopment(t *testing.T) {
	f := newEmbedFixture(t)
	body := func(u string) map[string]any {
		return map[string]any{"validator_url": u, "validator_token": "x", "allowed_origins": []string{hostOrigin}}
	}
	f.h.Cfg.Env = "production"
	t.Cleanup(func() { f.h.Cfg.Env = "test" })
	if w := f.put(body(f.validator.srv.URL + "/validate")); w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("http to loopback in production: want 422, got %d %s", w.Code, w.Body)
	}
	if w := f.put(body("https://fleetline.example/sild/validate")); w.Code != http.StatusOK {
		t.Fatalf("https in production: want 200, got %d %s", w.Code, w.Body)
	}
}

func TestEmbeddedSessionIsTenantBound(t *testing.T) {
	f := newEmbedFixture(t)
	other := f.h.SeedTenant()
	f.h.SeedAdmin(other.ID, "owner@other", models.PlatformOwner)
	twin := f.h.SeedAdmin(other.ID, "twin@other", models.PlatformAdmin)
	if err := f.h.Svc.SetExternalSub(context.Background(), other.ID, twin.ID, "fleetline.7"); err != nil {
		t.Fatal(err)
	}
	otherConv := f.h.Request("POST", "/v1/conversations").Bearer(f.h.SeedAPIKey(other.ID)).
		JSON(map[string]any{"members": []map[string]any{{"user_id": "u_other"}}}).Do()
	var conv struct {
		ID string `json:"id"`
	}
	testutil.DecodeJSON(t, otherConv, &conv)

	bearer := f.signIn(t, "tok-marko")
	w := f.h.Request("GET", "/v1/principal").Bearer(bearer).Do()
	if !strings.Contains(w.Body.String(), f.tenant.ID) || strings.Contains(w.Body.String(), other.ID) {
		t.Fatalf("session resolved outside its tenant: %s", w.Body)
	}
	if w := f.h.Request("GET", "/v1/conversations/"+conv.ID).Bearer(bearer).Do(); w.Code == http.StatusOK {
		t.Fatalf("read another tenant's conversation: %s", w.Body)
	}
}

func TestEmbeddedSessionEnds(t *testing.T) {
	f := newEmbedFixture(t)
	ctx := context.Background()

	t.Run("never outlives the validator's exp", func(t *testing.T) {
		exp := time.Now().Add(2 * time.Minute).Truncate(time.Second)
		f.validator.answer("tok-short", active("fleetline.7", exp))
		r := f.exchange(t, f.embed, "tok-short")
		if r.code != http.StatusOK || !r.expiresAt.Equal(exp) {
			t.Fatalf("session expires %v, validator said %v (%d)", r.expiresAt, exp, r.code)
		}
	})
	t.Run("capped at an hour", func(t *testing.T) {
		r := f.exchange(t, f.embed, "tok-marko")
		if r.code != http.StatusOK || r.expiresAt.After(time.Now().Add(time.Hour)) {
			t.Fatalf("session expires %v (%d)", r.expiresAt, r.code)
		}
	})
	t.Run("expired", func(t *testing.T) {
		bearer := f.signIn(t, "tok-marko")
		if err := f.h.DB.Model(&models.EmbedSession{}).Where("admin_user_id = ?", f.marko.ID).
			Update("expires_at", time.Now().Add(-time.Second)).Error; err != nil {
			t.Fatal(err)
		}
		if w := f.h.Request("GET", "/v1/principal").Bearer(bearer).Do(); w.Code != http.StatusUnauthorized {
			t.Fatalf("expired session: want 401, got %d", w.Code)
		}
	})
	t.Run("relinked", func(t *testing.T) {
		bearer := f.signIn(t, "tok-marko")
		if err := f.h.Svc.SetExternalSub(ctx, f.tenant.ID, f.marko.ID, "fleetline.8"); err != nil {
			t.Fatal(err)
		}
		if w := f.h.Request("GET", "/v1/principal").Bearer(bearer).Do(); w.Code != http.StatusUnauthorized {
			t.Fatalf("relinked member's session: want 401, got %d", w.Code)
		}
		f.link(t, f.marko.ID, "fleetline.7", http.StatusNoContent)
	})
	t.Run("logged out", func(t *testing.T) {
		bearer := f.signIn(t, "tok-marko")
		f.h.Request("POST", "/v1/admin/auth/logout").Bearer(bearer).Do()
		if w := f.h.Request("GET", "/v1/principal").Bearer(bearer).Do(); w.Code != http.StatusUnauthorized {
			t.Fatalf("logged-out session: want 401, got %d", w.Code)
		}
	})
	t.Run("provider removed", func(t *testing.T) {
		bearer := f.signIn(t, "tok-marko")
		if w := f.h.Request("DELETE", "/v1/identity-provider").Cookie("sild_admin", f.owner).Do(); w.Code != http.StatusNoContent {
			t.Fatalf("delete provider: %d", w.Code)
		}
		if w := f.h.Request("GET", "/v1/principal").Bearer(bearer).Do(); w.Code != http.StatusUnauthorized {
			t.Fatalf("session after provider removal: want 401, got %d", w.Code)
		}
	})
}

func TestEmbedOriginsRevealOnlyOrigins(t *testing.T) {
	f := newEmbedFixture(t)
	w := f.h.Request("GET", "/v1/admin/auth/embedded/"+f.embed).Do()
	var body map[string]any
	testutil.DecodeJSON(t, w, &body)
	if w.Code != http.StatusOK || len(body) != 1 {
		t.Fatalf("origins: %d %s", w.Code, w.Body)
	}
	if got := body["allowed_origins"].([]any); len(got) != 1 || got[0] != hostOrigin {
		t.Fatalf("origins: %v", got)
	}
	if w := f.h.Request("GET", "/v1/admin/auth/embedded/emb_unknown").Do(); w.Code != http.StatusNotFound {
		t.Fatalf("unknown key: want 404, got %d", w.Code)
	}
}

func TestExternalSubLinking(t *testing.T) {
	f := newEmbedFixture(t)
	other := f.h.SeedAdmin(f.tenant.ID, "liis@test", models.PlatformAgent)
	f.link(t, other.ID, "fleetline.7", http.StatusConflict)

	svc, err := f.h.Svc.CreateServiceAccount(context.Background(), f.tenant.ID, "assistant", "Assistant",
		models.PlatformAgent, models.RoleScope{Queues: []string{"triage"}})
	if err != nil {
		t.Fatal(err)
	}
	f.link(t, svc.ID, "fleetline.99", http.StatusUnprocessableEntity)

	w := f.h.Request("GET", "/v1/team").Cookie("sild_admin", f.owner).Do()
	if !strings.Contains(w.Body.String(), `"external_sub":"fleetline.7"`) {
		t.Fatalf("team list omits the link: %s", w.Body)
	}
}

func TestEmbeddedOnlyRefusesOtherSignInForLinkedMembers(t *testing.T) {
	f := newEmbedFixture(t)
	ctx := context.Background()
	owner := f.h.SeedAdmin(f.tenant.ID, "eva@test", models.PlatformOwner)
	for _, id := range []string{f.marko.ID, owner.ID} {
		if err := f.h.Svc.SetAdminPassword(ctx, f.tenant.ID, id, "password123"); err != nil {
			t.Fatal(err)
		}
	}
	login := func(email string) int {
		return f.h.Request("POST", "/v1/admin/auth/password").JSON(map[string]any{"email": email, "password": "password123"}).Do().Code
	}
	if got := login("marko@test"); got != http.StatusOK {
		t.Fatalf("linked member before embedded_only: %d", got)
	}
	earlier := loginAs(t, f.h, "marko@test")

	f.configure(t, map[string]any{
		"validator_url": f.validator.srv.URL + "/validate", "allowed_origins": []string{hostOrigin}, "embedded_only": true,
	})
	if got := login("marko@test"); got != http.StatusForbidden {
		t.Fatalf("linked member signed in with a password: %d", got)
	}
	if got := login("eva@test"); got != http.StatusOK {
		t.Fatalf("unlinked owner lost the break-glass login: %d", got)
	}
	if w := f.h.Request("GET", "/v1/principal").Cookie("sild_admin", earlier).Do(); w.Code != http.StatusUnauthorized {
		t.Fatalf("a session from before embedded_only still works: %d", w.Code)
	}
	f.signIn(t, "tok-marko")
}
