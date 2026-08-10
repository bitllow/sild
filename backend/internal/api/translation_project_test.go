package api_test

import (
	"net/http"
	"strconv"
	"testing"

	"github.com/bitllow/sild/backend/internal/store/models"
	"github.com/bitllow/sild/backend/internal/testutil"
)

const (
	ownKey    = "checkout.pay"
	ownSource = "Pay now"
)

// createProject declares a project for the tenant's own strings and returns its slug.
func (f *i18nFixture) createProject(t *testing.T, slug, name string) {
	t.Helper()
	w := f.h.Request("POST", "/v1/translations/projects").
		Cookie("sild_admin", f.owner).
		JSON(map[string]any{"id": slug, "name": name}).Do()
	if w.Code != http.StatusCreated {
		t.Fatalf("create project: %d %s", w.Code, w.Body)
	}
}

func (f *i18nFixture) declare(t *testing.T, project, key, source string) *http.Response {
	t.Helper()
	return f.h.Request("POST", "/v1/translations/projects/"+project+"/declarations").
		Cookie("sild_admin", f.owner).
		JSON(map[string]any{"key": key, "source": source}).Do().Result()
}

// enable turns on the languages a project offers; the fallback must be one of them.
func (f *i18nFixture) enable(t *testing.T, project string, locales ...string) {
	t.Helper()
	w := f.h.Request("PUT", "/v1/translations/projects/"+project).
		Cookie("sild_admin", f.owner).
		JSON(map[string]any{"fallback_locale": "en", "locales": locales}).Do()
	if w.Code != http.StatusNoContent {
		t.Fatalf("enable locales: %d %s", w.Code, w.Body)
	}
}

func (f *i18nFixture) projects(t *testing.T) map[string]map[string]any {
	t.Helper()
	w := f.h.Request("GET", "/v1/translations/projects").Cookie("sild_admin", f.owner).Do()
	if w.Code != http.StatusOK {
		t.Fatalf("list projects: %d %s", w.Code, w.Body)
	}
	var res struct {
		Items []map[string]any `json:"items"`
	}
	testutil.DecodeJSON(t, w, &res)
	out := map[string]map[string]any{}
	for _, it := range res.Items {
		out[it["slug"].(string)] = it
	}
	return out
}

// projectKeys lists one project's keys, which the platform fixture hardcodes to sild.
func (f *i18nFixture) projectKeys(t *testing.T, project, locale, query string) map[string]map[string]any {
	t.Helper()
	w := f.h.Request("GET", "/v1/translations/projects/"+project+"/keys?locale="+locale+query).
		Cookie("sild_admin", f.owner).Do()
	if w.Code != http.StatusOK {
		t.Fatalf("list keys: %d %s", w.Code, w.Body)
	}
	var res struct {
		Items []map[string]any `json:"items"`
	}
	testutil.DecodeJSON(t, w, &res)
	out := map[string]map[string]any{}
	for _, it := range res.Items {
		out[it["key"].(string)] = it
	}
	return out
}

func (f *i18nFixture) putIn(t *testing.T, project, locale, key, value string) *http.Response {
	t.Helper()
	return f.h.Request("PUT", "/v1/translations/projects/"+project+"/keys/"+key).
		Cookie("sild_admin", f.owner).
		JSON(map[string]any{"locale": locale, "value": value}).Do().Result()
}

func TestATenantProjectRendersItsDeclaredSource(t *testing.T) {
	f := newI18nFixture(t)
	f.createProject(t, "shop", "Shop")
	if res := f.declare(t, "shop", ownKey, ownSource); res.StatusCode != http.StatusNoContent {
		t.Fatalf("declare: %d", res.StatusCode)
	}
	rows := f.projectKeys(t, "shop", "en", "")
	if got := rows[ownKey]["source"]; got != ownSource {
		t.Fatalf("source = %v, want %q", got, ownSource)
	}
	if got := rows[ownKey]["value"]; got != ownSource {
		t.Fatalf("value = %v, want the source until it is translated", got)
	}
	if got := rows[ownKey]["namespace"]; got != "checkout" {
		t.Fatalf("namespace = %v, want checkout", got)
	}
}

