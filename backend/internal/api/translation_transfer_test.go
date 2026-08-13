package api_test

import (
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/bitllow/sild/backend/internal/store/models"
	"github.com/bitllow/sild/backend/internal/testutil"
)

// importReport is what an import (or a dry run of one) says it did.
type importReport struct {
	New     int  `json:"new"`
	Changed int  `json:"changed"`
	Skipped int  `json:"skipped"`
	Created int  `json:"keys_created"`
	DryRun  bool `json:"dry_run"`
	Rows    []struct {
		Key    string `json:"key"`
		Status string `json:"status"`
		Value  string `json:"value"`
		Reason string `json:"reason"`
	} `json:"rows"`
}

func (r importReport) status(key string) string {
	for _, row := range r.Rows {
		if row.Key == key {
			return row.Status
		}
	}
	return ""
}

// importAs posts a file as some caller — an owner session or a build token.
func importAs(t *testing.T, f *i18nFixture, auth func(*testutil.Req) *testutil.Req,
	project, locale, format, body, extra string) *testutil.Req {
	t.Helper()
	path := "/v1/translations/projects/" + project + "/import?locale=" + locale +
		"&format=" + format + extra
	return auth(f.h.Request("POST", path).Raw([]byte(body), contentTypeFor(format)))
}

func contentTypeFor(format string) string {
	if format == "json" {
		return "application/json"
	}
	return "text/plain"
}

func (f *i18nFixture) asOwner(r *testutil.Req) *testutil.Req {
	return r.Cookie("sild_admin", f.owner)
}

func (f *i18nFixture) importFile(t *testing.T, locale, format, body, extra string) importReport {
	return f.importInto(t, "sild", locale, format, body, extra)
}

func (f *i18nFixture) importInto(t *testing.T, project, locale, format, body, extra string) importReport {
	t.Helper()
	w := importAs(t, f, f.asOwner, project, locale, format, body, extra).Do()
	if w.Code != http.StatusOK {
		t.Fatalf("import: %d %s", w.Code, w.Body)
	}
	var rep importReport
	testutil.DecodeJSON(t, w, &rep)
	return rep
}

func (f *i18nFixture) export(t *testing.T, locale, format string) string {
	t.Helper()
	w := f.h.Request("GET", "/v1/translations/projects/sild/export?locale="+locale+"&format="+format).
		Cookie("sild_admin", f.owner).Do()
	if w.Code != http.StatusOK {
		t.Fatalf("export: %d %s", w.Code, w.Body)
	}
	return w.Body.String()
}

// A file goes out and comes back meaning the same thing, in every format a build
// might feed — including the plural siblings a native format reassembles.
func TestEveryFormatRoundTripsThroughTheAPI(t *testing.T) {
	for _, format := range []string{"json", "csv", "android"} {
		t.Run(format, func(t *testing.T) {
			f := newI18nFixture(t)
			if res := f.put(t, "lv", titleKey, "Runā ar mums"); res.StatusCode != http.StatusNoContent {
				t.Fatalf("seed override: %d", res.StatusCode)
			}
			body := f.export(t, "lv", format)
			if !strings.Contains(body, "Runā ar mums") {
				t.Fatalf("the export is missing the tenant's own text:\n%s", body)
			}
			rep := f.importFile(t, "lv", format, body, "")
			if rep.New != 0 || rep.Changed != 0 {
				t.Errorf("re-importing its own export changed something: %+v", rep)
			}
			if rows := f.keys(t, "lv", ""); rows[titleKey]["value"] != "Runā ar mums" {
				t.Errorf("the round-trip lost the override: %v", rows[titleKey]["value"])
			}
		})
	}
}

func TestAnExportCarriesThePluralFormsItsLanguageHas(t *testing.T) {
	f := newI18nFixture(t)
	lv := f.export(t, "lv", "json")
	if !strings.Contains(lv, "widget.home.agentsOnline.zero") {
		t.Errorf("lv export has no zero form:\n%s", lv)
	}
	if strings.Contains(lv, "widget.home.agentsOnline.few") {
		t.Errorf("lv export carries few, which Latvian does not have:\n%s", lv)
	}
	// Android reassembles the siblings under one <plurals>, and drops the dots a
	// resource name cannot hold.
	xml := f.export(t, "lv", "android")
	if !strings.Contains(xml, `<plurals name="widget_home_agentsOnline">`) {
		t.Errorf("no <plurals> element:\n%s", xml)
	}
	if !strings.Contains(xml, `quantity="zero"`) {
		t.Errorf("no zero item:\n%s", xml)
	}
}

