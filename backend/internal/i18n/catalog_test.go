package i18n_test

import (
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/bitllow/sild/backend/internal/i18n"
)

// generatedCatalog is what codegen fans the repo files out to for the web client.
const generatedCatalog = "../../../web/src/i18n/catalog.generated.ts"

func TestEveryLocaleCoversTheSameKeys(t *testing.T) {
	cat := i18n.Platform()
	for _, locale := range cat.Locales() {
		for _, key := range cat.Keys() {
			v, ok := cat.Default(locale, key)
			if !ok {
				t.Errorf("%s is missing %s", locale, key)
				continue
			}
			if strings.TrimSpace(v) == "" {
				t.Errorf("%s has %s but it is blank", locale, key)
			}
		}
	}
}

func TestEstonianIsEtNotEe(t *testing.T) {
	if _, ok := i18n.Platform().Default("et", "widget.home.title"); !ok {
		t.Fatal("et is missing")
	}
	if _, ok := i18n.Platform().Default("ee", "widget.home.title"); ok {
		t.Fatal("ee is the Ewe language; Estonian is et")
	}
}

// A stale generated catalog would ship a client that renders a key Sild no
// longer has, or misses one it just added.
func TestTheGeneratedClientCatalogIsCurrent(t *testing.T) {
	raw, err := os.ReadFile(generatedCatalog)
	if err != nil {
		t.Skipf("generated catalog not present: %v", err)
	}
	src := string(raw)
	cat := i18n.Platform()
	for _, locale := range cat.Locales() {
		if !strings.Contains(src, strconv.Quote(locale)) {
			t.Errorf("generated catalog is missing locale %s — run `make i18n`", locale)
		}
	}
	for _, key := range cat.Keys() {
		if !strings.Contains(src, strconv.Quote(key)) {
			t.Errorf("generated catalog is missing key %s — run `make i18n`", key)
		}
	}
}

func TestResolveFallsThroughToTheSourceLanguage(t *testing.T) {
	cat := i18n.Platform()
	// A language Sild does not ship: nothing resolves until the fallback does.
	if got := cat.Resolve(nil, "fi", "lv", "widget.home.title"); got != "Sazinieties ar mums" {
		t.Fatalf("fallback = %q, want the fallback language's text", got)
	}
	if got := cat.Resolve(nil, "fi", "", "widget.home.title"); got != "Chat with us" {
		t.Fatalf("no fallback = %q, want the source language", got)
	}
	if got := cat.Resolve(nil, "fi", "", "no.such.key"); got != "no.such.key" {
		t.Fatalf("unknown key = %q, want the key itself", got)
	}
}

func TestNormalizeDropsTheRegion(t *testing.T) {
	if got := i18n.Normalize("en-GB"); got != "en" {
		t.Fatalf("normalize = %q, want en", got)
	}
}

// A published bundle is complete, so a client never runs the fallback chain
// itself — the whole reason the widget can hold one map per locale.
func TestABundleCarriesEveryKey(t *testing.T) {
	cat := i18n.Platform()
	for _, locale := range append(cat.Locales(), "fi") {
		b := cat.Bundle(nil, locale, "lv")
		for _, k := range cat.Keys() {
			if b[k] == "" || b[k] == k {
				t.Fatalf("%s bundle does not resolve %s", locale, k)
			}
		}
	}
}

func TestAnOverrideBeatsTheShippedTextForItsOwnLocaleOnly(t *testing.T) {
	cat := i18n.Platform()
	ov := i18n.Overrides{"lv": {"widget.home.title": "Runā ar mums"}}
	if got := cat.Resolve(ov, "lv", "en", "widget.home.title"); got != "Runā ar mums" {
		t.Fatalf("lv = %q, want the override", got)
	}
	// A fully translated fallback must not outrank the language actually asked for.
	if got := cat.Resolve(ov, "es", "lv", "widget.home.title"); got != "Chatea con nosotros" {
		t.Fatalf("es = %q, want Spanish rather than the Latvian override", got)
	}
}
