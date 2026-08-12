package i18n_test

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
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

// The generated files that carry a key per line rather than a catalog: the typed
// constants each runtime looks strings up through.
var generatedKeyLists = []string{
	"../../../web/src/i18n/catalog.generated.ts",
	"../../../sdks/kotlin/core/src/commonMain/kotlin/io/sild/core/I18nCatalog.generated.kt",
	"../../../sdks/swift/Sources/Sild/SildKeys.generated.swift",
}

// The shared plural table, generated into each client's test fixture from the same
// repo file the Go tests read.
var generatedPluralCases = []string{
	"../../../web/src/i18n/plural-cases.generated.ts",
	"../../../sdks/kotlin/core/src/commonTest/kotlin/io/sild/core/PluralCases.generated.kt",
	"../../../sdks/swift/Tests/SildTests/PluralCases.generated.swift",
}

// Every locale carries exactly the keys it ought to: the same ordinary keys as
// every other, and one plural sibling per category its own language has — no
// more, so a form Latvian cannot select never ships, and no fewer, so a count
// never renders in the wrong language.
func TestEveryLocaleCoversItsOwnKeySet(t *testing.T) {
	cat := i18n.Platform()
	for _, locale := range cat.Locales() {
		want := cat.LocaleKeys(locale)
		if got := cat.KeysIn(locale); !slices.Equal(got, want) {
			for _, k := range want {
				if !slices.Contains(got, k) {
					t.Errorf("%s is missing %s", locale, k)
				}
			}
			for _, k := range got {
				if !slices.Contains(want, k) {
					t.Errorf("%s declares %s, which that language has no category for", locale, k)
				}
			}
		}
		for _, key := range want {
			if v, _ := cat.Default(locale, key); strings.TrimSpace(v) == "" {
				t.Errorf("%s has %s but it is blank", locale, key)
			}
		}
	}
}

// A key the repo no longer declares must be gone from every generated artifact,
// not merely outnumbered by the ones that are current: a stale accessor keeps
// compiling and renders nothing.
func TestNoGeneratedArtifactCarriesAKeyTheRepoDropped(t *testing.T) {
	cat := i18n.Platform()
	declared := map[string]bool{}
	for _, locale := range cat.Locales() {
		for _, k := range cat.LocaleKeys(locale) {
			declared[k] = true
			// A plural's base is what an accessor names, so it counts as declared.
			if base, _, ok := i18n.SplitPlural(k); ok && cat.IsPluralBase(base) {
				declared[base] = true
			}
		}
	}
	// Dotted, quoted strings are how every one of these files spells a key.
	quotedKey := regexp.MustCompile(`"([a-zA-Z][a-zA-Z0-9]*(?:\.[a-zA-Z0-9]+)+)"`)
	for _, path := range generatedKeyLists {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Skipf("generated file not present: %v", err)
		}
		for _, m := range quotedKey.FindAllStringSubmatch(string(raw), -1) {
			if !declared[m[1]] {
				t.Errorf("%s still carries %s, which the repo no longer declares — run `make i18n`",
					filepath.Base(path), m[1])
			}
		}
	}
}

// A typed accessor per key, on every runtime that ships one: story 43's build
// error rather than a blank label.
func TestEveryKeyHasATypedAccessor(t *testing.T) {
	cat := i18n.Platform()
	for _, path := range generatedKeyLists {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Skipf("generated file not present: %v", err)
		}
		src := string(raw)
		for _, key := range cat.Keys() {
			// A plural is addressed by its base and a count, so its siblings get no
			// accessor of their own.
			if base, _, ok := i18n.SplitPlural(key); ok && cat.IsPluralBase(base) {
				key = base
			}
			if !strings.Contains(src, strconv.Quote(key)) {
				t.Errorf("%s has no accessor for %s — run `make i18n`", filepath.Base(path), key)
			}
		}
	}
}

// The clients' plural fixtures come from the same table the Go test reads, so a
// case added to the repo has to reach all four suites or none.
func TestThePluralFixturesMatchTheSharedTable(t *testing.T) {
	want := cases(t)
	for _, path := range generatedPluralCases {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Skipf("generated fixture not present: %v", err)
		}
		src := string(raw)
		// Every runtime spells a row as (locale, count, category) in that order.
		row := regexp.MustCompile(`"([a-z]{2})",\s*(\d+),\s*"([a-z]+)"`)
		got := map[string]bool{}
		for _, m := range row.FindAllStringSubmatch(src, -1) {
			got[m[1]+"/"+m[2]+"/"+m[3]] = true
		}
		if len(got) != len(want) {
			t.Errorf("%s carries %d cases, the table has %d — run `make i18n`",
				filepath.Base(path), len(got), len(want))
		}
		for _, c := range want {
			key := fmt.Sprintf("%s/%d/%s", c.Locale, c.Count, c.Category)
			if !got[key] {
				t.Errorf("%s is missing %s — run `make i18n`", filepath.Base(path), key)
			}
		}
	}
}