// The spreadsheet is for a translator with no account, so it carries the English
// beside the blank they fill in — and comes back as a translation.
func TestTheSpreadsheetGoesOutWithSourcesAndComesBackAsTranslations(t *testing.T) {
	f := newI18nFixture(t)
	sheet := f.export(t, "lv", "sheet")
	if !strings.Contains(sheet, "key,source,translation") {
		t.Fatalf("no spreadsheet header:\n%s", sheet)
	}
	if !strings.Contains(sheet, titleEN) {
		t.Errorf("the English source is missing:\n%s", sheet)
	}
	filled := "key,source,translation\n" + titleKey + "," + titleEN + ",No sheet\n"
	rep := f.importFile(t, "lv", "csv", filled, "")
	if rep.New != 1 || rep.Changed != 0 {
		t.Fatalf("filled sheet reported %+v", rep)
	}
	if rows := f.keys(t, "lv", ""); rows[titleKey]["value"] != "No sheet" {
		t.Errorf("value = %v, want the sheet's text", rows[titleKey]["value"])
	}
}

// A dry run promises exactly what the same call without it does — the preview a
// tenant approves cannot disagree with the write.
func TestADryRunReportsWhatTheWriteThenDoes(t *testing.T) {
	f := newI18nFixture(t)
	body := `{"` + titleKey + `": "Sveiki", "widget.composer.send": "Sūtīt"}`

	preview := f.importFile(t, "lv", "json", body, "&dry_run=1")
	if !preview.DryRun {
		t.Error("the report does not say it was a dry run")
	}
	if rows := f.keys(t, "lv", ""); rows[titleKey]["value"] != titleLV {
		t.Fatalf("a dry run wrote something: %v", rows[titleKey]["value"])
	}

	applied := f.importFile(t, "lv", "json", body, "")
	if preview.New != applied.New || preview.Changed != applied.Changed || preview.Skipped != applied.Skipped {
		t.Fatalf("preview %+v then wrote %+v", preview, applied)
	}
	for _, row := range preview.Rows {
		if got := applied.status(row.Key); got != row.Status {
			t.Errorf("%s: previewed %s, wrote %s", row.Key, row.Status, got)
		}
	}
	// "Sūtīt" is already the shipped Latvian, so only one key actually moved.
	if applied.New != 1 || applied.Skipped != 1 {
		t.Errorf("report = %+v, want one written and one unchanged", applied)
	}
}

func TestImportWithoutTheFlagRefusesAKeyNobodyDeclared(t *testing.T) {
	f := newI18nFixture(t)
	rep := f.importFile(t, "lv", "json", `{"shop.checkout.pay": "Maksāt"}`, "")
	if rep.New != 0 || rep.Skipped != 1 {
		t.Fatalf("report = %+v, want the unknown key skipped", rep)
	}
	if rep.Rows[0].Reason != "unknown key" {
		t.Errorf("reason = %q", rep.Rows[0].Reason)
	}
}

// Sild's own keys are declared in its releases (ADR 0003), so no import may add one.
func TestCreateKeysIsRefusedForSildsOwnProject(t *testing.T) {
	f := newI18nFixture(t)
	w := importAs(t, f, f.asOwner, "sild", "lv", "json",
		`{"shop.checkout.pay": "Maksāt"}`, "&create_keys=1").Do()
	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("create_keys against the platform project: %d %s", w.Code, w.Body)
	}
}

func TestImportDeclaresNewKeysForATenantsOwnProjectWhenAsked(t *testing.T) {
	f := newI18nFixture(t)
	f.createProject(t, "shop", "Shop")

	rep := f.importInto(t, "shop", "en", "json",
		`{"checkout.pay": "Pay now", "checkout.cancel": "Cancel"}`, "&create_keys=1")
	if rep.New != 2 || rep.Created != 2 {
		t.Fatalf("report = %+v, want two keys created", rep)
	}
	w := f.h.Request("GET", "/v1/translations/projects/shop/keys?locale=en").
		Cookie("sild_admin", f.owner).Do()
	if !strings.Contains(w.Body.String(), "Pay now") {
		t.Errorf("the declared key is not in the editor:\n%s", w.Body)
	}
}

