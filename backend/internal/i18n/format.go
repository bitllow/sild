package i18n

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"maps"
	"slices"
	"sort"
	"strings"
)

// A translation file shape. Native formats are an import path for migration and
// an export path for a tenant who still feeds their own build — never how Sild
// delivers text (docs/adr/0003).
type Format string

const (
	FormatJSON Format = "json"
	FormatCSV  Format = "csv"
	// FormatSheet is a spreadsheet for a translator with no account: the source
	// text beside the translation. Export only — it is a working document.
	FormatSheet Format = "sheet"
	// FormatAndroid is res/values/strings.xml, plural siblings reassembled into
	// <plurals>. Resource names cannot hold a dot, so the keys carry underscores
	// and import resolves them back against the project's declarations.
	FormatAndroid Format = "android"
	// FormatIOS is Localizable.strings, which holds no plurals at all; the plural
	// keys travel in FormatIOSPlurals.
	FormatIOS        Format = "ios"
	FormatIOSPlurals Format = "ios-plurals"
	// The typed accessors a tenant's own build looks its keys up through, so a key
	// renamed in the editor is a compile error rather than a blank label. Export
	// only: source is generated from the declarations, never parsed back.
	FormatKotlin     Format = "kotlin"
	FormatSwift      Format = "swift"
	FormatTypeScript Format = "typescript"
)

// Row is one key's text, as a file carries it.
type Row struct {
	Key    string
	Source string
	Value  string
	// Plural marks a category sibling of a key the project DECLARED plural. Two
	// ordinary keys called status.one and status.other are not one (docs/adr/0004),
	// so the renderers ask this rather than reading the suffix.
	Plural bool
}

// pluralOf takes a row's base and category apart, for a row that is one.
func pluralOf(r Row) (base, category string, ok bool) {
	if !r.Plural {
		return r.Key, "", false
	}
	return SplitPlural(r.Key)
}

// parseFormats are the shapes a file may arrive in.
var parseFormats = []Format{FormatJSON, FormatCSV, FormatAndroid, FormatIOS, FormatIOSPlurals}

// renderFormats are the shapes a file may leave in.
var renderFormats = []Format{
	FormatJSON, FormatCSV, FormatSheet, FormatAndroid, FormatIOS, FormatIOSPlurals,
	FormatKotlin, FormatSwift, FormatTypeScript,
}

// KnownFormat reports whether a format may be parsed.
func KnownFormat(f Format) bool { return slices.Contains(parseFormats, f) }

// RenderableFormat reports whether a format may be written.
func RenderableFormat(f Format) bool { return slices.Contains(renderFormats, f) }

// Parse reads a file into key → text, plural containers expanded into the sibling
// keys Sild stores. Keys are returned as the file spells them; resolving an
// Android resource name back to a declared key is the caller's job, which is the
// only place the declarations are known.
func Parse(format Format, raw []byte) (map[string]string, error) {
	switch format {
	case FormatJSON:
		return parseJSON(raw)
	case FormatCSV:
		return parseCSV(raw)
	case FormatAndroid:
		return parseAndroid(raw)
	case FormatIOS:
		return parseIOSStrings(raw)
	case FormatIOSPlurals:
		return parseIOSPlurals(raw)
	}
	return nil, fmt.Errorf("unsupported format %q", format)
}

// Render writes rows out. Rows carry sibling keys; the plural formats reassemble
// them, and the others stay flat.
func Render(format Format, locale string, rows []Row) ([]byte, error) {
	sorted := slices.Clone(rows)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Key < sorted[j].Key })
	switch format {
	case FormatJSON:
		return renderJSON(sorted)
	case FormatCSV:
		return renderCSV(sorted, false)
	case FormatSheet:
		return renderCSV(sorted, true)
	case FormatAndroid:
		return renderAndroid(sorted)
	case FormatIOS:
		return renderIOSStrings(sorted)
	case FormatIOSPlurals:
		return renderIOSPlurals(locale, sorted)
	case FormatKotlin, FormatSwift, FormatTypeScript:
		return renderAccessors(format, sorted)
	}
	return nil, fmt.Errorf("unsupported format %q", format)
}