func TestATenantProjectPublishesTranslationsAndFallsBackToItsSource(t *testing.T) {
	f := newI18nFixture(t)
	f.createProject(t, "shop", "Shop")
	f.enable(t, "shop", "en", "lv")
	f.declare(t, "shop", ownKey, ownSource)
	f.declare(t, "shop", "checkout.cancel", "Cancel")
	if res := f.putIn(t, "shop", "lv", ownKey, "Maksāt tagad"); res.StatusCode != http.StatusNoContent {
		t.Fatalf("translate: %d", res.StatusCode)
	}

	w := f.h.Request("POST", "/v1/translations/projects/shop/releases").
		Cookie("sild_admin", f.owner).Do()
	if w.Code != http.StatusCreated {
		t.Fatalf("publish: %d %s", w.Code, w.Body)
	}
	var rel struct {
		Version int `json:"version"`
	}
	testutil.DecodeJSON(t, w, &rel)

	strs := f.bundleIn(t, "shop", "lv", rel.Version)
	if strs[ownKey] != "Maksāt tagad" {
		t.Fatalf("translated key = %q", strs[ownKey])
	}
	if strs["checkout.cancel"] != "Cancel" {
		t.Fatalf("untranslated key = %q, want the source", strs["checkout.cancel"])
	}
}

func TestRewordingASourceFlagsItsTranslationForReview(t *testing.T) {
	f := newI18nFixture(t)
	f.createProject(t, "shop", "Shop")
	f.enable(t, "shop", "en", "lv")
	f.declare(t, "shop", ownKey, ownSource)
	f.putIn(t, "shop", "lv", ownKey, "Maksāt tagad")

	if res := f.declare(t, "shop", ownKey, "Pay securely"); res.StatusCode != http.StatusNoContent {
		t.Fatalf("reword: %d", res.StatusCode)
	}
	rows := f.projectKeys(t, "shop", "lv", "")
	if got := rows[ownKey]["state"]; got != "needs_review" {
		t.Fatalf("state = %v, want needs_review", got)
	}
	if got := rows[ownKey]["value"]; got != "Maksāt tagad" {
		t.Fatalf("a flagged translation stopped rendering: %v", got)
	}
}

func TestUndeclaringAKeyTakesItsTranslationsWithIt(t *testing.T) {
	f := newI18nFixture(t)
	f.createProject(t, "shop", "Shop")
	f.enable(t, "shop", "en", "lv")
	f.declare(t, "shop", ownKey, ownSource)
	f.putIn(t, "shop", "lv", ownKey, "Maksāt tagad")

	w := f.h.Request("DELETE", "/v1/translations/projects/shop/declarations/"+ownKey).
		Cookie("sild_admin", f.owner).Do()
	if w.Code != http.StatusNoContent {
		t.Fatalf("undeclare: %d %s", w.Code, w.Body)
	}
	if rows := f.projectKeys(t, "shop", "lv", ""); len(rows) != 0 {
		t.Fatalf("keys after undeclare = %v, want none", rows)
	}
	// Re-declaring must not resurrect the translation the key used to carry.
	f.declare(t, "shop", ownKey, ownSource)
	rows := f.projectKeys(t, "shop", "lv", "")
	if got := rows[ownKey]["value"]; got != ownSource {
		t.Fatalf("value = %v, want the source", got)
	}
}

func TestCompletionReportsHowMuchOfALanguageIsTranslated(t *testing.T) {
	f := newI18nFixture(t)
	f.createProject(t, "shop", "Shop")
	f.enable(t, "shop", "en", "lv")
	f.declare(t, "shop", ownKey, ownSource)
	f.declare(t, "shop", "checkout.cancel", "Cancel")

	if got := f.projects(t)["shop"]["completion"].(map[string]any)["lv"]; got != float64(0) {
		t.Fatalf("completion before translating = %v, want 0", got)
	}
	f.putIn(t, "shop", "lv", ownKey, "Maksāt tagad")
	p := f.projects(t)["shop"]
	if got := p["completion"].(map[string]any)["lv"]; got != float64(50) {
		t.Fatalf("completion with one of two keys = %v, want 50", got)
	}
	if got := p["keys"]; got != float64(2) {
		t.Fatalf("keys = %v, want 2", got)
	}
}

