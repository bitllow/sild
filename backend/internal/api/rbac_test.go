package api_test

import (
	"context"
	"net/http"
	"sync"
	"testing"

	"github.com/bitllow/sild/backend/internal/store/models"
	"github.com/bitllow/sild/backend/internal/testutil"
)

// rbacFixture is a tenant with an owner signed in, which is what every team
// mutation needs before it can do anything at all.
type rbacFixture struct {
	h      *testutil.Harness
	tenant *models.Tenant
	owner  string
}

func newRBACFixture(t *testing.T) *rbacFixture {
	t.Helper()
	h := testutil.New(t)
	tenant := h.SeedTenant()
	h.SeedAdmin(tenant.ID, "owner@test", models.PlatformOwner)
	return &rbacFixture{h: h, tenant: tenant, owner: loginAs(t, h, "owner@test")}
}

func (f *rbacFixture) assign(t *testing.T, member string, body map[string]any) int {
	t.Helper()
	return f.h.Request("POST", "/v1/team/"+member+"/roles").
		Cookie("sild_admin", f.owner).JSON(body).Do().Code
}

func (f *rbacFixture) rolesOf(t *testing.T, member string) map[models.PlatformRole]models.RoleScope {
	t.Helper()
	held, err := f.h.Svc.RoleAssignments(context.Background(), f.tenant.ID, member)
	if err != nil {
		t.Fatalf("assignments: %v", err)
	}
	out := map[models.PlatformRole]models.RoleScope{}
	for _, a := range held {
		out[a.Role] = a.Scope
	}
	return out
}

// The case the single role column could not express.
func TestAMemberWorksTheQueueAndTranslatesAtOnce(t *testing.T) {
	f := newRBACFixture(t)
	member := f.h.SeedAdmin(f.tenant.ID, "both@test", models.PlatformAgent)
	if code := f.assign(t, member.ID, map[string]any{
		"role":  models.PlatformTranslator,
		"scope": map[string]any{"projects": []string{models.ScopeAll}, "locales": []string{"lv"}},
	}); code != http.StatusNoContent {
		t.Fatalf("adding a second role = %d", code)
	}
	cookie := loginAs(t, f.h, "both@test")

	if w := f.h.Request("GET", "/v1/conversations").Cookie("sild_admin", cookie).Do(); w.Code != http.StatusOK {
		t.Fatalf("the agent role stopped working: %d %s", w.Code, w.Body)
	}
	write := map[string]any{"locale": "lv", "value": "Runā ar mums"}
	if w := f.h.Request("PUT", "/v1/translations/projects/sild/keys/widget.home.title").
		Cookie("sild_admin", cookie).JSON(write).Do(); w.Code != http.StatusNoContent {
		t.Fatalf("the translator role does not reach translations: %d %s", w.Code, w.Body)
	}

	// Dropping one leaves the other standing.
	if w := f.h.Request("DELETE", "/v1/team/"+member.ID+"/roles/translator").
		Cookie("sild_admin", f.owner).Do(); w.Code != http.StatusNoContent {
		t.Fatalf("removing a role = %d %s", w.Code, w.Body)
	}
	cookie = loginAs(t, f.h, "both@test")
	if w := f.h.Request("GET", "/v1/conversations").Cookie("sild_admin", cookie).Do(); w.Code != http.StatusOK {
		t.Fatalf("removing the translator role cost them the queue: %d %s", w.Code, w.Body)
	}
	if w := f.h.Request("PUT", "/v1/translations/projects/sild/keys/widget.home.title").
		Cookie("sild_admin", cookie).JSON(write).Do(); w.Code != http.StatusForbidden {
		t.Fatalf("a removed role still writes translations: %d %s", w.Code, w.Body)
	}
}