// ContentType is what a browser should do with an export.
func ContentType(f Format) string {
	switch f {
	case FormatJSON:
		return "application/json; charset=utf-8"
	case FormatCSV, FormatSheet:
		return "text/csv; charset=utf-8"
	case FormatAndroid, FormatIOSPlurals:
		return "application/xml; charset=utf-8"
	}
	return "text/plain; charset=utf-8"
}

// SourceFormat reports whether a format is generated code rather than text a
// translator works on — it carries keys, never translations.
func SourceFormat(f Format) bool {
	switch f {
	case FormatKotlin, FormatSwift, FormatTypeScript:
		return true
	}
	return false
}

// Filename is the name an export downloads as.
func Filename(f Format, project, locale string) string {
	switch f {
	case FormatJSON:
		return fmt.Sprintf("%s-%s.json", project, locale)
	case FormatCSV, FormatSheet:
		return fmt.Sprintf("%s-%s.csv", project, locale)
	case FormatAndroid:
		return "strings.xml"
	case FormatIOS:
		return "Localizable.strings"
	case FormatIOSPlurals:
		return "Localizable.stringsdict"
	case FormatKotlin:
		return "SildProjectKeys.kt"
	case FormatSwift:
		return "SildProjectKeys.swift"
	case FormatTypeScript:
		return "sild-project-keys.ts"
	}
	return project + "-" + locale + ".txt"
}

// ── JSON ────────────────────────────────────────────────────────────────────

func parseJSON(raw []byte) (map[string]string, error) {
	var flat map[string]string
	if err := json.Unmarshal(raw, &flat); err == nil {
		return flat, nil
	}
	// A nested document is what most tools export, so read it rather than refuse.
	var nested map[string]any
	if err := json.Unmarshal(raw, &nested); err != nil {
		return nil, fmt.Errorf("not a JSON object of strings: %w", err)
	}
	out := map[string]string{}
	if err := flatten("", nested, out); err != nil {
		return nil, err
	}
	return out, nil
}

func flatten(prefix string, in map[string]any, out map[string]string) error {
	for k, v := range in {
		key := k
		if prefix != "" {
			key = prefix + "." + k
		}
		switch t := v.(type) {
		case string:
			out[key] = t
		case map[string]any:
			if err := flatten(key, t, out); err != nil {
				return err
			}
		default:
			return fmt.Errorf("%s is neither text nor a group", key)
		}
	}
	return nil
}