func TestKeysCanBeFilteredToOneNamespace(t *testing.T) {
	f := newI18nFixture(t)
	f.createProject(t, "shop", "Shop")
	f.declare(t, "shop", ownKey, ownSource)
	f.declare(t, "shop", "account.name", "Your name")

	rows := f.projectKeys(t, "shop", "en", "&namespace=checkout")
	if len(rows) != 1 || rows[ownKey] == nil {
		t.Fatalf("namespace filter returned %v", rows)
	}
	if got := f.projects(t)["shop"]["namespaces"]; len(got.([]any)) != 2 {
		t.Fatalf("namespaces = %v, want account and checkout", got)
	}
}

func TestTheSildProjectDeclaresNoKeysAndCannotBeRemoved(t *testing.T) {
	f := newI18nFixture(t)
	if res := f.declare(t, "sild", "widget.home.title", "Hi"); res.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("declaring into the platform project: %d, want 422", res.StatusCode)
	}
	w := f.h.Request("DELETE", "/v1/translations/projects/sild").Cookie("sild_admin", f.owner).Do()
	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("deleting the platform project: %d, want 422", w.Code)
	}
}

// Undeclaring addresses the key as a path segment, so one holding a slash could
// be declared and never removed again.
func TestAKeyThatWouldNotSurviveAURLPathIsRefused(t *testing.T) {
	f := newI18nFixture(t)
	f.createProject(t, "shop", "Shop")
	for _, key := range []string{"checkout/pay", "checkout pay", "checkout?pay", ""} {
		if res := f.declare(t, "shop", key, "Pay now"); res.StatusCode != http.StatusUnprocessableEntity {
			t.Fatalf("declaring %q: %d, want 422", key, res.StatusCode)
		}
	}
}

func TestDeletingAProjectTakesItsStringsWithIt(t *testing.T) {
	f := newI18nFixture(t)
	f.createProject(t, "shop", "Shop")
	f.declare(t, "shop", ownKey, ownSource)

	w := f.h.Request("DELETE", "/v1/translations/projects/shop").Cookie("sild_admin", f.owner).Do()
	if w.Code != http.StatusNoContent {
		t.Fatalf("delete project: %d %s", w.Code, w.Body)
	}
	if _, ok := f.projects(t)["shop"]; ok {
		t.Fatal("the project is still listed")
	}
	// Reusing the slug must not bring the previous project's keys back.
	f.createProject(t, "shop", "Shop again")
	if rows := f.projectKeys(t, "shop", "en", ""); len(rows) != 0 {
		t.Fatalf("keys after recreating = %v, want none", rows)
	}
}

func TestATenantsProjectsAreInvisibleToAnotherTenant(t *testing.T) {
	f := newI18nFixture(t)
	f.createProject(t, "shop", "Shop")
	f.declare(t, "shop", ownKey, ownSource)

	other := f.h.SeedTenant()
	f.h.SeedAdmin(other.ID, "other@test", models.PlatformOwner)
	token := loginAs(t, f.h, "other@test")

	w := f.h.Request("GET", "/v1/translations/projects/shop/keys?locale=en").
		Cookie("sild_admin", token).Do()
	if w.Code != http.StatusNotFound {
		t.Fatalf("reading another tenant's project: %d, want 404", w.Code)
	}
	w = f.h.Request("GET", "/v1/translations/projects").Cookie("sild_admin", token).Do()
	var res struct {
		Items []map[string]any `json:"items"`
	}
	testutil.DecodeJSON(t, w, &res)
	for _, it := range res.Items {
		if it["slug"] == "shop" {
			t.Fatal("another tenant's project is listed")
		}
	}
}