func TestARoleIsHeldAtMostOnce(t *testing.T) {
	f := newRBACFixture(t)
	member := f.h.SeedAdmin(f.tenant.ID, "agent@test", models.PlatformAgent)

	if code := f.assign(t, member.ID, map[string]any{"role": models.PlatformAgent}); code != http.StatusUnprocessableEntity {
		t.Fatalf("adding a role twice = %d, want 422", code)
	}
	// Rescoping the one they hold is how it widens.
	if w := f.h.Request("PUT", "/v1/team/"+member.ID+"/roles/agent").Cookie("sild_admin", f.owner).
		JSON(map[string]any{"scope": map[string]any{"peer": true}}).Do(); w.Code != http.StatusNoContent {
		t.Fatalf("rescoping = %d %s", w.Code, w.Body)
	}
	if !f.rolesOf(t, member.ID)[models.PlatformAgent].Peer {
		t.Fatal("the rescope did not take")
	}
}

func TestRescopingARoleNobodyHoldsIsNotFound(t *testing.T) {
	f := newRBACFixture(t)
	member := f.h.SeedAdmin(f.tenant.ID, "agent@test", models.PlatformAgent)
	w := f.h.Request("PUT", "/v1/team/"+member.ID+"/roles/translator").Cookie("sild_admin", f.owner).
		JSON(map[string]any{"scope": map[string]any{"locales": []string{"lv"}}}).Do()
	if w.Code != http.StatusNotFound {
		t.Fatalf("rescoping an unheld role = %d %s, want 404", w.Code, w.Body)
	}
}

// A scope the role does not define would be stored and never read — a limit the
// screen shows and the policy ignores.
func TestAScopeADimensionlessRoleCannotCarryIsRefused(t *testing.T) {
	f := newRBACFixture(t)
	member := f.h.SeedAdmin(f.tenant.ID, "agent@test", models.PlatformAgent)
	code := f.assign(t, member.ID, map[string]any{
		"role": models.PlatformAdmin, "scope": map[string]any{"locales": []string{"lv"}},
	})
	if code != http.StatusUnprocessableEntity {
		t.Fatalf("admin role with a language scope = %d, want 422", code)
	}
	if _, ok := f.rolesOf(t, member.ID)[models.PlatformAdmin]; ok {
		t.Fatal("the refused assignment was stored anyway")
	}
}

// A scope naming a project the tenant does not have is a limit no decision can
// ever match — it would read as a grant on screen and grant nothing.
func TestATranslatorScopeIsHeldToRealProjectsAndLanguages(t *testing.T) {
	f := newRBACFixture(t)
	member := f.h.SeedAdmin(f.tenant.ID, "agent@test", models.PlatformAgent)

	code := f.assign(t, member.ID, map[string]any{
		"role": models.PlatformTranslator, "scope": map[string]any{"projects": []string{"no-such-project"}},
	})
	if code != http.StatusUnprocessableEntity {
		t.Fatalf("scope naming an unknown project = %d, want 422", code)
	}
	code = f.assign(t, member.ID, map[string]any{
		"role": models.PlatformTranslator, "scope": map[string]any{"locales": []string{"not a tag"}},
	})
	if code != http.StatusUnprocessableEntity {
		t.Fatalf("scope naming a non-language = %d, want 422", code)
	}
	// The platform project is one every tenant has, so it is accepted by name.
	code = f.assign(t, member.ID, map[string]any{
		"role": models.PlatformTranslator, "scope": map[string]any{"projects": []string{"sild"}, "locales": []string{"lv"}},
	})
	if code != http.StatusNoContent {
		t.Fatalf("scope naming the platform project = %d, want 204", code)
	}
}

func TestTheTeamListCarriesEveryAssignment(t *testing.T) {
	f := newRBACFixture(t)
	member := f.h.SeedAdmin(f.tenant.ID, "both@test", models.PlatformAgent)
	f.assign(t, member.ID, map[string]any{
		"role":  models.PlatformTranslator,
		"scope": map[string]any{"locales": []string{"lv"}, "publish": true},
	})

	var page struct {
		Items []map[string]any `json:"items"`
	}
	testutil.DecodeJSON(t, f.h.Request("GET", "/v1/team").Cookie("sild_admin", f.owner).Do(), &page)
	for _, m := range page.Items {
		if m["id"] != member.ID {
			continue
		}
		held, _ := m["assignments"].([]any)
		if len(held) != 2 {
			t.Fatalf("assignments = %v, want the agent and translator rows", m["assignments"])
		}
		return
	}
	t.Fatal("the member is missing from the team list")
}

