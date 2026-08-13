package i18n

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"strings"
	"sync"
)

// The CLDR categories, in CLDR's own order. A plural key is a sibling per category
// the language has (docs/adr/0004).
const (
	CatZero  = "zero"
	CatOne   = "one"
	CatTwo   = "two"
	CatFew   = "few"
	CatMany  = "many"
	CatOther = "other"
)

var everyCategory = []string{CatZero, CatOne, CatTwo, CatFew, CatMany, CatOther}

// EveryCategory is every category any language has — what removing a plural key
// has to sweep, whatever language wrote which form.
func EveryCategory() []string { return slices.Clone(everyCategory) }

//go:embed plurals.json
var pluralFS []byte

type pluralTable struct {
	Default  string              `json:"default"`
	Families map[string][]string `json:"families"`
	Locales  map[string]string   `json:"locales"`
}

var plurals = sync.OnceValue(func() *pluralTable {
	var t pluralTable
	if err := json.Unmarshal(pluralFS, &t); err != nil {
		panic(fmt.Sprintf("i18n: plurals.json: %v", err))
	}
	if _, ok := t.Families[t.Default]; !ok {
		panic("i18n: plurals.json names a default family it does not define")
	}
	return &t
})

// Family names the rule that picks a category for a language. Unknown languages
// get the default rather than no plural support at all.
func Family(locale string) string {
	t := plurals()
	if f, ok := t.Locales[Normalize(locale)]; ok {
		return f
	}
	return t.Default
}

// TableLocales is every language the table names, so a contract test can hold the
// generated client tables to it.
func TableLocales() []string {
	return slices.Sorted(maps.Keys(plurals().Locales))
}

// Categories is exactly the categories a language has — what the editor offers a
// translator, so Latvian is asked for zero and Estonian is not.
func Categories(locale string) []string {
	t := plurals()
	cats, ok := t.Families[Family(locale)]
	if !ok {
		cats = t.Families[t.Default]
	}
	return slices.Clone(cats)
}

// Category picks the plural category for a count. Integers only: Sild's message
// format takes a count, not a formatted decimal (docs/adr/0004).
func Category(locale string, n int) string {
	if n < 0 {
		n = -n
	}
	mod10, mod100 := n%10, n%100
	family := Family(locale)
	switch family {
	case "other":
		return CatOther

	case "hindi":
		if n <= 1 {
			return CatOne
		}
		return CatOther

	case "romance", "romance_zero":
		if n == 1 || (n == 0 && family == "romance_zero") {
			return CatOne
		}
		// CLDR gives Romance a "many" for round millions, which is what a compact
		// "2 million" reads as.
		if n != 0 && n%1_000_000 == 0 {
			return CatMany
		}
		return CatOther

	case "czech":
		if n == 1 {
			return CatOne
		}
		if n >= 2 && n <= 4 {
			return CatFew
		}
		return CatOther

	case "romanian":
		if n == 1 {
			return CatOne
		}
		if n == 0 || (mod100 >= 1 && mod100 <= 19) {
			return CatFew
		}
		return CatOther

	case "lithuanian":
		if mod100 >= 11 && mod100 <= 19 {
			return CatOther
		}
		if mod10 == 1 {
			return CatOne
		}
		if mod10 >= 2 && mod10 <= 9 {
			return CatFew
		}
		return CatOther

	case "latvian":
		if mod10 == 0 || (mod100 >= 11 && mod100 <= 19) {
			return CatZero
		}
		if mod10 == 1 && mod100 != 11 {
			return CatOne
		}
		return CatOther

	case "hebrew":
		if n == 1 {
			return CatOne
		}
		if n == 2 {
			return CatTwo
		}
		return CatOther

	case "slovenian":
		switch mod100 {
		case 1:
			return CatOne
		case 2:
			return CatTwo
		case 3, 4:
			return CatFew
		}
		return CatOther

	case "arabic":
		switch {
		case n == 0:
			return CatZero
		case n == 1:
			return CatOne
		case n == 2:
			return CatTwo
		case mod100 >= 3 && mod100 <= 10:
			return CatFew
		case mod100 >= 11 && mod100 <= 99:
			return CatMany
		}
		return CatOther

	case "welsh":
		switch n {
		case 0:
			return CatZero
		case 1:
			return CatOne
		case 2:
			return CatTwo
		case 3:
			return CatFew
		case 6:
			return CatMany
		}
		return CatOther

	case "irish":
		switch {
		case n == 1:
			return CatOne
		case n == 2:
			return CatTwo
		case n >= 3 && n <= 6:
			return CatFew
		case n >= 7 && n <= 10:
			return CatMany
		}
		return CatOther

	case "maltese":
		switch {
		case n == 1:
			return CatOne
		case n == 2:
			return CatTwo
		case n == 0 || (mod100 >= 3 && mod100 <= 10):
			return CatFew
		case mod100 >= 11 && mod100 <= 19:
			return CatMany
		}
		return CatOther

	// Three families share the "few" test and differ only in what one is and what
	// everything else is: Polish's one is exactly one where Russian's is any number
	// ending in it, and Serbo-Croatian has no many.
	case "polish", "slavic", "serbocroatian":
		one := mod10 == 1 && mod100 != 11
		if family == "polish" {
			one = n == 1
		}
		switch {
		case one:
			return CatOne
		case mod10 >= 2 && mod10 <= 4 && (mod100 < 12 || mod100 > 14):
			return CatFew
		case family == "serbocroatian":
			return CatOther
		}
		return CatMany
	}
	if n == 1 {
		return CatOne
	}
	return CatOther
}

// SplitPlural takes a sibling key apart. ok is false for an ordinary key, so a
// key ending in a word that merely reads like a category ("chat.other") is only
// a plural sibling if its base actually declares one.
func SplitPlural(key string) (base, category string, ok bool) {
	i := strings.LastIndex(key, ".")
	if i <= 0 {
		return key, "", false
	}
	cat := key[i+1:]
	if !slices.Contains(everyCategory, cat) {
		return key, "", false
	}
	return key[:i], cat, true
}

// PluralKey is the sibling key holding one category of a plural.
func PluralKey(base, category string) string { return base + "." + category }

// Placeholders are the `{name}`s a text will have filled in, in the order they
// appear. A translator has to know what they are to place them where their own
// language's word order wants them.
func Placeholders(text string) []string {
	var out []string
	for {
		open := strings.IndexByte(text, '{')
		if open < 0 {
			return out
		}
		text = text[open+1:]
		close := strings.IndexByte(text, '}')
		if close < 0 {
			return out
		}
		if name := text[:close]; isPlaceholderName(name) && !slices.Contains(out, name) {
			out = append(out, name)
		}
		text = text[close+1:]
	}
}

// isPlaceholderName matches what every runtime's interpolation accepts: `\w+`.
func isPlaceholderName(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '_':
		default:
			return false
		}
	}
	return true
}