func renderJSON(rows []Row) ([]byte, error) {
	flat := make(map[string]string, len(rows))
	for _, r := range rows {
		flat[r.Key] = r.Value
	}
	// Indented and key-sorted (encoding/json sorts map keys), so a checked-in
	// export produces a readable diff.
	out, err := json.MarshalIndent(flat, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(out, '\n'), nil
}

// ── CSV and the spreadsheet ─────────────────────────────────────────────────

func parseCSV(raw []byte) (map[string]string, error) {
	recs, err := csv.NewReader(bytes.NewReader(raw)).ReadAll()
	if err != nil {
		return nil, fmt.Errorf("not readable as CSV: %w", err)
	}
	out := map[string]string{}
	for i, rec := range recs {
		if len(rec) < 2 {
			continue
		}
		key := strings.TrimSpace(rec[0])
		if key == "" || (i == 0 && strings.EqualFold(key, "key")) {
			continue // the header row, or a blank line
		}
		// A spreadsheet carries key, source, translation; a plain CSV key, value.
		// The last column is the text either way, which is what a translator filled.
		out[key] = rec[len(rec)-1]
	}
	return out, nil
}

func renderCSV(rows []Row, withSource bool) ([]byte, error) {
	var b strings.Builder
	w := csv.NewWriter(&b)
	header := []string{"key", "translation"}
	if withSource {
		header = []string{"key", "source", "translation"}
	}
	if err := w.Write(header); err != nil {
		return nil, err
	}
	for _, r := range rows {
		rec := []string{r.Key, r.Value}
		if withSource {
			rec = []string{r.Key, r.Source, r.Value}
		}
		if err := w.Write(rec); err != nil {
			return nil, err
		}
	}
	w.Flush()
	if err := w.Error(); err != nil {
		return nil, err
	}
	return []byte(b.String()), nil
}

// ── Android strings.xml ─────────────────────────────────────────────────────

// AndroidName is a key as an Android resource name: a resource name cannot hold a
// dot. Import reverses it against the declared keys, so the transform being
// many-to-one does not lose anything.
func AndroidName(key string) string { return strings.ReplaceAll(key, ".", "_") }

// KeyMatcher resolves keys as a file spells them onto the keys a project actually
// declares. An exact match always wins; a format that mangles keys supplies the
// spellings it would have produced, and an ambiguous one resolves to nothing
// rather than to a guess.
type KeyMatcher struct {
	exact   map[string]bool
	aliases map[string]string
	// ambiguous names two declared keys share an alias, so neither may claim it.
	ambiguous map[string]bool
}

// NewKeyMatcher builds the matcher for one format. Only Android mangles a key —
// its resource names cannot hold a dot — so only Android admits an alias, and a
// JSON file naming `widget_home_cta` means that key or nothing.
func NewKeyMatcher(format Format, declared []string) KeyMatcher {
	m := KeyMatcher{
		exact:     make(map[string]bool, len(declared)),
		aliases:   map[string]string{},
		ambiguous: map[string]bool{},
	}
	for _, key := range declared {
		m.exact[key] = true
	}
	if format != FormatAndroid {
		return m
	}
	for _, key := range declared {
		for _, alias := range androidSpellings(key) {
			if alias == key {
				continue
			}
			if first, taken := m.aliases[alias]; taken && first != key {
				m.ambiguous[alias] = true
				continue
			}
			m.aliases[alias] = key
		}
	}
	return m
}

// androidSpellings is how a key can come back from a strings.xml: the fully
// underscored resource name, and — for a plural sibling — the underscored base
// with its category still dotted, which is what reading a <plurals> produces.
func androidSpellings(key string) []string {
	out := []string{AndroidName(key)}
	if base, cat, ok := SplitPlural(key); ok {
		out = append(out, PluralKey(AndroidName(base), cat))
	}
	return out
}

// Resolve returns the declared key this name means, or false if none does.
func (m KeyMatcher) Resolve(name string) (string, bool) {
	if m.exact[name] {
		return name, true
	}
	// A key spelled the way this format would have written it, unless two declared
	// keys want that spelling — then the file has to name one of them exactly.
	if m.ambiguous[name] {
		return name, false
	}
	key, ok := m.aliases[name]
	if !ok {
		return name, false
	}
	return key, true
}

type androidResources struct {
	XMLName xml.Name         `xml:"resources"`
	Strings []androidString  `xml:"string"`
	Plurals []androidPlurals `xml:"plurals"`
}

type androidString struct {
	Name string `xml:"name,attr"`
	Text string `xml:",chardata"`
}

type androidPlurals struct {
	Name  string        `xml:"name,attr"`
	Items []androidItem `xml:"item"`
}

type androidItem struct {
	Quantity string `xml:"quantity,attr"`
	Text     string `xml:",chardata"`
}

func parseAndroid(raw []byte) (map[string]string, error) {
	var res androidResources
	if err := xml.Unmarshal(raw, &res); err != nil {
		return nil, fmt.Errorf("not readable as strings.xml: %w", err)
	}
	out := map[string]string{}
	for _, s := range res.Strings {
		out[s.Name] = s.Text
	}
	for _, p := range res.Plurals {
		for _, item := range p.Items {
			if !slices.Contains(everyCategory, item.Quantity) {
				return nil, fmt.Errorf("%s has no such plural quantity as %q", p.Name, item.Quantity)
			}
			out[PluralKey(p.Name, item.Quantity)] = item.Text
		}
	}
	return out, nil
}

func renderAndroid(rows []Row) ([]byte, error) {
	res := androidResources{}
	byBase := map[string]*androidPlurals{}
	// The dot-to-underscore transform is many-to-one, so two keys can want one
	// resource name. Refused rather than emitted twice: a duplicate name is a file
	// Android rejects, and importing it back would restore text onto one key.
	if err := uniqueNames(keysOf(rows), AndroidName, "resource name", "export for Android"); err != nil {
		return nil, err
	}
	for _, r := range rows {
		name := AndroidName(r.Key)

		base, cat, ok := pluralOf(r)
		if !ok {
			res.Strings = append(res.Strings, androidString{Name: name, Text: r.Value})
			continue
		}
		group := AndroidName(base)
		if byBase[group] == nil {
			byBase[group] = &androidPlurals{Name: group}
		}
		byBase[group].Items = append(byBase[group].Items, androidItem{Quantity: cat, Text: r.Value})
	}
	for _, name := range slices.Sorted(maps.Keys(byBase)) {
		res.Plurals = append(res.Plurals, *byBase[name])
	}
	out, err := xml.MarshalIndent(res, "", "    ")
	if err != nil {
		return nil, err
	}
	return append(append([]byte(xml.Header), out...), '\n'), nil
}

// ── iOS .strings and .stringsdict ───────────────────────────────────────────

func parseIOSStrings(raw []byte) (map[string]string, error) {
	out := map[string]string{}
	for line := range strings.SplitSeq(string(raw), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "//") || strings.HasPrefix(line, "/*") {
			continue
		}
		key, value, ok := splitIOSLine(line)
		if !ok {
			return nil, fmt.Errorf("not readable as a .strings entry: %s", line)
		}
		out[key] = value
	}
	return out, nil
}

