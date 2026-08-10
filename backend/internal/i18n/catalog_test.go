package i18n_test

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/bitllow/sild/backend/internal/i18n"
)

// What codegen fans the repo files out to. Every client surface is generated, so a
// key added here reaches all of them or none.
var generatedCatalogs = []string{
	"../../../web/src/i18n/catalog.generated.ts",
	"../../../sdks/kotlin/core/src/commonMain/kotlin/io/sild/core/I18nCatalog.generated.kt",
}

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

// A stale generated catalog would ship a client that renders a key Sild no longer
// has, misses one it just added, or renders last week's wording of one it kept.
func TestTheGeneratedClientCatalogsAreCurrent(t *testing.T) {
	cat := i18n.Platform()
	for _, path := range generatedCatalogs {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Skipf("generated catalog not present: %v", err)
		}
		src := string(raw)
		for _, locale := range cat.Locales() {
			if !strings.Contains(src, strconv.Quote(locale)) {
				t.Errorf("%s is missing locale %s — run `make i18n`", path, locale)
			}
			for _, key := range cat.Keys() {
				value, ok := cat.Default(locale, key)
				if !ok {
					continue
				}
				if !strings.Contains(src, generatedEntry(path, key, value)) {
					t.Errorf("%s has stale or missing text for %s/%s — run `make i18n`", path, locale, key)
				}
			}
		}
	}
}

// How codegen writes one entry, so a value edited without regenerating fails here
// rather than shipping. Kotlin escapes `$`, which starts a template otherwise.
func generatedEntry(path, key, value string) string {
	if strings.HasSuffix(path, ".kt") {
		esc := func(s string) string { return strings.ReplaceAll(strconv.Quote(s), "$", "\\$") }
		return esc(key) + " to " + esc(value)
	}
	return strconv.Quote(key) + ": " + strconv.Quote(value)
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

// Client surfaces that look strings up by key. A typo here renders the key itself
// to a customer, and no compiler catches it — so the repo does.
var keyCallSites = []string{
	"../../../sdks/kotlin/ui/src/main/kotlin/io/sild/ui",
	"../../../sdks/swift/Sources/Sild",
	"../../../web/src/widget",
}

// tCall finds t("some.key") — the one lookup shape every surface uses.
var tCall = regexp.MustCompile(`\bt\(\s*"([a-z][\w.]*\.[\w.]+)"`)

func TestNoSurfaceRendersAKeyTheCatalogDoesNotDeclare(t *testing.T) {
	cat := i18n.Platform()
	for _, dir := range keyCallSites {
		err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return err
			}
			switch filepath.Ext(path) {
			case ".kt", ".swift", ".ts", ".tsx":
			default:
				return nil
			}
			raw, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			for _, m := range tCall.FindAllStringSubmatch(string(raw), -1) {
				if !cat.Declared(m[1]) {
					t.Errorf("%s renders %q, which no locale file declares", path, m[1])
				}
			}
			return nil
		})
		if err != nil {
			t.Skipf("surface not present: %v", err)
		}
	}
}