// A build token is held to its project the way a translator's grant is, which is
// what makes it safe to put in CI.
func TestAProjectScopedKeyCannotReachAnotherProject(t *testing.T) {
	f := newI18nFixture(t)
	f.createProject(t, "shop", "Shop")
	f.createProject(t, "site", "Site")
	key := f.h.SeedScopedAPIKey(f.tenant.ID, models.RoleScope{Projects: []string{"shop"}})
	bearer := func(r *testutil.Req) *testutil.Req { return r.Bearer(key) }

	w := f.h.Request("GET", "/v1/translations/projects/shop/export?locale=en").Bearer(key).Do()
	if w.Code != http.StatusOK {
		t.Fatalf("export its own project: %d %s", w.Code, w.Body)
	}
	if w = f.h.Request("GET", "/v1/translations/projects/site/export?locale=en").Bearer(key).Do(); w.Code != http.StatusForbidden {
		t.Fatalf("export another project: %d %s", w.Code, w.Body)
	}
	w = importAs(t, f, bearer, "site", "en", "json", `{"a.b": "x"}`, "").Do()
	if w.Code != http.StatusForbidden {
		t.Fatalf("import into another project: %d %s", w.Code, w.Body)
	}
}

func TestALanguageScopedKeyCannotWriteAnotherLanguage(t *testing.T) {
	f := newI18nFixture(t)
	key := f.h.SeedScopedAPIKey(f.tenant.ID, models.RoleScope{Locales: []string{"lv"}})
	bearer := func(r *testutil.Req) *testutil.Req { return r.Bearer(key) }

	if w := importAs(t, f, bearer, "sild", "lv", "json", `{"`+titleKey+`": "Sveiki"}`, "").Do(); w.Code != http.StatusOK {
		t.Fatalf("import its own language: %d %s", w.Code, w.Body)
	}
	if w := importAs(t, f, bearer, "sild", "es", "json", `{"`+titleKey+`": "Hola"}`, "").Do(); w.Code != http.StatusForbidden {
		t.Fatalf("import another language: %d %s", w.Code, w.Body)
	}
}

// A pipeline pushing the build's new source strings is what create_keys is for, so
// a project-scoped token may declare them — in the source language only, since a
// key carries the English it was authored in.
func TestABuildTokenDeclaresKeysOnlyFromASourceLanguageFile(t *testing.T) {
	f := newI18nFixture(t)
	f.createProject(t, "shop", "Shop")
	f.enable(t, "shop", "en", "lv")
	key := f.h.SeedScopedAPIKey(f.tenant.ID, models.RoleScope{Projects: []string{"shop"}})
	bearer := func(r *testutil.Req) *testutil.Req { return r.Bearer(key) }

	w := importAs(t, f, bearer, "shop", "en", "json", `{"checkout.pay": "Pay"}`, "&create_keys=1").Do()
	if w.Code != http.StatusOK {
		t.Fatalf("push a source-language file: %d %s", w.Code, w.Body)
	}
	if rows := f.projectKeys(t, "shop", "en", ""); rows["checkout.pay"]["source"] != "Pay" {
		t.Errorf("source = %v, want the English the build pushed", rows["checkout.pay"]["source"])
	}
	// Latvian text is not an English source, so creating a key from it is refused
	// rather than recorded as one.
	w = importAs(t, f, bearer, "shop", "lv", "json", `{"checkout.cancel": "Atcelt"}`, "&create_keys=1").Do()
	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("create a key from a Latvian file: %d %s", w.Code, w.Body)
	}
}

// An unscoped key is what every key minted before scopes existed is, and it still
// reaches everything.
func TestAnUnscopedKeyStaysTenantWide(t *testing.T) {
	f := newI18nFixture(t)
	f.createProject(t, "shop", "Shop")
	key := f.h.SeedAPIKey(f.tenant.ID)
	for _, project := range []string{"sild", "shop"} {
		w := f.h.Request("GET", "/v1/translations/projects/"+project+"/export?locale=en").Bearer(key).Do()
		if w.Code != http.StatusOK {
			t.Errorf("%s: %d %s", project, w.Code, w.Body)
		}
	}
}

