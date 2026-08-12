// Package i18n holds Sild's own strings. The canonical files in locales/ are the
// source of truth (docs/adr/0003) — they are embedded here and generated into the
// client bundles, so no surface keeps its own copy.
package i18n

import (
	"embed"
	"encoding/json"
	"fmt"
	"maps"
	"path"
	"slices"
	"strings"
	"sync"
)

// PlatformProject is the reserved project every tenant has without creating one.
// Its keys come from the files below rather than from rows, so a tenant holds
// only the overrides it actually wrote.
const PlatformProject = "sild"

// SourceLocale is the language the keys are authored in and the last readable
// fallback before the key itself.
const SourceLocale = "en"

// CatalogVersion is the SDK line this key set belongs to, reported by the manifest
// so a tenant developer can tell what a bundle targets before upgrading. It moves
// with the native SDK version — a contract test holds the two together.
const CatalogVersion = "0.1.2"

//go:embed locales/*.json
var localeFS embed.FS

// Catalog is the platform project's keys and their text per locale.
type Catalog struct {
	byLocale map[string]map[string]string
	locales  []string
	keys     []string
	// pluralBases are the keys whose text is a set of category siblings. A
	// language's own categories are then filled in per locale, so Latvian is asked
	// for zero even though English never declares one.
	pluralBases map[string]bool
	// localeKeys caches that expansion per language: locale → []string.
	localeKeys sync.Map
}

var platform = sync.OnceValue(func() *Catalog {
	c, err := load()
	if err != nil {
		// The files are embedded at build time; a bad one is a broken binary.
		panic(fmt.Sprintf("i18n: %v", err))
	}
	return c
})

// Platform returns the embedded catalog.
func Platform() *Catalog { return platform() }

func load() (*Catalog, error) {
	entries, err := localeFS.ReadDir("locales")
	if err != nil {
		return nil, err
	}
	c := &Catalog{byLocale: map[string]map[string]string{}}
	for _, e := range entries {
		locale := strings.TrimSuffix(e.Name(), path.Ext(e.Name()))
		raw, err := localeFS.ReadFile("locales/" + e.Name())
		if err != nil {
			return nil, err
		}
		var strs map[string]string
		if err := json.Unmarshal(raw, &strs); err != nil {
			return nil, fmt.Errorf("locales/%s: %w", e.Name(), err)
		}
		c.byLocale[locale] = strs
		c.locales = append(c.locales, locale)
	}
	source, ok := c.byLocale[SourceLocale]
	if !ok {
		return nil, fmt.Errorf("locales/%s.json is missing", SourceLocale)
	}
	for k := range source {
		c.keys = append(c.keys, k)
	}
	slices.Sort(c.locales)
	slices.Sort(c.keys)
	c.pluralBases = PluralBases(c.keys)
	return c, nil
}

// NewCatalog builds a catalog whose only shipped locale is the source one — the
// shape a tenant-owned project has, where the declared keys are the sources and
// every other language arrives as an override. pluralBases are the keys the
// tenant declared plural; with none given they are read off the key names.
func NewCatalog(sources map[string]string, pluralBases ...string) *Catalog {
	c := &Catalog{
		byLocale: map[string]map[string]string{SourceLocale: sources},
		locales:  []string{SourceLocale},
		keys:     make([]string, 0, len(sources)),
	}
	for k := range sources {
		c.keys = append(c.keys, k)
	}
	slices.Sort(c.keys)
	c.pluralBases = PluralBases(c.keys)
	for _, b := range pluralBases {
		c.pluralBases[b] = true
	}
	return c
}

// PluralBases reads plural keys off a key set: a base is plural when it carries
// more than one category, which one key merely ending in a category word cannot
// (docs/adr/0004).
func PluralBases(keys []string) map[string]bool {
	seen := map[string][]string{}
	for _, k := range keys {
		if base, cat, ok := SplitPlural(k); ok && !slices.Contains(seen[base], cat) {
			seen[base] = append(seen[base], cat)
		}
	}
	out := map[string]bool{}
	for base, cats := range seen {
		if len(cats) > 1 {
			out[base] = true
		}
	}
	return out
}

// Locales returns every locale the platform ships, sorted.
func (c *Catalog) Locales() []string { return slices.Clone(c.locales) }

// Keys returns every declared key, sorted.
func (c *Catalog) Keys() []string { return slices.Clone(c.keys) }

// KeysIn returns the keys one locale's file actually declares, sorted — what a
// contract test compares against the keys that locale ought to carry.
func (c *Catalog) KeysIn(locale string) []string {
	return slices.Sorted(maps.Keys(c.byLocale[Normalize(locale)]))
}

// Namespaces returns every grouping prefix in use, sorted.
func (c *Catalog) Namespaces() []string {
	var out []string
	for _, k := range c.keys {
		if ns := Namespace(k); ns != "" && !slices.Contains(out, ns) {
			out = append(out, ns)
		}
	}
	slices.Sort(out)
	return out
}

// Namespace groups a key by everything before its first dot, so a large project
// stays navigable without a second field to keep in step with the key.
func Namespace(key string) string {
	if i := strings.Index(key, "."); i > 0 {
		return key[:i]
	}
	return ""
}