func TestAProjectScopedTranslatorCannotReachASecondProject(t *testing.T) {
	f := newI18nFixture(t)
	f.createProject(t, "shop", "Shop")
	f.createProject(t, "marketing", "Marketing")
	f.declare(t, "shop", ownKey, ownSource)
	f.declare(t, "marketing", "ad.headline", "Buy now")

	admin := f.h.SeedAdmin(f.tenant.ID, "translator@test", models.PlatformTranslator)
	w := f.h.Request("PUT", "/v1/translations/grants/"+admin.ID).
		Cookie("sild_admin", f.owner).
		JSON(map[string]any{"projects": []string{"shop"}, "locales": []string{}}).Do()
	if w.Code != http.StatusNoContent {
		t.Fatalf("grant: %d %s", w.Code, w.Body)
	}
	token := loginAs(t, f.h, "translator@test")

	if w := f.h.Request("GET", "/v1/translations/projects/shop/keys?locale=en").
		Cookie("sild_admin", token).Do(); w.Code != http.StatusOK {
		t.Fatalf("granted project: %d %s", w.Code, w.Body)
	}
	if w := f.h.Request("GET", "/v1/translations/projects/marketing/keys?locale=en").
		Cookie("sild_admin", token).Do(); w.Code != http.StatusForbidden {
		t.Fatalf("ungranted project: %d, want 403", w.Code)
	}

	// The collection narrows rather than refusing: a translator granted one project
	// lands on it, and is not told the others exist.
	w = f.h.Request("GET", "/v1/translations/projects").Cookie("sild_admin", token).Do()
	if w.Code != http.StatusOK {
		t.Fatalf("project list for a scoped translator: %d %s", w.Code, w.Body)
	}
	var res struct {
		Items []map[string]any `json:"items"`
	}
	testutil.DecodeJSON(t, w, &res)
	if len(res.Items) != 1 || res.Items[0]["slug"] != "shop" {
		t.Fatalf("listed projects = %v, want only shop", res.Items)
	}
}

// bundleIn reads one published locale of any project.
func (f *i18nFixture) bundleIn(t *testing.T, project, locale string, version int) map[string]string {
	t.Helper()
	w := f.h.Request("GET",
		"/v1/translations/bundle?project="+project+"&locale="+locale+"&version="+strconv.Itoa(version)).
		Cookie("sild_admin", f.owner).Do()
	if w.Code != http.StatusOK {
		t.Fatalf("bundle: %d %s", w.Code, w.Body)
	}
	var res struct {
		Strings map[string]string `json:"strings"`
	}
	testutil.DecodeJSON(t, w, &res)
	return res.Strings
}

// A tenant that changes only its fallback language changes what a device asking
// for something else reads, so a client holding the old validator must be told.
func TestChangingTheFallbackLanguageInvalidatesAHeldManifest(t *testing.T) {
	f := newI18nFixture(t)
	w := f.h.Request("GET", "/v1/translations/manifest?project=sild").
		Cookie("sild_admin", f.owner).Do()
	if w.Code != http.StatusOK {
		t.Fatalf("manifest: %d %s", w.Code, w.Body)
	}
	etag := w.Header().Get("ETag")
	if etag == "" {
		t.Fatal("no ETag to hold")
	}

	w = f.h.Request("PUT", "/v1/translations/projects/sild").
		Cookie("sild_admin", f.owner).
		JSON(map[string]any{"fallback_locale": "lv", "locales": []string{"en", "lv"}}).Do()
	if w.Code != http.StatusNoContent {
		t.Fatalf("set fallback: %d %s", w.Code, w.Body)
	}

	w = f.h.Request("GET", "/v1/translations/manifest?project=sild").
		Cookie("sild_admin", f.owner).Header("If-None-Match", etag).Do()
	if w.Code != http.StatusOK {
		t.Fatalf("conditional manifest = %d, want 200 with the new fallback", w.Code)
	}
	var m struct {
		FallbackLocale string `json:"fallback_locale"`
	}
	testutil.DecodeJSON(t, w, &m)
	if m.FallbackLocale != "lv" {
		t.Fatalf("fallback_locale = %q, want lv", m.FallbackLocale)
	}
}