// Publishing from CI is opt-in per key: pushing strings and putting them live are
// different acts, and a build token holds the second only when minted with it.
func TestOnlyAPublishingKeyCanCutARelease(t *testing.T) {
	f := newI18nFixture(t)
	quiet := f.h.SeedScopedAPIKey(f.tenant.ID, models.RoleScope{Projects: []string{"sild"}})
	if w := f.h.Request("POST", "/v1/translations/projects/sild/releases").Bearer(quiet).Do(); w.Code != http.StatusForbidden {
		t.Fatalf("publish without the grant: %d %s", w.Code, w.Body)
	}
	allowed := f.h.SeedScopedAPIKey(f.tenant.ID, models.RoleScope{
		Projects: []string{"sild"}, Locales: []string{models.ScopeAll}, Publish: true,
	})
	if w := f.h.Request("POST", "/v1/translations/projects/sild/releases").Bearer(allowed).Do(); w.Code != http.StatusCreated {
		t.Fatalf("publish with the grant: %d %s", w.Code, w.Body)
	}
}

// The preview an owner approves before publishing, and the only view a draft-only
// translator has of their queue.
func TestTheDraftDiffShowsWhatAPublishWouldChange(t *testing.T) {
	f := newI18nFixture(t)
	f.publish(t)
	f.put(t, "lv", titleKey, "Runā ar mums")

	diff := f.drafts(t, f.owner)
	if diff.NextVersion != 2 {
		t.Errorf("next version = %d, want 2", diff.NextVersion)
	}
	var found bool
	for _, r := range diff.Rows {
		if r.Locale == "lv" && r.Key == titleKey {
			found = true
			if r.Live != titleLV || r.Draft != "Runā ar mums" {
				t.Errorf("row = %+v", r)
			}
		}
	}
	if !found {
		t.Fatalf("the edited key is not in the diff: %+v", diff.Rows)
	}

	f.publish(t)
	if after := f.drafts(t, f.owner); len(after.Rows) != 0 {
		t.Errorf("publishing left %d rows pending: %+v", len(after.Rows), after.Rows)
	}
}

type draftDiff struct {
	NextVersion int `json:"next_version"`
	Rows        []struct {
		Locale string `json:"locale"`
		Key    string `json:"key"`
		Live   string `json:"live"`
		Draft  string `json:"draft"`
	} `json:"rows"`
}

func (f *i18nFixture) drafts(t *testing.T, session string) draftDiff {
	return f.draftsIn(t, "sild", session)
}

func (f *i18nFixture) draftsIn(t *testing.T, project, session string) draftDiff {
	t.Helper()
	w := f.h.Request("GET", "/v1/translations/projects/"+project+"/drafts").
		Cookie("sild_admin", session).Do()
	if w.Code != http.StatusOK {
		t.Fatalf("drafts: %d %s", w.Code, w.Body)
	}
	var diff draftDiff
	testutil.DecodeJSON(t, w, &diff)
	return diff
}

// A translator scoped to one language sees their own drafts waiting and nobody
// else's — the read-only queue the publish grant would otherwise hide.
func TestADraftOnlyTranslatorSeesOnlyTheirLanguagesDrafts(t *testing.T) {
	f := newI18nFixture(t)
	f.h.SeedAdminScoped(f.tenant.ID, "lv@test", models.PlatformTranslator, models.RoleScope{
		Projects: []string{models.ScopeAll}, Locales: []string{"lv"},
	})
	session := loginAs(t, f.h, "lv@test")
	f.put(t, "lv", titleKey, "Runā ar mums")
	f.put(t, "es", titleKey, "Habla con nosotros")

	diff := f.drafts(t, session)
	if len(diff.Rows) == 0 {
		t.Fatal("a translator cannot see that their drafts are waiting")
	}
	for _, r := range diff.Rows {
		if r.Locale != "lv" {
			t.Errorf("a translator scoped to lv saw a %s draft", r.Locale)
		}
	}
}

func TestExportRefusesAFormatItCannotWrite(t *testing.T) {
	f := newI18nFixture(t)
	w := f.h.Request("GET", "/v1/translations/projects/sild/export?locale=lv&format=yaml").
		Cookie("sild_admin", f.owner).Do()
	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("yaml export: %d %s", w.Code, w.Body)
	}
}

