// Package i18n holds Sild's own strings. The canonical files in locales/ are the
// source of truth (docs/adr/0003) — they are embedded here and generated into the
// client bundles, so no surface keeps its own copy.
package i18n

import (
	"embed"
	"encoding/json"
	"fmt"
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

//go:embed locales/*.json
var localeFS embed.FS

// Catalog is the platform project's keys and their text per locale.
type Catalog struct {
	byLocale map[string]map[string]string
	locales  []string
	keys     []string
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
	return c, nil
}

// NewCatalog builds a catalog whose only shipped locale is the source one — the
// shape a tenant-owned project has, where the declared keys are the sources and
// every other language arrives as an override.
func NewCatalog(sources map[string]string) *Catalog {
	c := &Catalog{
		byLocale: map[string]map[string]string{SourceLocale: sources},
		locales:  []string{SourceLocale},
		keys:     make([]string, 0, len(sources)),
	}
	for k := range sources {
		c.keys = append(c.keys, k)
	}
	slices.Sort(c.keys)
	return c
}

// Locales returns every locale the platform ships, sorted.
func (c *Catalog) Locales() []string { return slices.Clone(c.locales) }

// Keys returns every declared key, sorted.
func (c *Catalog) Keys() []string { return slices.Clone(c.keys) }

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

// Source returns the English a key was authored with.
func (c *Catalog) Source(key string) string { return c.byLocale[SourceLocale][key] }

// Declared reports whether the key exists in the platform project.
func (c *Catalog) Declared(key string) bool {
	_, ok := c.byLocale[SourceLocale][key]
	return ok
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
	for _, l := range []string{Normalize(locale), Normalize(fallback), SourceLocale} {
		if l == "" {
			continue
		}
		if v, ok := ov[l][key]; ok && v != "" {
			return v
		}
		if v, ok := c.Default(l, key); ok && v != "" {
			return v
		}
	}
	return key
}

// Bundle is every key resolved for one locale — what a client holds and renders
// from. Keys missing in the locale resolve through the same chain, so a bundle
// is always complete and a client never has to fall back on its own.
func (c *Catalog) Bundle(ov Overrides, locale, fallback string) map[string]string {
	out := make(map[string]string, len(c.keys))
	for _, k := range c.keys {
		out[k] = c.Resolve(ov, locale, fallback, k)
	}
	return out
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
