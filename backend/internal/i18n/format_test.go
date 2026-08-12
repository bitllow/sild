package i18n_test

import (
	"maps"
	"os"
	"os/exec"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/bitllow/sild/backend/internal/i18n"
)

// rows is a small set with an ordinary key, a quoted one, and a plural.
func rows() []i18n.Row {
	return []i18n.Row{
		{Key: "widget.home.cta", Source: "Send us a message", Value: `Sazinieties ar "mums"`},
		{Key: "widget.home.agentsOnline.zero", Source: "{count} agents online", Value: "{count} aģentu tiešsaistē"},
		{Key: "widget.home.agentsOnline.one", Source: "{count} agent online", Value: "{count} aģents tiešsaistē"},
		{Key: "widget.home.agentsOnline.other", Source: "{count} agents online", Value: "{count} aģenti tiešsaistē"},
	}
}

func flat(rs []i18n.Row) map[string]string {
	out := map[string]string{}
	for _, r := range rs {
		out[r.Key] = r.Value
	}
	return out
}

// Every format a file may arrive in round-trips: what Sild wrote out is what it
// reads back, plural siblings and all.
func TestEveryFormatRoundTripsItsRows(t *testing.T) {
	want := flat(rows())
	for _, format := range []i18n.Format{i18n.FormatJSON, i18n.FormatCSV, i18n.FormatAndroid} {
		raw, err := i18n.Render(format, "lv", rows())
		if err != nil {
			t.Fatalf("%s render: %v", format, err)
		}
		got, err := i18n.Parse(format, raw)
		if err != nil {
			t.Fatalf("%s parse: %v\n%s", format, err, raw)
		}
		if !maps.Equal(resolve(got, want), want) {
			t.Errorf("%s round-trip: got %v want %v\n%s", format, got, want, raw)
		}
	}
}

// resolve maps parsed names onto declared keys the way import does.
func resolve(got, declared map[string]string) map[string]string {
	m := i18n.NewKeyMatcher(slices.Collect(maps.Keys(declared)))
	out := map[string]string{}
	for name, value := range got {
		key, ok := m.Resolve(name)
		if !ok {
			key = name
		}
		out[key] = value
	}
	return out
}

// iOS splits: .strings holds no plurals, and the stringsdict holds only those.
func TestTheIOSPairSplitsPluralsFromTheRest(t *testing.T) {
	strs, err := i18n.Render(i18n.FormatIOS, "lv", rows())
	if err != nil {
		t.Fatalf("render .strings: %v", err)
	}
	plain, err := i18n.Parse(i18n.FormatIOS, strs)
	if err != nil {
		t.Fatalf("parse .strings: %v\n%s", err, strs)
	}
	if len(plain) != 1 || plain["widget.home.cta"] != `Sazinieties ar "mums"` {
		t.Fatalf(".strings carried %v", plain)
	}

	dict, err := i18n.Render(i18n.FormatIOSPlurals, "lv", rows())
	if err != nil {
		t.Fatalf("render .stringsdict: %v", err)
	}
	plurals, err := i18n.Parse(i18n.FormatIOSPlurals, dict)
	if err != nil {
		t.Fatalf("parse .stringsdict: %v\n%s", err, dict)
	}
	for _, cat := range []string{"zero", "one", "other"} {
		key := "widget.home.agentsOnline." + cat
		if plurals[key] != flat(rows())[key] {
			t.Errorf("%s: got %q", key, plurals[key])
		}
	}
	if len(plurals) != 3 {
		t.Errorf(".stringsdict carried %d entries, want 3", len(plurals))
	}
}

// A plist dict is ordered: every <key> is followed by its own value. Grouped
// elements parse back through a matching parser and are still a file Xcode
// rejects, so the shape itself is what this asserts.
func TestTheStringsdictIsAnOrderedPlist(t *testing.T) {
	raw, err := i18n.Render(i18n.FormatIOSPlurals, "lv", rows())
	if err != nil {
		t.Fatal(err)
	}
	doc := string(raw)
	element := regexp.MustCompile(`<(key|string|dict|/dict)>?`)
	var shape []string
	for _, m := range element.FindAllStringSubmatch(doc, -1) {
		shape = append(shape, m[1])
	}
	// A key is never followed by another key: its value comes first.
	for i := 0; i < len(shape)-1; i++ {
		if shape[i] == "key" && shape[i+1] == "key" {
			t.Fatalf("two keys in a row — the values are grouped, not paired:\n%s", doc)
		}
	}
	for _, want := range []string{
		"<key>NSStringLocalizedFormatKey</key>",
		"<string>%#@count@</string>",
		"<key>NSStringFormatSpecTypeKey</key>",
		"<string>NSStringPluralRuleType</string>",
	} {
		if !strings.Contains(doc, want) {
			t.Errorf("missing %s:\n%s", want, doc)
		}
	}
}

