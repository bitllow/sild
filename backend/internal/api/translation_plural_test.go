package api_test

import (
	"net/http"
	"testing"

	"github.com/bitllow/sild/backend/internal/store/models"
)

const agentsOnline = "widget.home.agentsOnline"

// The editor offers exactly the categories the target language has, so a Latvian
// translator is asked for zero and never for few (docs/adr/0004).
func TestTheEditorOffersTheCategoriesTheLanguageHas(t *testing.T) {
	f := newI18nFixture(t)
	lv := f.keys(t, "lv", "")
	if _, ok := lv[agentsOnline+".zero"]; !ok {
		t.Error("lv is not offered a zero form")
	}
	if _, ok := lv[agentsOnline+".few"]; ok {
		t.Error("lv is offered few, which Latvian does not have")
	}
	if got := lv[agentsOnline+".zero"]["plural_category"]; got != "zero" {
		t.Errorf("plural_category = %v", got)
	}
	if got := lv[agentsOnline+".zero"]["plural_base"]; got != agentsOnline {
		t.Errorf("plural_base = %v", got)
	}

	ru := f.keys(t, "ru", "")
	for _, cat := range []string{"one", "few", "many", "other"} {
		if _, ok := ru[agentsOnline+"."+cat]; !ok {
			t.Errorf("ru is not offered %s", cat)
		}
	}
	if _, ok := ru[agentsOnline+".zero"]; ok {
		t.Error("ru is offered zero, which Russian does not have")
	}

	en := f.keys(t, "en", "")
	if _, ok := en[agentsOnline+".zero"]; ok {
		t.Error("en is offered zero, which English does not have")
	}
}

// A count's placeholder has to be documented, or a translator cannot place it
// where their own language's word order wants it.
func TestAKeysPlaceholdersTravelWithIt(t *testing.T) {
	f := newI18nFixture(t)
	rows := f.keys(t, "lv", "")
	got, _ := rows[agentsOnline+".one"]["placeholders"].([]any)
	if len(got) != 1 || got[0] != "count" {
		t.Errorf("placeholders = %v, want [count]", rows[agentsOnline+".one"]["placeholders"])
	}
	if plain, _ := rows[titleKey]["placeholders"].([]any); len(plain) != 0 {
		t.Errorf("a key with no placeholders reported %v", plain)
	}
}

// Latvian's zero form is writable even though English declares no such sibling,
// and it publishes into the bundle a Latvian device reads.
func TestALanguagesOwnPluralFormIsWritableAndPublishes(t *testing.T) {
	f := newI18nFixture(t)
	zero := agentsOnline + ".zero"
	if res := f.put(t, "lv", zero, "nav neviena aģenta"); res.StatusCode != http.StatusNoContent {
		t.Fatalf("write the lv zero form: %d", res.StatusCode)
	}
	if got := f.keys(t, "lv", "")[zero]["value"]; got != "nav neviena aģenta" {
		t.Fatalf("value = %v", got)
	}
	v := f.publish(t)
	bundle := f.bundle(t, "lv", v)
	if bundle[zero] != "nav neviena aģenta" {
		t.Errorf("the bundle carries %q for zero", bundle[zero])
	}
	if _, ok := bundle[agentsOnline+".few"]; ok {
		t.Error("the lv bundle carries a few form Latvian cannot select")
	}
}

// A form English cannot select has no English source of its own, so it reads from
// the other form — what a translator is actually working from.
func TestAZeroFormSourcesFromTheOtherForm(t *testing.T) {
	f := newI18nFixture(t)
	rows := f.keys(t, "lv", "")
	other := rows[agentsOnline+".other"]["source"]
	if got := rows[agentsOnline+".zero"]["source"]; got != other {
		t.Errorf("zero source = %v, want the other form's %v", got, other)
	}
}

// A tenant declares a plural key by giving the source language's forms; it lands
// as siblings, and every other language is then offered its own categories.
func TestATenantDeclaresAPluralKeyByItsSourceForms(t *testing.T) {
	f := newI18nFixture(t)
	f.createProject(t, "shop", "Shop")
	f.enable(t, "shop", "en", "lv")

	if res := f.declare(t, "shop", "cart.items", "", map[string]string{
		"one": "{count} item", "other": "{count} items",
	}); res.StatusCode != http.StatusNoContent {
		t.Fatalf("declare a plural: %d", res.StatusCode)
	}

	lv := f.projectKeys(t, "shop", "lv", "")
	for _, cat := range []string{"zero", "one", "other"} {
		if _, ok := lv["cart.items."+cat]; !ok {
			t.Errorf("lv is not offered cart.items.%s", cat)
		}
	}
	if _, ok := lv["cart.items.few"]; ok {
		t.Error("lv is offered few")
	}
	if got := lv["cart.items.one"]["source"]; got != "{count} item" {
		t.Errorf("source = %v", got)
	}
}

// A plural is one key to a tenant, so undeclaring it takes every sibling with it.
func TestUndeclaringAPluralRemovesEverySibling(t *testing.T) {
	f := newI18nFixture(t)
	f.createProject(t, "shop", "Shop")
	if res := f.declare(t, "shop", "cart.items", "", map[string]string{
		"one": "{count} item", "other": "{count} items",
	}); res.StatusCode != http.StatusNoContent {
		t.Fatalf("declare a plural: %d", res.StatusCode)
	}
	w := f.h.Request("DELETE", "/v1/translations/projects/shop/declarations/cart.items").
		Cookie("sild_admin", f.owner).Do()
	if w.Code != http.StatusNoContent {
		t.Fatalf("undeclare: %d %s", w.Code, w.Body)
	}
	if rows := f.projectKeys(t, "shop", "en", ""); len(rows) != 0 {
		t.Errorf("%d rows survived: %v", len(rows), rows)
	}
}