func TestImportRefusesAnEmptyBodyRatherThanReportingNothingToDo(t *testing.T) {
	f := newI18nFixture(t)
	w := importAs(t, f, f.asOwner, "sild", "lv", "json", "", "").Do()
	if w.Code != http.StatusBadRequest {
		t.Fatalf("empty import: %d %s", w.Code, w.Body)
	}
}

func TestAnExportDownloadsUnderItsPlatformsFilename(t *testing.T) {
	f := newI18nFixture(t)
	for format, want := range map[string]string{
		"android":     "strings.xml",
		"ios":         "Localizable.strings",
		"ios-plurals": "Localizable.stringsdict",
		"json":        "sild-lv.json",
	} {
		w := f.h.Request("GET", "/v1/translations/projects/sild/export?locale=lv&format="+
			url.QueryEscape(format)).Cookie("sild_admin", f.owner).Do()
		if w.Code != http.StatusOK {
			t.Fatalf("%s: %d %s", format, w.Code, w.Body)
		}
		if got := w.Header().Get("Content-Disposition"); !strings.Contains(got, want) {
			t.Errorf("%s downloads as %q, want %s", format, got, want)
		}
	}
}

// A key created by import is written against its own text, so it lands current
// rather than reading "needs review" the moment it arrives.
func TestAnImportedKeyDoesNotArriveStale(t *testing.T) {
	f := newI18nFixture(t)
	f.createProject(t, "shop", "Shop")

	rep := f.importInto(t, "shop", "en", "json", `{"checkout.pay": "Pay now"}`, "&create_keys=1")
	if rep.Created != 1 {
		t.Fatalf("report = %+v", rep)
	}
	row := f.projectKeys(t, "shop", "en", "")["checkout.pay"]
	if got := row["state"]; got != "custom" {
		t.Errorf("state = %v, want custom — a key born here is not stale", got)
	}
}

// The same rule a declaration goes through: update and delete address a key as one
// path segment, so a key a file could smuggle in has to be refused.
func TestImportRefusesAKeyNoRouteCouldAddress(t *testing.T) {
	f := newI18nFixture(t)
	f.createProject(t, "shop", "Shop")

	w := importAs(t, f, f.asOwner, "shop", "en", "json",
		`{"checkout/pay": "Pay now"}`, "&create_keys=1").Do()
	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("a slash-bearing key: %d %s", w.Code, w.Body)
	}
	if rows := f.projectKeys(t, "shop", "en", ""); len(rows) != 0 {
		t.Errorf("%d keys were created anyway: %v", len(rows), rows)
	}
}

// One sibling is not a plural. A key that merely ends in a category word must not
// turn its base into one, or the editor would ask for forms nobody declared.
func TestOneCategorySiblingDoesNotMakeAPlural(t *testing.T) {
	f := newI18nFixture(t)
	f.createProject(t, "shop", "Shop")
	f.enable(t, "shop", "en", "lv")

	f.importInto(t, "shop", "en", "json", `{"chat.other": "Something else"}`, "&create_keys=1")
	rows := f.projectKeys(t, "shop", "lv", "")
	if _, ok := rows["chat.zero"]; ok {
		t.Errorf("a lone .other key was expanded into categories: %v", rows)
	}
	if got := rows["chat.other"]["plural_category"]; got != "" {
		t.Errorf("plural_category = %v, want none", got)
	}
}