// splitIOSLine reads `"key" = "value";`, honouring a backslash-escaped quote.
func splitIOSLine(line string) (key, value string, ok bool) {
	line = strings.TrimSuffix(strings.TrimSpace(line), ";")
	k, rest, found := unquoteIOS(strings.TrimSpace(line))
	if !found {
		return "", "", false
	}
	rest = strings.TrimSpace(rest)
	if !strings.HasPrefix(rest, "=") {
		return "", "", false
	}
	v, _, found := unquoteIOS(strings.TrimSpace(strings.TrimPrefix(rest, "=")))
	if !found {
		return "", "", false
	}
	return k, v, true
}

func unquoteIOS(s string) (text, rest string, ok bool) {
	if !strings.HasPrefix(s, `"`) {
		return "", s, false
	}
	var b strings.Builder
	escaped := false
	for i := 1; i < len(s); i++ {
		c := s[i]
		if escaped {
			b.WriteByte(c)
			escaped = false
			continue
		}
		switch c {
		case '\\':
			escaped = true
		case '"':
			return b.String(), s[i+1:], true
		default:
			b.WriteByte(c)
		}
	}
	return "", s, false
}

func quoteIOS(s string) string {
	return `"` + strings.ReplaceAll(strings.ReplaceAll(s, `\`, `\\`), `"`, `\"`) + `"`
}

func renderIOSStrings(rows []Row) ([]byte, error) {
	var b strings.Builder
	for _, r := range rows {
		if r.Plural {
			continue // plurals travel in the stringsdict
		}
		fmt.Fprintf(&b, "%s = %s;\n", quoteIOS(r.Key), quoteIOS(r.Value))
	}
	return []byte(b.String()), nil
}

// A plist dict is ORDERED — each <key> is followed by its value — so entries are
// emitted in order rather than as parallel lists. Xcode rejects the alternative.
type plistEntry struct {
	Key   string
	Value string
	Dict  *plistNode
}

type plistNode struct{ Entries []plistEntry }

func (n plistNode) MarshalXML(e *xml.Encoder, start xml.StartElement) error {
	start.Name = xml.Name{Local: "dict"}
	if err := e.EncodeToken(start); err != nil {
		return err
	}
	for _, entry := range n.Entries {
		if err := e.EncodeElement(entry.Key, xml.StartElement{Name: xml.Name{Local: "key"}}); err != nil {
			return err
		}
		if entry.Dict != nil {
			if err := e.EncodeElement(*entry.Dict, xml.StartElement{Name: xml.Name{Local: "dict"}}); err != nil {
				return err
			}
			continue
		}
		if err := e.EncodeElement(entry.Value, xml.StartElement{Name: xml.Name{Local: "string"}}); err != nil {
			return err
		}
	}
	return e.EncodeToken(xml.EndElement{Name: start.Name})
}

// UnmarshalXML reads a dict back in order, so a key is paired with the value that
// actually follows it rather than with whatever landed at the same index.
func (n *plistNode) UnmarshalXML(d *xml.Decoder, start xml.StartElement) error {
	var pending *plistEntry
	for {
		tok, err := d.Token()
		if err != nil {
			return err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			switch t.Name.Local {
			case "key":
				var key string
				if err := d.DecodeElement(&key, &t); err != nil {
					return err
				}
				n.Entries = append(n.Entries, plistEntry{Key: key})
				pending = &n.Entries[len(n.Entries)-1]
			case "string":
				var value string
				if err := d.DecodeElement(&value, &t); err != nil {
					return err
				}
				if pending == nil {
					return fmt.Errorf("a value with no key before it")
				}
				pending.Value = value
				pending = nil
			case "dict":
				var inner plistNode
				if err := d.DecodeElement(&inner, &t); err != nil {
					return err
				}
				if pending == nil {
					return fmt.Errorf("a dict with no key before it")
				}
				pending.Dict = &inner
				pending = nil
			default:
				if err := d.Skip(); err != nil {
					return err
				}
			}
		case xml.EndElement:
			if t.Name.Local == start.Name.Local {
				return nil
			}
		}
	}
}

type plist struct {
	XMLName xml.Name  `xml:"plist"`
	Version string    `xml:"version,attr"`
	Dict    plistNode `xml:"dict"`
}

func parseIOSPlurals(raw []byte) (map[string]string, error) {
	var doc plist
	if err := xml.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("not readable as a .stringsdict: %w", err)
	}
	out := map[string]string{}
	// The outer dict pairs each plural key with the dict describing it; that dict
	// pairs the format key with the spec dict holding the categories.
	for _, base := range doc.Dict.Entries {
		if base.Dict == nil {
			continue
		}
		for _, spec := range base.Dict.Entries {
			if spec.Dict == nil {
				continue
			}
			for _, form := range spec.Dict.Entries {
				if slices.Contains(everyCategory, form.Key) {
					out[PluralKey(base.Key, form.Key)] = form.Value
				}
			}
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no plural entries found")
	}
	return out, nil
}

func renderIOSPlurals(locale string, rows []Row) ([]byte, error) {
	byBase := map[string][]Row{}
	for _, r := range rows {
		if base, _, ok := pluralOf(r); ok {
			byBase[base] = append(byBase[base], r)
		}
	}
	doc := plist{Version: "1.0"}
	for _, base := range slices.Sorted(maps.Keys(byBase)) {
		// The spec dict names the rule and the value type, then one entry per
		// category; "count" is what Sild's plural text interpolates.
		spec := plistNode{Entries: []plistEntry{
			{Key: "NSStringFormatSpecTypeKey", Value: "NSStringPluralRuleType"},
			{Key: "NSStringFormatValueTypeKey", Value: "d"},
		}}
		for _, r := range byBase[base] {
			_, cat, _ := pluralOf(r)
			spec.Entries = append(spec.Entries, plistEntry{Key: cat, Value: r.Value})
		}
		entry := plistNode{Entries: []plistEntry{
			{Key: "NSStringLocalizedFormatKey", Value: "%#@count@"},
			{Key: "count", Dict: &spec},
		}}
		doc.Dict.Entries = append(doc.Dict.Entries, plistEntry{Key: base, Dict: &entry})
	}
	if len(doc.Dict.Entries) == 0 {
		return nil, fmt.Errorf("%s has no plural keys to export", locale)
	}
	out, err := xml.MarshalIndent(doc, "", "\t")
	if err != nil {
		return nil, err
	}
	return append(append([]byte(xml.Header), out...), '\n'), nil
}

// ── Typed accessors ─────────────────────────────────────────────────────────

// renderAccessors writes a constant per key for a tenant's build to look strings up
// through, so a key renamed in the editor fails their compile. Plural siblings
// collapse to their base, which is what a count is addressed by.
func renderAccessors(format Format, rows []Row) ([]byte, error) {
	plain, plural := []string{}, []string{}
	for _, r := range rows {
		base, _, ok := pluralOf(r)
		if !ok {
			plain = append(plain, r.Key)
			continue
		}
		if !slices.Contains(plural, base) {
			plural = append(plural, base)
		}
	}
	if err := uniqueNames(append(slices.Clone(plain), plural...), Ident, "constant", "export typed keys"); err != nil {
		return nil, err
	}
	if format == FormatTypeScript {
		return accessorFile(typeScriptAccessors(plain, plural)), nil
	}
	return accessorFile(constantAccessors(format, plain, plural)), nil
}

// uniqueNames refuses a key set two of whose keys would be written the same way:
// "pay-now" and "pay_now" are two keys and one constant, and emitting both is a
// file that does not compile. The same rule an Android resource name needs.
func uniqueNames(keys []string, name func(string) string, what, action string) error {
	seen := map[string]string{}
	for _, key := range keys {
		n := name(key)
		if first, taken := seen[n]; taken && first != key {
			return fmt.Errorf("%s and %s would both be the %s %s: rename one to %s",
				first, key, what, n, action)
		}
		seen[n] = key
	}
	return nil
}

func keysOf(rows []Row) []string {
	out := make([]string, 0, len(rows))
	for _, r := range rows {
		out = append(out, r.Key)
	}
	return out
}

func accessorFile(body []string) []byte {
	lines := append([]string{
		"// Generated by Sild from this project's declared keys. Do not edit.",
		"",
	}, body...)
	return []byte(strings.Join(lines, "\n") + "\n")
}

// constantAccessors writes a constant per key. Kotlin and Swift differ only in how
// a constant and a container are spelled.
func constantAccessors(format Format, plain, plural []string) []string {
	open, member, closing := "object SildProjectKeys {", "    const val %s: String = %q", "}"
	nested := []string{"", "    /** Keys addressed by a count. */", "    object Plural {"}
	inner := "        const val %s: String = %q"
	if format == FormatSwift {
		open, member, closing = "public enum SildProjectKeys {", "    public static let %s = %q", "}"
		nested = []string{"", "    /// Keys addressed by a count.", "    public enum Plural {"}
		inner = "        public static let %s = %q"
	}
	out := []string{open}
	for _, key := range plain {
		out = append(out, fmt.Sprintf(member, Ident(key), key))
	}
	out = append(out, nested...)
	for _, base := range plural {
		out = append(out, fmt.Sprintf(inner, Ident(base), base))
	}
	return append(out, "    }", closing)
}

// union writes a TypeScript string-literal union, or `never` for an empty one —
// `type X = ;` does not parse.
func union(name string, keys []string) []string {
	if len(keys) == 0 {
		return []string{fmt.Sprintf("export type %s = never;", name)}
	}
	out := []string{fmt.Sprintf("export type %s =", name)}
	for _, key := range keys {
		out = append(out, fmt.Sprintf("  | %q", key))
	}
	return append(out, ";")
}

func typeScriptAccessors(plain, plural []string) []string {
	out := []string{"/** Every key this project declares: a typo or a renamed key is a type error. */"}
	out = append(out, union("ProjectStringKey", plain)...)
	out = append(out, "", "/** Keys addressed by a count. */")
	return append(out, union("ProjectPluralKey", plural)...)
}

// Ident is a key as an identifier: "checkout.pay" → checkoutPay. A key may start
// with a digit or spell a language keyword, neither of which compiles, so the
// result is prefixed where it would not stand on its own.
func Ident(key string) string {
	var b strings.Builder
	upper := false
	for _, r := range key {
		switch {
		case r == '.' || r == '-' || r == '_':
			upper = b.Len() > 0
		case upper:
			b.WriteString(strings.ToUpper(string(r)))
			upper = false
		default:
			b.WriteRune(r)
		}
	}
	out := b.String()
	if out == "" {
		return "key"
	}
	if first := out[0]; first >= '0' && first <= '9' || reservedWords[out] {
		return "key" + strings.ToUpper(out[:1]) + out[1:]
	}
	return out
}

// reservedWords is what a constant must not be called in Kotlin, Swift or
// TypeScript. One list, so a name avoided anywhere is avoided everywhere.
var reservedWords = map[string]bool{
	"as": true, "break": true, "case": true, "catch": true, "class": true,
	"const": true, "continue": true, "default": true, "do": true, "else": true,
	"enum": true, "extension": true, "false": true, "for": true, "fun": true,
	"guard": true, "if": true, "import": true, "in": true, "init": true,
	"interface": true, "internal": true, "is": true, "let": true, "new": true,
	"null": true, "nil": true, "object": true, "private": true, "protocol": true,
	"public": true, "return": true, "self": true, "static": true, "struct": true,
	"super": true, "switch": true, "this": true, "throw": true, "true": true,
	"try": true, "typealias": true, "typeof": true, "val": true, "var": true,
	"void": true, "when": true, "where": true, "while": true,
}
