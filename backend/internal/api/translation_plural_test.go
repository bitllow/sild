package api_test

import (
	"net/http"
	"testing"
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

	w := f.h.Request("POST", "/v1/translations/projects/shop/declarations").
		Cookie("sild_admin", f.owner).
		JSON(map[string]any{
			"key":     "cart.items",
			"plurals": map[string]string{"one": "{count} item", "other": "{count} items"},
		}).Do()
	if w.Code != http.StatusNoContent {
		t.Fatalf("declare a plural: %d %s", w.Code, w.Body)
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
	w := f.h.Request("POST", "/v1/translations/projects/shop/declarations").
		Cookie("sild_admin", f.owner).
		JSON(map[string]any{
			"key":     "cart.items",
			"plurals": map[string]string{"one": "{count} item", "other": "{count} items"},
		}).Do()
	if w.Code != http.StatusNoContent {
		t.Fatalf("declare: %d %s", w.Code, w.Body)
	}
	w = f.h.Request("DELETE", "/v1/translations/projects/shop/declarations/cart.items").
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