// Completion counts against the language's own key set: Latvian needs a zero form
// English never declares, so counting against English would read 100% with one
// form missing.
func TestCompletionCountsTheLanguagesOwnForms(t *testing.T) {
	f := newI18nFixture(t)
	f.createProject(t, "shop", "Shop")
	f.enable(t, "shop", "en", "lv")
	w := f.h.Request("POST", "/v1/translations/projects/shop/declarations").
		Cookie("sild_admin", f.owner).
		JSON(map[string]any{
			"key":     "cart.items",
			"plurals": map[string]string{"one": "{count} item", "other": "{count} items"},
		}).Do()
	if w.Code != http.StatusNoContent {
		t.Fatalf("declare: %d %s", w.Code, w.Body)
	}

	// Two of Latvian's three forms: not done, however many English has.
	for _, cat := range []string{"one", "other"} {
		if res := f.putIn(t, "shop", "lv", "cart.items."+cat, "kaut kas"); res.StatusCode != http.StatusNoContent {
			t.Fatalf("write lv %s: %d", cat, res.StatusCode)
		}
	}
	if got := f.completion(t, "shop", "lv"); got == 100 {
		t.Errorf("lv reads %d%% with its zero form missing", got)
	}
	if res := f.putIn(t, "shop", "lv", "cart.items.zero", "nav neviena"); res.StatusCode != http.StatusNoContent {
		t.Fatalf("write lv zero: %d", res.StatusCode)
	}
	if got := f.completion(t, "shop", "lv"); got != 100 {
		t.Errorf("lv reads %d%% with every form written", got)
	}
}

func (f *i18nFixture) completion(t *testing.T, project, locale string) int {
	t.Helper()
	figures, ok := f.projects(t)[project]["completion"].(map[string]any)
	if !ok {
		t.Fatalf("%s reports no completion figures", project)
	}
	pct, _ := figures[locale].(float64)
	return int(pct)
}

// `publish` on an unscoped key would decide nothing, so it is refused rather than
// stored and quietly ignored.
func TestPublishOnAnUnscopedKeyIsRefused(t *testing.T) {
	f := newI18nFixture(t)
	w := f.h.Request("POST", "/v1/api-keys").Cookie("sild_admin", f.owner).
		JSON(map[string]any{"label": "ci", "publish": true}).Do()
	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("publish with no scope: %d %s", w.Code, w.Body)
	}
}

// A key's reach has to be readable after the fact, or a build token cannot be
// audited.
func TestAKeysScopeIsListedBackToTheTenant(t *testing.T) {
	f := newI18nFixture(t)
	f.createProject(t, "shop", "Shop")
	w := f.h.Request("POST", "/v1/api-keys").Cookie("sild_admin", f.owner).
		JSON(map[string]any{"label": "ci", "projects": []string{"shop"}, "publish": true}).Do()
	if w.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", w.Code, w.Body)
	}
	var created map[string]any
	testutil.DecodeJSON(t, w, &created)
	// A named project with no languages means every language of it.
	if got, _ := created["locales"].([]any); len(got) != 1 || got[0] != "all" {
		t.Errorf("locales = %v, want [all]", created["locales"])
	}

	w = f.h.Request("GET", "/v1/api-keys").Cookie("sild_admin", f.owner).Do()
	if w.Code != http.StatusOK {
		t.Fatalf("list: %d %s", w.Code, w.Body)
	}
	if !strings.Contains(w.Body.String(), `"projects":["shop"]`) {
		t.Errorf("the list does not say what the key reaches:\n%s", w.Body)
	}
}

// The manifest's validator has to cover everything its body carries, or a client
// holding an older SDK target never learns the catalog moved.
func TestTheManifestValidatorCoversTheSDKVersion(t *testing.T) {
	f := newI18nFixture(t)
	w := f.h.Request("GET", "/v1/translations/manifest?project=sild").Cookie("sild_admin", f.owner).Do()
	if w.Code != http.StatusOK {
		t.Fatalf("manifest: %d %s", w.Code, w.Body)
	}
	var body struct {
		SDKVersion string `json:"sdk_version"`
	}
	testutil.DecodeJSON(t, w, &body)
	if body.SDKVersion == "" {
		t.Fatal("the manifest does not say which SDK line these keys target")
	}
	if etag := w.Header().Get("ETag"); !strings.Contains(etag, body.SDKVersion) {
		t.Errorf("ETag %s does not cover sdk_version %s", etag, body.SDKVersion)
	}
}

