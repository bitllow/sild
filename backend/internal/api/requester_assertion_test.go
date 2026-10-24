package api_test

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/golang-jwt/jwt/v5"

	"github.com/bitllow/sild/backend/internal/testutil"
)

func adapterBody(t *testing.T, conv, requester string) string {
	t.Helper()
	b, _ := json.Marshal(map[string]any{
		"id": "a3f1c9e2b7d84f0e", "requester": map[string]any{"id": requester}, "conversation_ref": conv,
		"source": "driver.devices", "params": map[string]any{"driver_id": "drv_2207"}, "reason": "no trip alerts",
	})
	return string(b)
}

func (f *requesterFixture) claim(t *testing.T, conv string) {
	t.Helper()
	w := f.h.Request("GET", "/v1/conversations/"+conv).Bearer(f.key).Do()
	var c struct {
		Assignment struct {
			ID string `json:"id"`
		} `json:"assignment"`
	}
	testutil.DecodeJSON(t, w, &c)
	if w := f.h.Request("PATCH", "/v1/assignments/"+c.Assignment.ID).Bearer(f.key).JSON(map[string]any{"assignee_actor_id": "me"}).Do(); w.Code != http.StatusOK {
		t.Fatalf("claim: %d %s", w.Code, w.Body)
	}
}

func (f *requesterFixture) mint(conv, body string) *httptest.ResponseRecorder {
	return f.h.Request("POST", "/v1/conversations/"+conv+"/requester-assertions").Bearer(f.key).JSON(map[string]any{"body": body}).Do()
}

func jwksKey(t *testing.T, h *testutil.Harness) func(*jwt.Token) (any, error) {
	t.Helper()
	w := h.Request("GET", "/.well-known/jwks.json").Do()
	var set struct {
		Keys []struct{ Kid, X, Y string } `json:"keys"`
	}
	testutil.DecodeJSON(t, w, &set)
	return func(tok *jwt.Token) (any, error) {
		for _, k := range set.Keys {
			if k.Kid != tok.Header["kid"] {
				continue
			}
			x, _ := base64.RawURLEncoding.DecodeString(k.X)
			y, _ := base64.RawURLEncoding.DecodeString(k.Y)
			return &ecdsa.PublicKey{Curve: elliptic.P256(), X: new(big.Int).SetBytes(x), Y: new(big.Int).SetBytes(y)}, nil
		}
		return nil, jwt.ErrTokenUnverifiable
	}
}

func TestRequesterAssertionVouchesForTheConversationsLinkedRequester(t *testing.T) {
	f := newRequesterFixture(t)
	if err := f.h.Svc.SetExternalSub(context.Background(), f.tenant.ID, f.markoID, "fleetline.7"); err != nil {
		t.Fatal(err)
	}
	body := adapterBody(t, f.own, "fleetline.7")
	if r := f.mint(f.own, body); r.Code != http.StatusForbidden {
		t.Fatalf("minted before holding the assignment: %d %s", r.Code, r.Body)
	}
	f.claim(t, f.own)

	r := f.mint(f.own, body)
	if r.Code != http.StatusCreated {
		t.Fatalf("mint: %d %s", r.Code, r.Body)
	}
	var out struct {
		Assertion string `json:"assertion"`
	}
	_ = json.Unmarshal(r.Body.Bytes(), &out)
	claims := jwt.MapClaims{}
	if _, err := jwt.ParseWithClaims(out.Assertion, claims, jwksKey(t, f.h), jwt.WithValidMethods([]string{"ES256"})); err != nil {
		t.Fatalf("assertion does not verify against the JWKS: %v", err)
	}
	sum := sha256.Sum256([]byte(body))
	want := map[string]any{
		"typ": "requester", "sub": "fleetline.7", "tid": f.tenant.ID, "conv": f.own,
		"rid": "a3f1c9e2b7d84f0e", "bsh": base64.RawURLEncoding.EncodeToString(sum[:]),
	}
	for k, v := range want {
		if claims[k] != v {
			t.Errorf("claim %s = %v, want %v", k, claims[k], v)
		}
	}

	if r := f.mint(f.own, adapterBody(t, f.own, "fleetline.8")); r.Code != http.StatusUnprocessableEntity {
		t.Fatalf("vouched for someone else: %d %s", r.Code, r.Body)
	}
}

func TestRequesterAssertionNeedsALinkedRequester(t *testing.T) {
	f := newRequesterFixture(t)
	f.claim(t, f.own)
	r := f.mint(f.own, adapterBody(t, f.own, "fleetline.7"))
	if r.Code != http.StatusConflict {
		t.Fatalf("unlinked requester: %d %s", r.Code, r.Body)
	}
}

func TestOnlyTheRequesterApprovesDataAccess(t *testing.T) {
	f := newRequesterFixture(t)
	approve := func(req *testutil.Req) int { return req.Do().Code }
	path := "/v1/conversations/" + f.own + "/approvals"
	if got := approve(f.h.Request("POST", path).Bearer(f.key)); got != http.StatusForbidden {
		t.Fatalf("the assistant approved: %d", got)
	}
	if got := approve(f.h.Request("POST", path).Cookie("sild_admin", f.liis)); got < 400 {
		t.Fatalf("another requester approved: %d", got)
	}
	w := f.h.Request("POST", path).Cookie("sild_admin", f.marko).Do()
	if w.Code != http.StatusCreated {
		t.Fatalf("approve: %d %s", w.Code, w.Body)
	}
	var m struct {
		SenderKind string           `json:"sender_kind"`
		Parts      []map[string]any `json:"parts"`
	}
	testutil.DecodeJSON(t, w, &m)
	if m.SenderKind != "system" || len(m.Parts) != 1 || m.Parts[0]["type"] != "approval" {
		t.Fatalf("approval message: %+v", m)
	}
}

func TestAStepMayWaitForApproval(t *testing.T) {
	f := newRequesterFixture(t)
	f.botMessage(t, []map[string]any{{"type": "steps", "steps": []map[string]any{{"label": "Checked his phones", "state": "needs_approval"}}}})
}