// The plural tables the clients pick categories from are generated from
// plurals.json, so a language added there has to reach them — otherwise a locale
// the editor offers zero for renders one/other on the device.
func TestTheGeneratedPluralTablesMatchTheRepo(t *testing.T) {
	for _, path := range generatedCatalogs {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Skipf("generated catalog not present: %v", err)
		}
		src := string(raw)
		pair := regexp.MustCompile(`"([a-z]{2})"\s*(?:to|:)\s*"([a-z_]+)"`)
		got := map[string]string{}
		for _, m := range pair.FindAllStringSubmatch(src, -1) {
			got[m[1]] = m[2]
		}
		for _, locale := range i18n.TableLocales() {
			want := i18n.Family(locale)
			if got[locale] != want {
				t.Errorf("%s maps %s to %q, the repo says %q — run `make i18n`",
					filepath.Base(path), locale, got[locale], want)
			}
			// And the family it names has to carry the categories the repo gives it.
			for _, cat := range i18n.Categories(locale) {
				if !strings.Contains(src, strconv.Quote(cat)) {
					t.Errorf("%s is missing the %s category — run `make i18n`",
						filepath.Base(path), cat)
				}
			}
		}
	}
}

// The manifest tells a tenant developer which SDK line a bundle targets, so the
// number it reports has to be the SDK's own.
func TestTheCatalogVersionIsTheShippedSDKVersion(t *testing.T) {
	raw, err := os.ReadFile("../../../sdks/kotlin/core/src/commonMain/kotlin/io/sild/core/Version.kt")
	if err != nil {
		t.Skipf("native SDK not present: %v", err)
	}
	want := `SDK_VERSION = ` + strconv.Quote(i18n.CatalogVersion)
	if !strings.Contains(string(raw), want) {
		t.Errorf("i18n.CatalogVersion is %s; the Kotlin SDK declares something else — move both together",
			i18n.CatalogVersion)
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
// Compared per locale block rather than by searching the whole file, so two
// locales' values swapped between them cannot pass either.
func TestTheGeneratedClientCatalogsAreCurrent(t *testing.T) {
	cat := i18n.Platform()
	for _, path := range generatedCatalogs {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Skipf("generated catalog not present: %v", err)
		}
		blocks := generatedBlocks(path, string(raw))
		for _, locale := range cat.Locales() {
			got, ok := blocks[locale]
			if !ok {
				t.Errorf("%s is missing locale %s — run `make i18n`", filepath.Base(path), locale)
				continue
			}
			want := map[string]string{}
			for _, key := range cat.LocaleKeys(locale) {
				if v, has := cat.Default(locale, key); has {
					want[key] = v
				}
			}
			for key, value := range want {
				if got[key] != value {
					t.Errorf("%s has %s/%s as %q, the repo says %q — run `make i18n`",
						filepath.Base(path), locale, key, got[key], value)
				}
			}
			for key := range got {
				if _, declared := want[key]; !declared {
					t.Errorf("%s carries %s/%s, which the repo does not declare — run `make i18n`",
						filepath.Base(path), locale, key)
				}
			}
		}
	}
}

// defaultsMap is the region of a generated catalog holding the per-locale strings,
// which both outputs open with DEFAULTS and close at column zero (TS) or with a
// bare `)` (Kotlin).
func defaultsMap(src string) string {
	open := strings.Index(src, "DEFAULTS")
	if open < 0 {
		return src
	}
	src = src[open:]
	for _, close := range []string{"\n};", "\n)"} {
		if i := strings.Index(src, close); i > 0 {
			src = src[:i]
		}
	}
	return src
}

// generatedBlocks reads a generated catalog back into locale → key → text. Both
// outputs nest one block per locale, so the parse is the same shape either way;
// Kotlin escapes `$`, which starts a template otherwise.
func generatedBlocks(path, src string) map[string]map[string]string {
	entry := regexp.MustCompile(`"((?:[^"\\]|\\.)*)"\s*(?:to|:)\s*"((?:[^"\\]|\\.)*)"`)
	unquote := func(s string) string {
		if strings.HasSuffix(path, ".kt") {
			s = strings.ReplaceAll(s, "\\$", "$")
		}
		if out, err := strconv.Unquote(`"` + s + `"`); err == nil {
			return out
		}
		return s
	}
	out := map[string]map[string]string{}
	// Only the defaults map: the plural tables further down pair a locale with a
	// family name and would otherwise read as text for the last locale's block.
	src = defaultsMap(src)
	// A block opens with `"lv": {` (TS) or `"lv" to mapOf(` (Kotlin) and closes at
	// the next such opening or the end of the map.
	head := regexp.MustCompile(`"([a-z]{2})"\s*(?::\s*\{|to mapOf\()`)
	heads := head.FindAllStringSubmatchIndex(src, -1)
	for i, h := range heads {
		end := len(src)
		if i+1 < len(heads) {
			end = heads[i+1][0]
		}
		locale := src[h[2]:h[3]]
		body := src[h[1]:end]
		strings := map[string]string{}
		for _, m := range entry.FindAllStringSubmatch(body, -1) {
			strings[unquote(m[1])] = unquote(m[2])
		}
		out[locale] = strings
	}
	return out
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
