package i18n_test

import (
	_ "embed"
	"encoding/json"
	"slices"
	"testing"

	"github.com/bitllow/sild/backend/internal/i18n"
)

//go:embed plural-cases.json
var pluralCases []byte

type pluralCase struct {
	Locale   string `json:"locale"`
	Count    int    `json:"count"`
	Category string `json:"category"`
}

func cases(t *testing.T) []pluralCase {
	t.Helper()
	var doc struct {
		Cases []pluralCase `json:"cases"`
	}
	if err := json.Unmarshal(pluralCases, &doc); err != nil {
		t.Fatalf("plural-cases.json: %v", err)
	}
	if len(doc.Cases) == 0 {
		t.Fatal("plural-cases.json declares no cases")
	}
	return doc.Cases
}

func TestCategoryFollowsTheSharedCaseTable(t *testing.T) {
	for _, c := range cases(t) {
		if got := i18n.Category(c.Locale, c.Count); got != c.Category {
			t.Errorf("%s/%d: got %s, want %s", c.Locale, c.Count, got, c.Category)
		}
	}
}

// Every case the table names has to be a category the editor would have offered,
// or a translator could never write the text the runtime asks for.
func TestEveryCaseLandsInACategoryTheLocaleOffers(t *testing.T) {
	for _, c := range cases(t) {
		if cats := i18n.Categories(c.Locale); !slices.Contains(cats, c.Category) {
			t.Errorf("%s offers %v, which excludes %s", c.Locale, cats, c.Category)
		}
	}
}

func TestALanguageWithNoTableEntryStillPluralizes(t *testing.T) {
	if got := i18n.Category("qq", 1); got != i18n.CatOne {
		t.Errorf("unknown language at 1: got %s", got)
	}
	if got := i18n.Category("qq", 7); got != i18n.CatOther {
		t.Errorf("unknown language at 7: got %s", got)
	}
}

func TestCategoriesAlwaysEndInOther(t *testing.T) {
	for _, locale := range []string{"en", "lv", "lt", "ru", "ja", "qq"} {
		cats := i18n.Categories(locale)
		if len(cats) == 0 || cats[len(cats)-1] != i18n.CatOther {
			t.Errorf("%s: %v does not end in other", locale, cats)
		}
	}
}

func TestSplitPluralOnlyClaimsACategorySuffix(t *testing.T) {
	base, cat, ok := i18n.SplitPlural("cart.items.few")
	if !ok || base != "cart.items" || cat != "few" {
		t.Errorf("got %q %q %v", base, cat, ok)
	}
	if _, _, ok := i18n.SplitPlural("widget.home.title"); ok {
		t.Error("an ordinary key read as a plural sibling")
	}
	if _, _, ok := i18n.SplitPlural("standalone"); ok {
		t.Error("a key with no dot read as a plural sibling")
	}
}

func TestLatvianGetsAZeroFormEnglishNeverDeclares(t *testing.T) {
	cat := i18n.Platform()
	const base = "widget.home.agentsOnline"
	if !cat.IsPluralBase(base) {
		t.Fatalf("%s is not a plural base", base)
	}
	lv := cat.LocaleKeys("lv")
	if !slices.Contains(lv, base+".zero") {
		t.Error("lv is not offered a zero form")
	}
	if slices.Contains(lv, base+".few") {
		t.Error("lv is offered few, which Latvian does not have")
	}
	if en := cat.LocaleKeys("en"); slices.Contains(en, base+".zero") {
		t.Error("en is offered zero, which English does not have")
	}
}

// The zero form Latvian needs renders as Latvian even before anyone writes it.
func TestAnUnwrittenCategoryFallsBackWithinItsLanguage(t *testing.T) {
	cat := i18n.Platform()
	const base = "widget.home.agentsOnline"
	got := cat.Resolve(nil, "lt", "en", base+".few")
	if want, _ := cat.Default("lt", base+".few"); got != want || got == "" {
		t.Fatalf("lt few: got %q want %q", got, want)
	}
	// Estonian has no few form at all, so asking for one lands on Estonian's other.
	et, _ := cat.Default("et", base+".other")
	if got := cat.Resolve(nil, "et", "en", base+".few"); got != et {
		t.Errorf("et few: got %q, want Estonian's other %q", got, et)
	}
}

func TestABundleCarriesTheLanguagesOwnCategories(t *testing.T) {
	cat := i18n.Platform()
	const base = "widget.home.agentsOnline"
	ru := cat.Bundle(nil, "ru", "en")
	for _, want := range []string{"one", "few", "many", "other"} {
		if ru[base+"."+want] == "" {
			t.Errorf("ru bundle is missing %s", want)
		}
	}
	if _, ok := ru[base+".zero"]; ok {
		t.Error("ru bundle carries zero, which Russian does not have")
	}
	lv := cat.Bundle(nil, "lv", "en")
	if lv[base+".zero"] == "" {
		t.Error("lv bundle is missing zero")
	}
}