// The catalogue is what the Team screen renders; a role missing from it is a
// role nobody can grant.
func TestTheRolesEndpointDescribesEveryRole(t *testing.T) {
	f := newRBACFixture(t)
	var res struct {
		Roles []struct {
			Role        string `json:"role"`
			Label       string `json:"label"`
			Description string `json:"description"`
			Dimensions  []struct {
				Key    string `json:"key"`
				Kind   string `json:"kind"`
				Source string `json:"source"`
			} `json:"dimensions"`
		} `json:"roles"`
	}
	testutil.DecodeJSON(t, f.h.Request("GET", "/v1/roles").Cookie("sild_admin", f.owner).Do(), &res)

	seen := map[string]bool{}
	for _, r := range res.Roles {
		if r.Label == "" || r.Description == "" {
			t.Errorf("%s is undescribed", r.Role)
		}
		seen[r.Role] = true
	}
	for _, want := range []models.PlatformRole{
		models.PlatformOwner, models.PlatformAdmin, models.PlatformAgent, models.PlatformTranslator,
	} {
		if !seen[string(want)] {
			t.Errorf("%s is missing from the catalogue", want)
		}
	}
	for _, r := range res.Roles {
		if r.Role != string(models.PlatformTranslator) {
			continue
		}
		if len(r.Dimensions) != 3 {
			t.Fatalf("translator dimensions = %v, want projects, languages and publish", r.Dimensions)
		}
	}
}

func TestATranslatorCannotReadTheRoleCatalogue(t *testing.T) {
	f := newRBACFixture(t)
	f.h.SeedAdmin(f.tenant.ID, "translator@test", models.PlatformTranslator)
	cookie := loginAs(t, f.h, "translator@test")
	if w := f.h.Request("GET", "/v1/roles").Cookie("sild_admin", cookie).Do(); w.Code != http.StatusForbidden {
		t.Fatalf("translator reading the catalogue = %d, want 403", w.Code)
	}
}

func TestAssignmentsAreTenantScoped(t *testing.T) {
	f := newRBACFixture(t)
	other := f.h.SeedTenant()
	stranger := f.h.SeedAdmin(other.ID, "stranger@test", models.PlatformAgent)

	if code := f.assign(t, stranger.ID, map[string]any{"role": models.PlatformAdmin}); code != http.StatusNotFound {
		t.Fatalf("granting a role in another tenant = %d, want 404", code)
	}
	if len(f.rolesOf(t, stranger.ID)) != 0 {
		t.Fatal("the other tenant's member gained a role in this one")
	}
	held, err := f.h.Svc.RoleAssignments(context.Background(), other.ID, stranger.ID)
	if err != nil || len(held) != 1 || held[0].Role != models.PlatformAgent {
		t.Fatalf("the stranger's own assignments were disturbed: %v (err=%v)", held, err)
	}
}

// Two owners stepping down at once must not both see the other and leave.
func TestConcurrentOwnerRemovalsLeaveOneStanding(t *testing.T) {
	h := testutil.New(t)
	tenant := h.SeedTenant()
	first := h.SeedAdmin(tenant.ID, "one@test", models.PlatformOwner)
	second := h.SeedAdmin(tenant.ID, "two@test", models.PlatformOwner)

	var wg sync.WaitGroup
	for _, id := range []string{first.ID, second.ID} {
		wg.Add(1)
		go func(adminID string) {
			defer wg.Done()
			_ = h.Svc.RemoveRole(context.Background(), tenant.ID, adminID, models.PlatformOwner)
		}(id)
	}
	wg.Wait()

	owners := 0
	all, err := h.Svc.TenantRoleAssignments(context.Background(), tenant.ID)
	if err != nil {
		t.Fatalf("list assignments: %v", err)
	}
	for _, a := range all {
		if a.Role == models.PlatformOwner {
			owners++
		}
	}
	if owners == 0 {
		t.Fatal("both removals succeeded and the tenant has no owner")
	}
}