// Source returns the English a key was authored with. A plural sibling English
// does not have — Latvian's zero — sources from the base's other form, which is
// what a translator is working from.
func (c *Catalog) Source(key string) string {
	if v, ok := c.byLocale[SourceLocale][key]; ok {
		return v
	}
	if base, _, ok := SplitPlural(key); ok && c.pluralBases[base] {
		return c.byLocale[SourceLocale][PluralKey(base, CatOther)]
	}
	return ""
}

// Declared reports whether the key exists in the platform project.
func (c *Catalog) Declared(key string) bool {
	_, ok := c.byLocale[SourceLocale][key]
	return ok
}

// DeclaredFor reports whether a locale has this key to translate. It admits the
// plural siblings the language itself needs: a Latvian zero form is writable
// even though English declares no such key.
func (c *Catalog) DeclaredFor(locale, key string) bool {
	if c.Declared(key) {
		return true
	}
	base, cat, ok := SplitPlural(key)
	return ok && c.pluralBases[base] && slices.Contains(Categories(locale), cat)
}

// IsPluralBase reports whether a key's text is a set of category siblings.
func (c *Catalog) IsPluralBase(base string) bool { return c.pluralBases[base] }

// LocaleKeys is every key as it exists for one language: ordinary keys as
// declared, and each plural base expanded into exactly that language's
// categories. This is what the editor lists and what a bundle carries.
//
// Memoized: a catalog is immutable once built, and every publish, bundle read and
// key listing asks for the same expansion.
func (c *Catalog) LocaleKeys(locale string) []string {
	locale = Normalize(locale)
	if keys, ok := c.localeKeys.Load(locale); ok {
		return keys.([]string)
	}
	keys := c.expandFor(locale)
	c.localeKeys.Store(locale, keys)
	return keys
}

func (c *Catalog) expandFor(locale string) []string {
	cats := Categories(locale)
	expanded := map[string]bool{}
	out := make([]string, 0, len(c.keys))
	for _, k := range c.keys {
		base, _, ok := SplitPlural(k)
		if !ok || !c.pluralBases[base] {
			out = append(out, k)
			continue
		}
		if expanded[base] {
			continue
		}
		expanded[base] = true
		for _, cat := range cats {
			out = append(out, PluralKey(base, cat))
		}
	}
	slices.Sort(out)
	return out
}

// Default returns the shipped text for a key in a locale.
func (c *Catalog) Default(locale, key string) (string, bool) {
	v, ok := c.byLocale[Normalize(locale)][key]
	return v, ok
}

// Overrides is a tenant's own text, by locale then key. A missing entry means
// the tenant did not change that string.
type Overrides map[string]map[string]string

// Resolve returns the text to render, trying the requested locale, then the
// tenant's fallback, then the source language, then the key itself. Within a
// locale a tenant override beats the shipped default, so a fully translated
// fallback never outranks the language actually asked for.
func (c *Catalog) Resolve(ov Overrides, locale, fallback, key string) string {
	return c.resolve(ov, c.chain(locale, fallback), key)
}

// chain is the languages a lookup walks, normalized once for a whole bundle.
func (c *Catalog) chain(locale, fallback string) [3]string {
	return [3]string{Normalize(locale), Normalize(fallback), SourceLocale}
}

func (c *Catalog) resolve(ov Overrides, chain [3]string, key string) string {
	// A category the language has but the text does not reads as the other form
	// rather than falling out of the language: an untranslated Latvian zero is
	// better Latvian than English.
	alt := ""
	if base, cat, ok := SplitPlural(key); ok && c.pluralBases[base] && cat != CatOther {
		alt = PluralKey(base, CatOther)
	}
	for _, l := range chain {
		if l == "" {
			continue
		}
		if v := c.textIn(ov, l, key); v != "" {
			return v
		}
		if alt != "" {
			if v := c.textIn(ov, l, alt); v != "" {
				return v
			}
		}
	}
	return key
}

// textIn is one language's text for a key: the tenant's own first, then Sild's.
func (c *Catalog) textIn(ov Overrides, locale, key string) string {
	if v := ov[locale][key]; v != "" {
		return v
	}
	return c.byLocale[locale][key]
}

// Bundle is every key resolved for one locale — what a client holds and renders
// from. Keys missing in the locale resolve through the same chain, so a bundle
// is always complete and a client never has to fall back on its own.
func (c *Catalog) Bundle(ov Overrides, locale, fallback string) map[string]string {
	keys := c.LocaleKeys(locale)
	chain := c.chain(locale, fallback)
	out := make(map[string]string, len(keys))
	for _, k := range keys {
		out[k] = c.resolve(ov, chain, k)
	}
	return out
}

// ValidLanguage reports whether a normalized tag is a language subtag: the two
// or three letters every ISO 639 code has. Anything else is a typo that would be
// stored, offered in a picker, and never match a device.
func ValidLanguage(tag string) bool {
	if len(tag) < 2 || len(tag) > 3 {
		return false
	}
	for _, r := range tag {
		if r < 'a' || r > 'z' {
			return false
		}
	}
	return true
}

// Normalize lowercases a BCP-47 tag's language subtag and drops the rest. Sild
// ships flat languages, so "en-GB" and "en" are one locale.
func Normalize(tag string) string {
	tag = strings.TrimSpace(tag)
	if tag == "" {
		return ""
	}
	if i := strings.IndexAny(tag, "-_"); i > 0 {
		tag = tag[:i]
	}
	return strings.ToLower(tag)
}
