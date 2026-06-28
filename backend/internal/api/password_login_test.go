package api_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/bitllow/sild/backend/internal/store/models"
	"github.com/bitllow/sild/backend/internal/testutil"
)

// §2.4: email/password is an alternative admin login. A valid credential mints a
// usable session; a wrong password is rejected.
func TestAdminPasswordLogin(t *testing.T) {
	h := testutil.New(t)
	tenant := h.SeedTenant()
	admin := h.SeedAdmin(tenant.ID, "owner@test", models.PlatformOwner)
	if err := h.Svc.SetAdminPassword(context.Background(), tenant.ID, admin.ID, "correct-horse"); err != nil {
		t.Fatal(err)
	}

	// wrong password → 401
	w := h.Request("POST", "/v1/admin/auth/password").
		JSON(map[string]any{"email": "owner@test", "password": "nope"}).Do()
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("wrong password should be 401, got %d %s", w.Code, w.Body)
	}

	// correct password → session cookie
	w = h.Request("POST", "/v1/admin/auth/password").
		JSON(map[string]any{"email": "owner@test", "password": "correct-horse"}).Do()
	if w.Code != http.StatusOK {
		t.Fatalf("password login: %d %s", w.Code, w.Body)
	}
	cookie := extractCookie(w.Header().Get("Set-Cookie"))
	if cookie == "" {
		t.Fatal("expected a session cookie")
	}

	// the session works on an admin route
	w = h.Request("GET", "/v1/admin/assignments").Cookie("sild_admin", cookie).Do()
	if w.Code != http.StatusOK {
		t.Fatalf("session should authorize admin routes, got %d %s", w.Code, w.Body)
	}

	// an admin without a password set cannot log in by password
	h.SeedAdmin(tenant.ID, "nopass@test", models.PlatformAgent)
	w = h.Request("POST", "/v1/admin/auth/password").
		JSON(map[string]any{"email": "nopass@test", "password": "anything"}).Do()
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("passwordless admin must not log in by password, got %d %s", w.Code, w.Body)
	}
}