// A key minted with a translation scope is a build token and nothing else: the
// thing a tenant is invited to put in CI must not also read their conversations.
func TestAScopedKeyReachesTranslationsAndNothingElse(t *testing.T) {
	f := newI18nFixture(t)
	unscoped := f.h.SeedAPIKey(f.tenant.ID)
	// A conversation to be blind to.
	token := f.h.MintToken(f.tenant.ID, "u_alice")
	w := f.h.Request("POST", "/v1/conversations").Bearer(token).JSON(map[string]any{}).Do()
	if w.Code != http.StatusCreated {
		t.Fatalf("seed a conversation: %d %s", w.Code, w.Body)
	}
	var conv struct {
		ID string `json:"id"`
	}
	testutil.DecodeJSON(t, w, &conv)

	scoped := f.h.SeedScopedAPIKey(f.tenant.ID, models.RoleScope{Projects: []string{"sild"}})
	if w := f.h.Request("GET", "/v1/translations/projects/sild/export?locale=en").Bearer(scoped).Do(); w.Code != http.StatusOK {
		t.Fatalf("its own surface: %d %s", w.Code, w.Body)
	}
	// Everything outside the translation surface is refused, including the routes
	// that take the credential kind as their whole authorization.
	for _, probe := range []struct {
		method, path string
		body         map[string]any
	}{
		{"GET", "/v1/conversations", nil},
		{"GET", "/v1/conversations/" + conv.ID, nil},
		{"POST", "/v1/tokens", map[string]any{"user_id": "u_alice"}},
	} {
		req := f.h.Request(probe.method, probe.path).Bearer(scoped)
		if probe.body != nil {
			req = req.JSON(probe.body)
		}
		if w := req.Do(); w.Code != http.StatusForbidden {
			t.Errorf("%s %s: %d %s — a build token reached beyond translations",
				probe.method, probe.path, w.Code, w.Body)
		}
	}

	// An unscoped key is the tenant's own backend credential and keeps its reach.
	if w := f.h.Request("GET", "/v1/conversations/"+conv.ID).Bearer(unscoped).Do(); w.Code != http.StatusOK {
		t.Errorf("an unscoped key lost its reach: %d %s", w.Code, w.Body)
	}
}

// A file with a bad key late in it must be refused whole: the dry run says so, and
// the write leaves nothing behind.
func TestABadKeyLateInAFileLeavesNothingWritten(t *testing.T) {
	f := newI18nFixture(t)
	f.createProject(t, "shop", "Shop")
	body := `{"checkout.pay": "Pay now", "checkout/cancel": "Cancel"}`

	preview := importAs(t, f, f.asOwner, "shop", "en", "json", body, "&create_keys=1&dry_run=1").Do()
	if preview.Code != http.StatusUnprocessableEntity {
		t.Fatalf("dry run: %d %s", preview.Code, preview.Body)
	}
	applied := importAs(t, f, f.asOwner, "shop", "en", "json", body, "&create_keys=1").Do()
	if applied.Code != http.StatusUnprocessableEntity {
		t.Fatalf("apply: %d %s", applied.Code, applied.Body)
	}
	if rows := f.projectKeys(t, "shop", "en", ""); len(rows) != 0 {
		t.Errorf("the good key was written anyway: %v", rows)
	}
}

// Removing a published key changes the next release, so the preview has to say so.
func TestTheDraftDiffShowsARemovedKey(t *testing.T) {
	f := newI18nFixture(t)
	f.createProject(t, "shop", "Shop")
	if res := f.declare(t, "shop", "checkout.pay", "Pay now"); res.StatusCode != http.StatusNoContent {
		t.Fatalf("declare: %d", res.StatusCode)
	}
	f.publishIn(t, "shop")
	w := f.h.Request("DELETE", "/v1/translations/projects/shop/declarations/checkout.pay").
		Cookie("sild_admin", f.owner).Do()
	if w.Code != http.StatusNoContent {
		t.Fatalf("undeclare: %d %s", w.Code, w.Body)
	}

	diff := f.draftsIn(t, "shop", f.owner)
	var found bool
	for _, r := range diff.Rows {
		if r.Key == "checkout.pay" {
			found = true
			if r.Live != "Pay now" || r.Draft != "" {
				t.Errorf("row = %+v, want the live text going away", r)
			}
		}
	}
	if !found {
		t.Errorf("the removal is not in the preview: %+v", diff.Rows)
	}
}