// The platform's own parser is the authority on whether a .stringsdict is a
// plist at all. Skipped where plutil is not installed — it is macOS only.
func TestTheStringsdictPassesPlutil(t *testing.T) {
	plutil, err := exec.LookPath("plutil")
	if err != nil {
		t.Skip("plutil is macOS only")
	}
	out, err := i18n.Render(i18n.FormatIOSPlurals, "lv", rows())
	if err != nil {
		t.Fatal(err)
	}
	path := t.TempDir() + "/Localizable.stringsdict"
	if err := os.WriteFile(path, out, 0o600); err != nil {
		t.Fatal(err)
	}
	if res, err := exec.Command(plutil, "-lint", path).CombinedOutput(); err != nil {
		t.Fatalf("plutil rejected the export: %v\n%s\n%s", err, res, out)
	}
}

func TestAndroidReassemblesSiblingsIntoOnePluralsElement(t *testing.T) {
	raw, err := i18n.Render(i18n.FormatAndroid, "lv", rows())
	if err != nil {
		t.Fatal(err)
	}
	xml := string(raw)
	if !strings.Contains(xml, `<plurals name="widget_home_agentsOnline">`) {
		t.Errorf("no <plurals> element:\n%s", xml)
	}
	if !strings.Contains(xml, `<item quantity="zero">`) {
		t.Errorf("no zero item:\n%s", xml)
	}
	if strings.Contains(xml, `name="widget_home_agentsOnline_one"`) {
		t.Errorf("a sibling leaked out as its own string:\n%s", xml)
	}
}

// The spreadsheet is what a translator with no account fills in, so the source
// text has to travel beside the translation and come back as the translation.
func TestTheSpreadsheetCarriesTheSourceBesideTheTranslation(t *testing.T) {
	raw, err := i18n.Render(i18n.FormatSheet, "lv", rows())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "key,source,translation") {
		t.Fatalf("no header:\n%s", raw)
	}
	if !strings.Contains(string(raw), "Send us a message") {
		t.Errorf("the source text is missing:\n%s", raw)
	}
	got, err := i18n.Parse(i18n.FormatCSV, raw)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if !maps.Equal(got, flat(rows())) {
		t.Errorf("filled sheet read back as %v", got)
	}
}

// Most tools export nested JSON, so reading it is worth more than refusing it.
func TestNestedJSONReadsAsDottedKeys(t *testing.T) {
	got, err := i18n.Parse(i18n.FormatJSON, []byte(`{"widget":{"home":{"cta":"Hei"}}}`))
	if err != nil {
		t.Fatal(err)
	}
	if got["widget.home.cta"] != "Hei" {
		t.Errorf("got %v", got)
	}
}

func TestAMalformedFileIsRefusedRatherThanPartlyRead(t *testing.T) {
	for _, tc := range []struct {
		format i18n.Format
		raw    string
	}{
		{i18n.FormatJSON, `{"a": ["not text"]}`},
		{i18n.FormatIOS, `"unterminated = "x";`},
		{i18n.FormatAndroid, `<resources><string name="a">x</resources>`},
		{i18n.FormatIOSPlurals, `<plist version="1.0"><dict/></plist>`},
	} {
		if _, err := i18n.Parse(tc.format, []byte(tc.raw)); err == nil {
			t.Errorf("%s accepted %q", tc.format, tc.raw)
		}
	}
}

func TestAnUnknownFormatIsRefused(t *testing.T) {
	if _, err := i18n.Parse("yaml", []byte("a: b")); err == nil {
		t.Error("yaml was accepted")
	}
	if _, err := i18n.Render("yaml", "en", rows()); err == nil {
		t.Error("yaml was rendered")
	}
	if i18n.KnownFormat("sheet") {
		t.Error("the spreadsheet is an export shape, not an import one")
	}
}

// The dot-to-underscore transform is many-to-one, so two keys can want one
// resource name. Emitting both would produce a file Android rejects and an import
// that restores text onto the wrong key.
func TestAndroidRefusesTwoKeysThatWantOneResourceName(t *testing.T) {
	_, err := i18n.Render(i18n.FormatAndroid, "en", []i18n.Row{
		{Key: "checkout.pay", Value: "Pay"},
		{Key: "checkout_pay", Value: "Pay again"},
	})
	if err == nil {
		t.Fatal("both keys were exported under one resource name")
	}
	if !strings.Contains(err.Error(), "checkout_pay") {
		t.Errorf("the error does not name the collision: %v", err)
	}
}
