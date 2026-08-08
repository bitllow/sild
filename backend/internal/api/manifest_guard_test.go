package api_test

import (
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/bitllow/sild/backend/internal/api"
	"github.com/bitllow/sild/backend/internal/principal"
	"github.com/bitllow/sild/backend/internal/store/models"
	"github.com/bitllow/sild/backend/internal/testutil"
)

// Matching the manifest on method+path says nothing about the group a route was
// mounted in, so these fire real requests: a credential kind a route does not
// declare must be refused. Group middleware runs ahead of every handler, so the
// verdict does not depend on the path params naming real objects.

// probePath substitutes params and wildcards with values the router will match.
func probePath(p string) string {
	p = strings.ReplaceAll(p, "*objectKey", "t_probe/o_probe/f.png")
	parts := strings.Split(p, "/")
	for i, seg := range parts {
		if strings.HasPrefix(seg, ":") {
			parts[i] = "probe"
		}
	}
	return strings.Join(parts, "/")
}

// probeEnv holds one credential of each kind, all valid, for one tenant — and one
// session per platform role, since an agent, an admin and an owner are all
// principal.KindAdmin.
type probeEnv struct {
	h     *testutil.Harness
	key   string
	jwt   string
	owner string
	admin string
	agent string
}

func newProbeEnv(t *testing.T) probeEnv {
	t.Helper()
	h := testutil.New(t)
	tenant := h.SeedTenant()
	h.SeedAdmin(tenant.ID, "owner@probe", models.PlatformOwner)
	h.SeedAdmin(tenant.ID, "admin@probe", models.PlatformAdmin)
	h.SeedAdmin(tenant.ID, "agent@probe", models.PlatformAgent)
	return probeEnv{
		h:     h,
		key:   h.SeedAPIKey(tenant.ID),
		jwt:   h.MintToken(tenant.ID, "u_probe"),
		owner: loginAs(t, h, "owner@probe"),
		admin: loginAs(t, h, "admin@probe"),
		agent: loginAs(t, h, "agent@probe"),
	}
}

func (e probeEnv) fire(method, path string, credential func(*testutil.Req) *testutil.Req) int {
	r := e.h.Request(method, path).JSON(map[string]any{})
	return credential(r).Do().Code
}

func TestMountedRoutesRefuseUndeclaredPrincipalKinds(t *testing.T) {
	e := newProbeEnv(t)
	byKind := map[principal.Kind]func(*testutil.Req) *testutil.Req{
		principal.KindAPIKey: func(r *testutil.Req) *testutil.Req { return r.Bearer(e.key) },
		principal.KindUser:   func(r *testutil.Req) *testutil.Req { return r.Bearer(e.jwt) },
		principal.KindAdmin:  func(r *testutil.Req) *testutil.Req { return r.Cookie("sild_admin", e.owner) },
	}

	for _, g := range api.RouteGuards() {
		for kind, credential := range byKind {
			if slices.Contains(g.Principals, kind) {
				continue
			}
			t.Run(g.Method+" "+g.Path+"/"+string(kind), func(t *testing.T) {
				code := e.fire(g.Method, probePath(g.Path), credential)
				if code != http.StatusUnauthorized && code != http.StatusForbidden {
					t.Errorf("declared for %v, but a %s credential got %d — want 401 or 403",
						g.Principals, kind, code)
				}
			})
		}
	}
}

// The role dimension Principals cannot express: every session below the tier a
// route declares must be refused.
func TestMountedRoutesRefuseSessionsBelowTheDeclaredRole(t *testing.T) {
	e := newProbeEnv(t)
	for _, tier := range []struct {
		role    string
		session string
		guards  func(api.RouteGuard) bool
	}{
		{"agent", e.agent, func(g api.RouteGuard) bool { return g.PrivilegedOnly }},
		{"admin", e.admin, func(g api.RouteGuard) bool { return g.OwnerOnly }},
	} {
		for _, g := range api.RouteGuards() {
			if !tier.guards(g) {
				continue
			}
			t.Run(tier.role+" "+g.Method+" "+g.Path, func(t *testing.T) {
				code := e.fire(g.Method, probePath(g.Path), func(r *testutil.Req) *testutil.Req {
					return r.Cookie("sild_admin", tier.session)
				})
				if code != http.StatusForbidden {
					t.Errorf("guards %v, but an %s session got %d — want 403", g.Actions, tier.role, code)
				}
			})
		}
	}
}