func TestAPluralDeclarationNeedsEverySourceForm(t *testing.T) {
	f := newI18nFixture(t)
	f.createProject(t, "shop", "Shop")
	w := f.h.Request("POST", "/v1/translations/projects/shop/declarations").
		Cookie("sild_admin", f.owner).
		JSON(map[string]any{"key": "cart.items", "plurals": map[string]string{"one": "{count} item"}}).Do()
	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("a half-declared plural: %d %s", w.Code, w.Body)
	}
	// English has no few form to declare, so asking for one is a mistake worth saying.
	w = f.h.Request("POST", "/v1/translations/projects/shop/declarations").
		Cookie("sild_admin", f.owner).
		JSON(map[string]any{"key": "cart.items", "plurals": map[string]string{
			"one": "a", "other": "b", "few": "c",
		}}).Do()
	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("a form the source language does not have: %d %s", w.Code, w.Body)
	}
}

// The editor labels a plural row by its base, so removing one is removing the key
// — whichever sibling the row happened to name.
func TestRemovingOneSiblingRemovesThePlural(t *testing.T) {
	f := newI18nFixture(t)
	f.createProject(t, "shop", "Shop")
	if res := f.declare(t, "shop", "cart.items", "", map[string]string{
		"one": "{count} item", "other": "{count} items",
	}); res.StatusCode != http.StatusNoContent {
		t.Fatalf("declare: %d", res.StatusCode)
	}
	w := f.h.Request("DELETE", "/v1/translations/projects/shop/declarations/cart.items.one").
		Cookie("sild_admin", f.owner).Do()
	if w.Code != http.StatusNoContent {
		t.Fatalf("undeclare a sibling: %d %s", w.Code, w.Body)
	}
	if rows := f.projectKeys(t, "shop", "en", ""); len(rows) != 0 {
		t.Errorf("%d rows survived: %v", len(rows), rows)
	}
}

// A build token holds the client fetch, so the fetch is held to its project too.
func TestAScopedKeyCannotFetchAnotherProjectsBundle(t *testing.T) {
	f := newI18nFixture(t)
	f.createProject(t, "shop", "Shop")
	f.publish(t)
	key := f.h.SeedScopedAPIKey(f.tenant.ID, models.RoleScope{Projects: []string{"shop"}})

	w := f.h.Request("GET", "/v1/translations/manifest?project=shop").Bearer(key).Do()
	if w.Code != http.StatusOK {
		t.Fatalf("its own manifest: %d %s", w.Code, w.Body)
	}
	if w = f.h.Request("GET", "/v1/translations/manifest?project=sild").Bearer(key).Do(); w.Code != http.StatusForbidden {
		t.Errorf("another project's manifest: %d %s", w.Code, w.Body)
	}
	w = f.h.Request("GET", "/v1/translations/bundle?project=sild&locale=lv&version=1").Bearer(key).Do()
	if w.Code != http.StatusForbidden {
		t.Errorf("another project's bundle: %d %s", w.Code, w.Body)
	}
	// A user JWT carries no translation scope, so the client read is untouched.
	token := f.h.MintToken(f.tenant.ID, "u_alice")
	if w := f.h.Request("GET", "/v1/translations/manifest?project=sild").Bearer(token).Do(); w.Code != http.StatusOK {
		t.Errorf("an ordinary client lost its manifest: %d %s", w.Code, w.Body)
	}
}

// A key that becomes a plural — or stops being one — leaves no half of the old
// shape behind, or the editor would list both and a bundle would carry both.
func TestRedeclaringAKeyReplacesItsShape(t *testing.T) {
	f := newI18nFixture(t)
	f.createProject(t, "shop", "Shop")

	if res := f.declare(t, "shop", "cart.items", "Items"); res.StatusCode != http.StatusNoContent {
		t.Fatalf("declare ordinary: %d", res.StatusCode)
	}
	if res := f.declare(t, "shop", "cart.items", "", map[string]string{
		"one": "{count} item", "other": "{count} items",
	}); res.StatusCode != http.StatusNoContent {
		t.Fatalf("declare plural: %d", res.StatusCode)
	}
	rows := f.projectKeys(t, "shop", "en", "")
	if _, ok := rows["cart.items"]; ok {
		t.Errorf("the ordinary key survived becoming a plural: %v", rows)
	}
	if _, ok := rows["cart.items.one"]; !ok {
		t.Errorf("the plural did not land: %v", rows)
	}

	// And back again.
	if res := f.declare(t, "shop", "cart.items", "Items"); res.StatusCode != http.StatusNoContent {
		t.Fatalf("declare ordinary again: %d", res.StatusCode)
	}
	rows = f.projectKeys(t, "shop", "en", "")
	if _, ok := rows["cart.items.one"]; ok {
		t.Errorf("a sibling survived the key becoming ordinary: %v", rows)
	}
	if len(rows) != 1 {
		t.Errorf("%d rows for one key: %v", len(rows), rows)
	}
}