// A grant names a project by id, so deleting the project has to take the grant with
// it — otherwise creating a project of the same name later hands it to whoever held
// the old one.
func TestDeletingAProjectRevokesItsGrants(t *testing.T) {
	f := newI18nFixture(t)
	f.createProject(t, "shop", "Shop")
	f.h.SeedAdminScoped(f.tenant.ID, "shopkeeper@test", models.PlatformTranslator,
		models.RoleScope{Projects: []string{"shop"}, Locales: []string{models.ScopeAll}})
	key := f.h.SeedScopedAPIKey(f.tenant.ID, models.RoleScope{Projects: []string{"shop"}})
	session := loginAs(t, f.h, "shopkeeper@test")

	// Both reach it while it exists.
	if w := f.h.Request("GET", "/v1/translations/projects/shop/export?locale=en").Bearer(key).Do(); w.Code != http.StatusOK {
		t.Fatalf("the key cannot reach its project: %d %s", w.Code, w.Body)
	}
	w := f.h.Request("DELETE", "/v1/translations/projects/shop").Cookie("sild_admin", f.owner).Do()
	if w.Code != http.StatusNoContent {
		t.Fatalf("delete: %d %s", w.Code, w.Body)
	}

	// A project of the same name, and neither grant follows it.
	f.createProject(t, "shop", "Shop again")
	if w := f.h.Request("GET", "/v1/translations/projects/shop/export?locale=en").Bearer(key).Do(); w.Code != http.StatusForbidden {
		t.Errorf("the old key reached the new project: %d %s", w.Code, w.Body)
	}
	w = f.h.Request("GET", "/v1/translations/projects/shop/keys?locale=en").Cookie("sild_admin", session).Do()
	if w.Code != http.StatusForbidden {
		t.Errorf("the old grant reached the new project: %d %s", w.Code, w.Body)
	}
	// The member's assignment says so, rather than the screen showing a live grant.
	w = f.h.Request("GET", "/v1/team").Cookie("sild_admin", f.owner).Do()
	if strings.Contains(w.Body.String(), `"projects":["shop"]`) {
		t.Errorf("the team screen still shows the deleted project as granted:\n%s", w.Body)
	}
}

// Declaring keys is the tenant's act. A build token does it as the build's source
// of truth; a translator uploading a file does not get there that way.
func TestATranslatorCannotDeclareKeysByImporting(t *testing.T) {
	f := newI18nFixture(t)
	f.createProject(t, "shop", "Shop")
	f.h.SeedAdminScoped(f.tenant.ID, "tr@test", models.PlatformTranslator, models.RoleScope{
		Projects: []string{models.ScopeAll}, Locales: []string{models.ScopeAll},
	})
	session := loginAs(t, f.h, "tr@test")
	as := func(r *testutil.Req) *testutil.Req { return r.Cookie("sild_admin", session) }

	// Their own work — writing text for a key that exists — is untouched.
	if w := importAs(t, f, as, "shop", "en", "json", `{"a.b": "x"}`, "").Do(); w.Code != http.StatusOK {
		t.Fatalf("a translator importing text: %d %s", w.Code, w.Body)
	}
	w := importAs(t, f, as, "shop", "en", "json", `{"checkout.pay": "Pay"}`, "&create_keys=1").Do()
	if w.Code != http.StatusForbidden {
		t.Fatalf("a translator declaring keys: %d %s", w.Code, w.Body)
	}
	if rows := f.projectKeys(t, "shop", "en", ""); len(rows) != 0 {
		t.Errorf("%d keys were declared anyway: %v", len(rows), rows)
	}
}

// A scope is stored the way authorization reads it, or it grants nothing at all.
func TestAKeyScopeIsStoredNormalized(t *testing.T) {
	f := newI18nFixture(t)
	w := f.h.Request("POST", "/v1/api-keys").Cookie("sild_admin", f.owner).
		JSON(map[string]any{"label": "ci", "projects": []string{"sild"}, "locales": []string{"LV", "et-EE"}}).Do()
	if w.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", w.Code, w.Body)
	}
	var created struct {
		Key string `json:"key"`
	}
	testutil.DecodeJSON(t, w, &created)
	// The request carries "lv"; a scope holding "LV" would refuse it.
	if w := importAs(t, f, func(r *testutil.Req) *testutil.Req { return r.Bearer(created.Key) },
		"sild", "lv", "json", `{"`+titleKey+`": "Sveiki"}`, "").Do(); w.Code != http.StatusOK {
		t.Errorf("a key scoped to LV cannot write lv: %d %s", w.Code, w.Body)
	}
}
