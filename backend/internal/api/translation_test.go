package api_test

import (
	"context"
	"net/http"
	"strconv"
	"testing"

	"github.com/bitllow/sild/backend/internal/store/models"
	"github.com/bitllow/sild/backend/internal/testutil"
)

const (
	titleKey  = "widget.home.title"
	titleEN   = "Chat with us"
	titleLV   = "Sazinieties ar mums"
	genericLV = "Jauna ziņa"
)

type i18nFixture struct {
	h      *testutil.Harness
	tenant *models.Tenant
	owner  string
}

func newI18nFixture(t *testing.T) *i18nFixture {
	t.Helper()
	h := testutil.New(t)
	tenant := h.SeedTenant()
	h.SeedAdmin(tenant.ID, "owner@test", models.PlatformOwner)
	return &i18nFixture{h: h, tenant: tenant, owner: loginAs(t, h, "owner@test")}
}

func (f *i18nFixture) keys(t *testing.T, locale, query string) map[string]map[string]any {
	t.Helper()
	w := f.h.Request("GET", "/v1/translations/projects/sild/keys?locale="+locale+query).
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

func (f *i18nFixture) put(t *testing.T, locale, key, value string) *http.Response {
	t.Helper()
	return f.h.Request("PUT", "/v1/translations/projects/sild/keys/"+key).
		Cookie("sild_admin", f.owner).
		JSON(map[string]any{"locale": locale, "value": value}).Do().Result()
}

// manifestVersion is what a device would see: the published version of lv, or 0
// before anything is published.
func (f *i18nFixture) manifestVersion(t *testing.T) int {
	t.Helper()
	w := f.h.Request("GET", "/v1/translations/manifest?project=sild").Cookie("sild_admin", f.owner).Do()
	if w.Code != http.StatusOK {
		t.Fatalf("manifest: %d %s", w.Code, w.Body)
	}
	var m struct {
		Locales map[string]int `json:"locales"`
	}
	testutil.DecodeJSON(t, w, &m)
	return m.Locales["lv"]
}

func (f *i18nFixture) publish(t *testing.T) int { return f.publishIn(t, "sild") }

func (f *i18nFixture) publishIn(t *testing.T, project string) int {
	t.Helper()
	w := f.h.Request("POST", "/v1/translations/projects/"+project+"/releases").
		Cookie("sild_admin", f.owner).Do()
	if w.Code != http.StatusCreated {
		t.Fatalf("publish: %d %s", w.Code, w.Body)
	}
	var rel struct {
		Version int `json:"version"`
	}
	testutil.DecodeJSON(t, w, &rel)
	return rel.Version
}

func (f *i18nFixture) bundle(t *testing.T, locale string, version int) map[string]string {
	t.Helper()
	w := f.h.Request("GET",
		"/v1/translations/bundle?project=sild&locale="+locale+"&version="+strconv.Itoa(version)).
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

func TestOverrideShadowsTheShippedDefault(t *testing.T) {
	f := newI18nFixture(t)
	if res := f.put(t, "lv", titleKey, "Runā ar mums"); res.StatusCode != http.StatusNoContent {
		t.Fatalf("put override: %d", res.StatusCode)
	}
	rows := f.keys(t, "lv", "")
	if got := rows[titleKey]["value"]; got != "Runā ar mums" {
		t.Fatalf("value = %v, want the override", got)
	}
	if got := rows[titleKey]["state"]; got != "custom" {
		t.Fatalf("state = %v, want custom", got)
	}
	if got := rows["widget.composer.send"]["value"]; got != "Sūtīt" {
		t.Fatalf("an untouched key changed: %v", got)
	}
}

func TestResetRestoresTheShippedDefault(t *testing.T) {
	f := newI18nFixture(t)
	f.put(t, "lv", titleKey, "Runā ar mums")

	w := f.h.Request("DELETE", "/v1/translations/projects/sild/keys/"+titleKey+"?locale=lv").
		Cookie("sild_admin", f.owner).Do()
	if w.Code != http.StatusNoContent {
		t.Fatalf("reset: %d %s", w.Code, w.Body)
	}
	rows := f.keys(t, "lv", "")
	if got := rows[titleKey]["value"]; got != titleLV {
		t.Fatalf("value after reset = %v, want the shipped default", got)
	}
	if got := rows[titleKey]["state"]; got != "default" {
		t.Fatalf("state after reset = %v, want default", got)
	}
}

// A Sild release that rewords the English leaves the translation written against
// the old wording flagged — and still rendering.
func TestASourceChangeFlagsTheOverrideForReview(t *testing.T) {
	f := newI18nFixture(t)
	err := f.h.Store.Translations().PutOverride(context.Background(), &models.TranslationOverride{
		TenantID: f.tenant.ID, Project: "sild", Locale: "lv", Key: titleKey,
		Value: "Runā ar mums", SourceHash: "written-against-something-else",
	})
	if err != nil {
		t.Fatalf("seed override: %v", err)
	}
	rows := f.keys(t, "lv", "")
	if got := rows[titleKey]["state"]; got != "needs_review" {
		t.Fatalf("state = %v, want needs_review", got)
	}
	if got := rows[titleKey]["value"]; got != "Runā ar mums" {
		t.Fatalf("a flagged translation stopped rendering: %v", got)
	}
}

func TestAnUnpublishedEditIsInvisibleToABundleRead(t *testing.T) {
	f := newI18nFixture(t)
	v1 := f.publish(t)
	f.put(t, "lv", titleKey, "Runā ar mums")

	if got := f.bundle(t, "lv", v1)[titleKey]; got != titleLV {
		t.Fatalf("bundle %d = %q, want the text published at that version", v1, got)
	}
	v2 := f.publish(t)
	if got := f.bundle(t, "lv", v2)[titleKey]; got != "Runā ar mums" {
		t.Fatalf("bundle %d = %q, want the published override", v2, got)
	}
}

func TestAPublishedVersionNeverChanges(t *testing.T) {
	f := newI18nFixture(t)
	f.put(t, "lv", titleKey, "First")
	v1 := f.publish(t)
	f.put(t, "lv", titleKey, "Second")
	f.publish(t)

	if got := f.bundle(t, "lv", v1)[titleKey]; got != "First" {
		t.Fatalf("version %d changed under a later publish: %q", v1, got)
	}
}

func TestRollbackRestoresEarlierContent(t *testing.T) {
	f := newI18nFixture(t)
	f.put(t, "lv", titleKey, "First")
	v1 := f.publish(t)
	f.put(t, "lv", titleKey, "Second")
	f.publish(t)

	w := f.h.Request("POST", "/v1/translations/projects/sild/releases/"+strconv.Itoa(v1)+"/rollback").
		Cookie("sild_admin", f.owner).Do()
	if w.Code != http.StatusCreated {
		t.Fatalf("rollback: %d %s", w.Code, w.Body)
	}
	var rel struct {
		Version int `json:"version"`
	}
	testutil.DecodeJSON(t, w, &rel)
	if rel.Version <= v1 {
		t.Fatalf("rollback version = %d, want a new one past %d", rel.Version, v1)
	}
	if got := f.bundle(t, "lv", rel.Version)[titleKey]; got != "First" {
		t.Fatalf("rolled-back bundle = %q, want the earlier text", got)
	}
}

func TestManifestReportsTheCurrentVersionAndAnswers304(t *testing.T) {
	f := newI18nFixture(t)
	version := f.publish(t)

	w := f.h.Request("GET", "/v1/translations/manifest?project=sild").
		Cookie("sild_admin", f.owner).Do()
	if w.Code != http.StatusOK {
		t.Fatalf("manifest: %d %s", w.Code, w.Body)
	}
	var m struct {
		Locales map[string]int `json:"locales"`
	}
	testutil.DecodeJSON(t, w, &m)
	if m.Locales["lv"] != version {
		t.Fatalf("manifest lv = %d, want %d", m.Locales["lv"], version)
	}
	etag := w.Header().Get("ETag")
	if etag == "" {
		t.Fatal("manifest served no ETag, so every launch would re-download")
	}
	again := f.h.Request("GET", "/v1/translations/manifest?project=sild").
		Cookie("sild_admin", f.owner).Header("If-None-Match", etag).Do()
	if again.Code != http.StatusNotModified {
		t.Fatalf("unchanged manifest = %d, want 304", again.Code)
	}
}

// A tenant may add a language Sild does not ship; every key then falls through
// to the fallback rather than rendering a key.
func TestATenantAddedLanguageFallsBackRatherThanShowingKeys(t *testing.T) {
	f := newI18nFixture(t)
	w := f.h.Request("PUT", "/v1/translations/projects/sild").
		Cookie("sild_admin", f.owner).
		JSON(map[string]any{
			"fallback_locale": "lv",
			"locales":         []string{"en", "lv", "fi"},
		}).Do()
	if w.Code != http.StatusNoContent {
		t.Fatalf("save project: %d %s", w.Code, w.Body)
	}
	version := f.publish(t)
	if got := f.bundle(t, "fi", version)[titleKey]; got != titleLV {
		t.Fatalf("untranslated language = %q, want the fallback language's text", got)
	}
}

func TestTranslatorCannotReachAnythingButTranslations(t *testing.T) {
	f := newI18nFixture(t)
	f.h.SeedAdminScoped(f.tenant.ID, "translator@test", models.PlatformTranslator, everyProjectAndLanguage())
	cookie := loginAs(t, f.h, "translator@test")

	for _, path := range []string{"/v1/team", "/v1/api-keys", "/v1/brands", "/v1/realtime/token"} {
		w := f.h.Request("GET", path).Cookie("sild_admin", cookie).Do()
		if w.Code != http.StatusForbidden {
			t.Errorf("GET %s as translator = %d, want 403", path, w.Code)
		}
	}
	// Collections answer an out-of-scope caller with an empty page rather than a
	// refusal (ARCHITECTURE §5), so what matters is that nothing comes back.
	for _, path := range []string{"/v1/conversations", "/v1/contacts"} {
		w := f.h.Request("GET", path).Cookie("sild_admin", cookie).Do()
		var res struct {
			Items []map[string]any `json:"items"`
		}
		testutil.DecodeJSON(t, w, &res)
		if len(res.Items) != 0 {
			t.Errorf("GET %s as translator returned %d rows", path, len(res.Items))
		}
	}
	w := f.h.Request("GET", "/v1/translations/projects/sild/keys?locale=lv").
		Cookie("sild_admin", cookie).Do()
	if w.Code != http.StatusOK {
		t.Fatalf("translator reading translations = %d, want 200", w.Code)
	}
}

func TestATranslatorsScopeNarrowsWhichLanguagesTheyMayWrite(t *testing.T) {
	f := newI18nFixture(t)
	f.h.SeedAdminScoped(f.tenant.ID, "translator@test", models.PlatformTranslator, models.RoleScope{
		Projects: []string{models.ScopeAll}, Locales: []string{"lv"},
	})
	cookie := loginAs(t, f.h, "translator@test")

	granted := f.h.Request("PUT", "/v1/translations/projects/sild/keys/"+titleKey).
		Cookie("sild_admin", cookie).
		JSON(map[string]any{"locale": "lv", "value": "Runā ar mums"}).Do()
	if granted.Code != http.StatusNoContent {
		t.Fatalf("granted language = %d, want 204: %s", granted.Code, granted.Body)
	}
	refused := f.h.Request("PUT", "/v1/translations/projects/sild/keys/"+titleKey).
		Cookie("sild_admin", cookie).
		JSON(map[string]any{"locale": "es", "value": "Habla con nosotros"}).Do()
	if refused.Code != http.StatusForbidden {
		t.Fatalf("ungranted language = %d, want 403", refused.Code)
	}
}

// Auto-publish is the project's setting, but publishing is the person's grant:
// a draft-only translator's edit must not go live because someone else turned it on.
func TestADraftOnlyTranslatorDoesNotTripAutoPublish(t *testing.T) {
	f := newI18nFixture(t)
	if w := f.h.Request("PUT", "/v1/translations/projects/sild").Cookie("sild_admin", f.owner).
		JSON(map[string]any{"locales": []string{"en", "lv"}, "fallback_locale": "en", "auto_publish": true}).Do(); w.Code != http.StatusNoContent {
		t.Fatalf("enable auto-publish: %d %s", w.Code, w.Body)
	}
	f.h.SeedAdminScoped(f.tenant.ID, "drafter@test", models.PlatformTranslator, everyProjectAndLanguage())
	cookie := loginAs(t, f.h, "drafter@test")

	before := f.manifestVersion(t)
	if w := f.h.Request("PUT", "/v1/translations/projects/sild/keys/"+titleKey).Cookie("sild_admin", cookie).
		JSON(map[string]any{"locale": "lv", "value": "Runā ar mums"}).Do(); w.Code != http.StatusNoContent {
		t.Fatalf("draft write: %d %s", w.Code, w.Body)
	}
	if got := f.manifestVersion(t); got != before {
		t.Fatalf("the draft went live: version moved %d → %d", before, got)
	}

	// The owner's own write still auto-publishes — the setting is not broken.
	if w := f.h.Request("PUT", "/v1/translations/projects/sild/keys/"+titleKey).Cookie("sild_admin", f.owner).
		JSON(map[string]any{"locale": "lv", "value": "Sazinieties"}).Do(); w.Code != http.StatusNoContent {
		t.Fatalf("owner write: %d %s", w.Code, w.Body)
	}
	if got := f.manifestVersion(t); got == before {
		t.Fatal("auto-publish did not fire for a caller who may publish")
	}
}

// everyProjectAndLanguage is the widest translator scope: every project, every
// language, drafts only.
func everyProjectAndLanguage() models.RoleScope {
	return models.RoleScope{Projects: []string{models.ScopeAll}, Locales: []string{models.ScopeAll}}
}

func TestATranslatorCannotPublishUntilItIsGranted(t *testing.T) {
	f := newI18nFixture(t)
	f.h.SeedAdminScoped(f.tenant.ID, "translator@test", models.PlatformTranslator, everyProjectAndLanguage())
	cookie := loginAs(t, f.h, "translator@test")

	w := f.h.Request("POST", "/v1/translations/projects/sild/releases").
		Cookie("sild_admin", cookie).Do()
	if w.Code != http.StatusForbidden {
		t.Fatalf("translator publishing = %d, want 403", w.Code)
	}

	scope := everyProjectAndLanguage()
	scope.Publish = true
	f.h.SeedAdminScoped(f.tenant.ID, "publisher@test", models.PlatformTranslator, scope)
	publisher := loginAs(t, f.h, "publisher@test")
	if w := f.h.Request("POST", "/v1/translations/projects/sild/releases").
		Cookie("sild_admin", publisher).Do(); w.Code != http.StatusCreated {
		t.Fatalf("translator granted publishing = %d %s, want 201", w.Code, w.Body)
	}
}

// The whole point of the recipient locale: a nudge is composed server-side, so
// without it a Latvian user's app is Latvian and their notification is English.
func TestNudgeTextFollowsTheRecipientsLanguage(t *testing.T) {
	f := newPushFixture(t)
	ctx := context.Background()
	if err := f.h.Store.Contacts().SetLocale(ctx, f.tenant.ID, "u_bob", "lv"); err != nil {
		t.Fatalf("set locale: %v", err)
	}
	f.send(t, "u_alice", "running late")

	sent := f.h.Notifier.Nudges()
	if len(sent) != 1 {
		t.Fatalf("expected 1 nudge, got %d", len(sent))
	}
	if sent[0].Nudge.Title != genericLV {
		t.Fatalf("nudge title = %q, want the recipient's language (%q)", sent[0].Nudge.Title, genericLV)
	}
}

func TestNudgeTextFollowsAPublishedOverride(t *testing.T) {
	f := newPushFixture(t)
	ctx := context.Background()
	if err := f.h.Store.Contacts().SetLocale(ctx, f.tenant.ID, "u_bob", "lv"); err != nil {
		t.Fatalf("set locale: %v", err)
	}
	seedOverride(t, f.h, f.tenant.ID, "lv", "push.newMessage", "Ziņa no Acme")
	if _, err := f.h.Svc.PublishTranslations(ctx, f.tenant.ID, "sild", ""); err != nil {
		t.Fatalf("publish: %v", err)
	}
	f.send(t, "u_alice", "running late")

	sent := f.h.Notifier.Nudges()
	if len(sent) != 1 {
		t.Fatalf("expected 1 nudge, got %d", len(sent))
	}
	if sent[0].Nudge.Title != "Ziņa no Acme" {
		t.Fatalf("nudge title = %q, want the tenant's published override", sent[0].Nudge.Title)
	}
}

// A draft is not live text. A translator's unpublished edit reaching a lock
// screen would make the publish step a lie.
func TestAnUnpublishedOverrideDoesNotReachANudge(t *testing.T) {
	f := newPushFixture(t)
	ctx := context.Background()
	if err := f.h.Store.Contacts().SetLocale(ctx, f.tenant.ID, "u_bob", "lv"); err != nil {
		t.Fatalf("set locale: %v", err)
	}
	seedOverride(t, f.h, f.tenant.ID, "lv", "push.newMessage", "Ziņa no Acme")
	f.send(t, "u_alice", "running late")

	sent := f.h.Notifier.Nudges()
	if len(sent) != 1 {
		t.Fatalf("expected 1 nudge, got %d", len(sent))
	}
	if sent[0].Nudge.Title != genericLV {
		t.Fatalf("nudge title = %q, want the shipped text until someone publishes", sent[0].Nudge.Title)
	}
}

func seedOverride(t *testing.T, h *testutil.Harness, tenantID, locale, key, value string) {
	t.Helper()
	err := h.Store.Translations().PutOverride(context.Background(), &models.TranslationOverride{
		TenantID: tenantID, Project: "sild", Locale: locale, Key: key, Value: value,
	})
	if err != nil {
		t.Fatalf("seed override: %v", err)
	}
}

func TestALocaleScopedTranslatorCannotReadAnotherLanguage(t *testing.T) {
	f := newI18nFixture(t)
	f.h.SeedAdminScoped(f.tenant.ID, "translator@test", models.PlatformTranslator, models.RoleScope{
		Projects: []string{models.ScopeAll}, Locales: []string{"es"},
	})
	cookie := loginAs(t, f.h, "translator@test")

	if w := f.h.Request("GET", "/v1/translations/projects/sild/keys?locale=lv").
		Cookie("sild_admin", cookie).Do(); w.Code != http.StatusForbidden {
		t.Fatalf("ungranted language = %d, want 403", w.Code)
	}
	// Omitting it must not fall through to the tenant's fallback language.
	if w := f.h.Request("GET", "/v1/translations/projects/sild/keys").
		Cookie("sild_admin", cookie).Do(); w.Code != http.StatusBadRequest {
		t.Fatalf("omitted language = %d, want 400", w.Code)
	}
}

func TestTranslationsAreScopedToTheirTenant(t *testing.T) {
	f := newI18nFixture(t)
	f.put(t, "lv", titleKey, "Runā ar mums")

	other := f.h.SeedTenant()
	f.h.SeedAdmin(other.ID, "other@test", models.PlatformOwner)
	cookie := loginAs(t, f.h, "other@test")

	w := f.h.Request("GET", "/v1/translations/projects/sild/keys?locale=lv").
		Cookie("sild_admin", cookie).Do()
	if w.Code != http.StatusOK {
		t.Fatalf("other tenant list: %d %s", w.Code, w.Body)
	}
	var res struct {
		Items []map[string]any `json:"items"`
	}
	testutil.DecodeJSON(t, w, &res)
	for _, it := range res.Items {
		if it["key"] == titleKey && it["value"] != titleLV {
			t.Fatalf("another tenant sees this tenant's override: %v", it["value"])
		}
	}
}
