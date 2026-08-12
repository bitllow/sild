package domain

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"maps"
	"slices"
	"strings"
	"time"

	"github.com/bitllow/sild/backend/internal/i18n"
	"github.com/bitllow/sild/backend/internal/id"
	"github.com/bitllow/sild/backend/internal/store"
	"github.com/bitllow/sild/backend/internal/store/models"
)

// A key's standing in one locale.
const (
	TranslationDefault     = "default"
	TranslationCustom      = "custom"
	TranslationNeedsReview = "needs_review"
)

// TranslationProjectView is a project's settings and where it stands.
type TranslationProjectView struct {
	Slug             string
	Name             string
	Platform         bool
	FallbackLocale   string
	AutoPublish      bool
	Locales          []string
	AvailableLocales []string
	CurrentVersion   *int
	// Keys is how many strings the project declares, and Completion how many of
	// them carry text per locale — what says whether a market is ready to launch.
	Keys       int
	Completion map[string]int
	Namespaces []string
}

// TranslationKeyView is one key as the editor sees it.
type TranslationKeyView struct {
	Key       string
	Namespace string
	Source    string
	Value     string
	State     string
	// PluralBase and PluralCategory are set on a category sibling, so the editor can
	// group a plural's forms and label each with the category it is.
	PluralBase     string
	PluralCategory string
	// Placeholders are the `{name}`s the text will have filled in, so a translator
	// knows what they are and where their language wants them.
	Placeholders []string
}

// TranslationManifest is locale → published version, and what a client polls.
type TranslationManifest struct {
	Project        string
	FallbackLocale string
	Locales        map[string]int
}

// TranslationReleaseView is one published version. ID positions the cursor; the
// wire view does not carry it.
type TranslationReleaseView struct {
	ID          string
	Version     int
	PublishedAt time.Time
	PublishedBy string
	// Locales is what this version carries, so a caller need not read it back.
	Locales []string
}

// What an imported row did, or would do.
const (
	ImportNew     = "new"
	ImportChanged = "changed"
	ImportSkipped = "skipped"
)

// TranslationImportRow is one row of an import report.
type TranslationImportRow struct {
	Key    string
	Status string
	Value  string
	// Reason says why a row was skipped — an unknown key, or text already there.
	Reason string
	// declared is whether the project already has this key, resolved once while the
	// report is built so the write does not resolve it again.
	declared bool
}

// TranslationImportReport is what an import did, or what a dry run would do.
type TranslationImportReport struct {
	Project  string
	Locale   string
	Format   string
	DryRun   bool
	Rows     []TranslationImportRow
	New      int
	Changed  int
	Skipped  int
	KeysMade int
}

// TranslationDraftRow is one key whose draft text differs from what is live.
type TranslationDraftRow struct {
	Locale string
	Key    string
	// Live is the text the current release carries, empty before anything is
	// published or when the key is new since.
	Live  string
	Draft string
}

// TranslationDraftDiff is what publishing would change — the release preview.
type TranslationDraftDiff struct {
	Project string
	// Version the diff is taken against, nil before a first publish.
	Version *int
	// NextVersion is the version a publish would cut.
	NextVersion int
	Rows        []TranslationDraftRow
}

func sourceHash(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

// TranslationProject returns the project's settings, defaulted for a tenant that
// has never changed them. The platform project exists in every tenant without a
// row; a tenant-owned one has to have been created.
func (s *Service) TranslationProject(ctx context.Context, tenantID, project string) (TranslationProjectView, error) {
	v, _, err := s.projectAndCatalog(ctx, tenantID, project)
	return v, err
}

// projectAndCatalog is the project view and the catalog behind it. Every caller
// that needs both takes them together: building the view reads the declared keys
// already, and reading them twice per request is the same query twice.
func (s *Service) projectAndCatalog(ctx context.Context, tenantID, project string) (TranslationProjectView, *i18n.Catalog, error) {
	p, err := s.store.Translations().GetProject(ctx, tenantID, project)
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		return TranslationProjectView{}, nil, err
	}
	if p == nil && project != i18n.PlatformProject {
		return TranslationProjectView{}, nil, ErrNotFound
	}
	return s.projectView(ctx, tenantID, project, p)
}

// ListTranslationProjects returns the platform project and every project the
// tenant created, platform first.
func (s *Service) ListTranslationProjects(ctx context.Context, tenantID string) ([]TranslationProjectView, error) {
	rows, err := s.store.Translations().ListProjects(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	byslug := make(map[string]*models.TranslationProject, len(rows))
	for i := range rows {
		byslug[rows[i].Slug] = &rows[i]
	}
	platform, _, err := s.projectView(ctx, tenantID, i18n.PlatformProject, byslug[i18n.PlatformProject])
	if err != nil {
		return nil, err
	}
	out := []TranslationProjectView{platform}
	for i := range rows {
		if rows[i].Slug == i18n.PlatformProject {
			continue
		}
		v, _, err := s.projectView(ctx, tenantID, rows[i].Slug, &rows[i])
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, nil
}

// projectView fills in what the settings row does not carry. p is nil for a
// platform project the tenant has never configured.
func (s *Service) projectView(ctx context.Context, tenantID, project string, p *models.TranslationProject) (TranslationProjectView, *i18n.Catalog, error) {
	platform := project == i18n.PlatformProject
	v := TranslationProjectView{
		Slug:             project,
		Platform:         platform,
		FallbackLocale:   i18n.SourceLocale,
		AvailableLocales: i18n.Platform().Locales(),
	}
	if platform {
		v.Name = "Sild"
		v.Locales = i18n.Platform().Locales()
	} else {
		v.Locales = []string{i18n.SourceLocale}
	}
	if p != nil {
		v.FallbackLocale = p.FallbackLocale
		v.AutoPublish = p.AutoPublish
		if p.Name != "" {
			v.Name = p.Name
		}
		locales, err := s.store.Translations().ProjectLocales(ctx, tenantID, project)
		if err != nil {
			return TranslationProjectView{}, nil, err
		}
		if len(locales) > 0 {
			v.Locales = locales
		}
	}

	rel, err := s.store.Translations().LatestRelease(ctx, tenantID, project)
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		return TranslationProjectView{}, nil, err
	}
	if rel != nil {
		version := rel.Version
		v.CurrentVersion = &version
	}

	cat, err := s.catalogFor(ctx, tenantID, project)
	if err != nil {
		return TranslationProjectView{}, nil, err
	}
	v.Keys = len(cat.Keys())
	v.Namespaces = cat.Namespaces()
	v.Completion, err = s.completion(ctx, tenantID, project, cat, v.Locales)
	if err != nil {
		return TranslationProjectView{}, nil, err
	}
	return v, cat, nil
}

// completion is the share of keys carrying text in each enabled locale. A locale
// Sild ships is complete by construction, so only what the tenant wrote is counted.
func (s *Service) completion(ctx context.Context, tenantID, project string, cat *i18n.Catalog, locales []string) (map[string]int, error) {
	shipped := cat.Locales()
	out := make(map[string]int, len(locales))
	// Counted only when some locale actually needs the figure: the platform project
	// as shipped never does, and every manifest poll resolves the project.
	var counts map[string]int
	for _, l := range locales {
		if slices.Contains(shipped, l) {
			out[l] = 100
			continue
		}
		// Per locale, not per source key: Latvian needs a zero form English never
		// declares, so counting against English would read 100% with one missing.
		total := len(cat.LocaleKeys(l))
		if total == 0 {
			out[l] = 100
			continue
		}
		if counts == nil {
			var err error
			if counts, err = s.store.Translations().TranslatedCounts(ctx, tenantID, project); err != nil {
				return nil, err
			}
			if counts == nil {
				counts = map[string]int{}
			}
		}
		out[l] = min(100, counts[l]*100/total)
	}
	return out, nil
}

// catalogFor is the project's declared strings: the repo's for the platform
// project, the tenant's key rows for one of its own. Everything downstream —
// resolution, staleness, bundles — is then the same code for both.
func (s *Service) catalogFor(ctx context.Context, tenantID, project string) (*i18n.Catalog, error) {
	if project == i18n.PlatformProject {
		return i18n.Platform(), nil
	}
	keys, err := s.store.Translations().Keys(ctx, tenantID, project)
	if err != nil {
		return nil, err
	}
	sources := make(map[string]string, len(keys))
	var plural []string
	for _, k := range keys {
		sources[k.Key] = k.Source
		if !k.Plural {
			continue
		}
		if base, _, ok := i18n.SplitPlural(k.Key); ok {
			plural = append(plural, base)
		}
	}
	return i18n.NewCatalog(sources, plural...), nil
}

// CreateTranslationProject declares a project for a tenant's own strings. It
// starts with the source language on and nothing declared.
func (s *Service) CreateTranslationProject(ctx context.Context, tenantID, slug, name string) (TranslationProjectView, error) {
	slug = strings.ToLower(strings.TrimSpace(slug))
	if !validProjectSlug(slug) {
		return TranslationProjectView{}, invalid("a project id is 2-64 characters of a-z, 0-9 and dashes")
	}
	// models.ScopeAll is reserved too: a project named "all" in a translator's
	// scope would read as every project.
	if slug == i18n.PlatformProject || slug == models.ScopeAll {
		return TranslationProjectView{}, invalid("that project id is reserved")
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return TranslationProjectView{}, invalid("name is required")
	}
	existing, err := s.store.Translations().GetProject(ctx, tenantID, slug)
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		return TranslationProjectView{}, err
	}
	if existing != nil {
		return TranslationProjectView{}, invalid("a project with that id already exists")
	}
	p := &models.TranslationProject{
		TenantID: tenantID, Slug: slug, Name: name,
		FallbackLocale: i18n.SourceLocale, UpdatedAt: s.now(),
	}
	if err := s.store.Translations().SaveProject(ctx, p, []string{i18n.SourceLocale}); err != nil {
		return TranslationProjectView{}, err
	}
	v, _, err := s.projectView(ctx, tenantID, slug, p)
	return v, err
}

// DeleteTranslationProject removes a tenant's project and everything in it. The
// platform project is not a tenant's to remove.
func (s *Service) DeleteTranslationProject(ctx context.Context, tenantID, project string) error {
	if project == i18n.PlatformProject {
		return invalid("the Sild project cannot be deleted")
	}
	if _, err := s.TranslationProject(ctx, tenantID, project); err != nil {
		return err
	}
	// The grants naming it go first: a scope names projects by id, so a failure
	// after the project was gone would hand a new project of the same name to
	// whoever held the old one. Narrowed grants over a project still standing is
	// the safe half of that pair.
	if err := s.revokeProjectGrants(ctx, tenantID, project); err != nil {
		return err
	}
	return s.store.Translations().DeleteProject(ctx, tenantID, project)
}

// revokeProjectGrants drops one project from every translator assignment and every
// scoped key. A grant that named only that project ends up naming nothing, which
// grants nothing — the Team screen shows it as such.
func (s *Service) revokeProjectGrants(ctx context.Context, tenantID, project string) error {
	rows, err := s.store.RoleAssignments().ListByTenant(ctx, tenantID)
	if err != nil {
		return err
	}
	for _, a := range rows {
		kept := without(a.Scope.Projects, project)
		if len(kept) == len(a.Scope.Projects) {
			continue
		}
		a.Scope.Projects = kept
		if err := s.store.RoleAssignments().Rescope(ctx, tenantID, a.AdminUserID, a.Role, a.Scope); err != nil {
			return err
		}
	}
	keys, err := s.store.APIKeys().ListByTenant(ctx, tenantID)
	if err != nil {
		return err
	}
	for _, k := range keys {
		kept := without(k.Scope.Projects, project)
		if len(kept) == len(k.Scope.Projects) {
			continue
		}
		k.Scope.Projects = kept
		if err := s.store.APIKeys().Rescope(ctx, tenantID, k.ID, k.Scope); err != nil {
			return err
		}
	}
	return nil
}

// without drops one member, leaving "all" alone: a grant over every project is not
// a grant that named this one.
func without(set []string, value string) []string {
	out := make([]string, 0, len(set))
	for _, s := range set {
		if s != value {
			out = append(out, s)
		}
	}
	return out
}

// validKey keeps a key addressable as a URL path segment — undeclaring one is a
// DELETE on it, and a key holding a slash could never be reached again.
func validKey(key string) bool {
	if key == "" || len(key) > 255 {
		return false
	}
	for _, r := range key {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		case r == '.' || r == '-' || r == '_':
		default:
			return false
		}
	}
	return true
}

// validProjectSlug keeps a project id usable in a URL path segment and in an
// SDK's generated accessors.
func validProjectSlug(slug string) bool {
	if len(slug) < 2 || len(slug) > 64 {
		return false
	}
	for _, r := range slug {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '-' {
			continue
		}
		return false
	}
	return true
}

// SaveTranslationProject replaces the project's settings and enabled locales.
// Locales are free-form BCP-47 tags, so a tenant may add a language Sild does
// not ship. The platform project's name is Sild's, so a name is ignored there.
func (s *Service) SaveTranslationProject(ctx context.Context, tenantID, project, name, fallback string, autoPublish bool, locales []string) error {
	cur, err := s.TranslationProject(ctx, tenantID, project)
	if err != nil {
		return err
	}
	fallback = i18n.Normalize(fallback)
	if fallback == "" {
		return invalid("fallback_locale is required")
	}
	clean := make([]string, 0, len(locales))
	for _, l := range locales {
		n := i18n.Normalize(l)
		if !i18n.ValidLanguage(n) || n == models.ScopeAll {
			return invalid("locale must be a language tag")
		}
		if !slices.Contains(clean, n) {
			clean = append(clean, n)
		}
	}
	if !slices.Contains(clean, fallback) {
		return invalid("the fallback language must be one of the enabled languages")
	}
	slices.Sort(clean)

	// The platform project's name is not the tenant's to set; its own is.
	if !cur.Platform {
		if n := strings.TrimSpace(name); n != "" {
			cur.Name = n
		}
	} else {
		cur.Name = ""
	}
	p := &models.TranslationProject{
		TenantID:       tenantID,
		Slug:           project,
		Name:           cur.Name,
		FallbackLocale: fallback,
		AutoPublish:    autoPublish,
		UpdatedAt:      s.now(),
	}
	return s.store.Translations().SaveProject(ctx, p, clean)
}

// TranslationKeys pages the project's keys as they stand in one locale.
func (s *Service) TranslationKeys(ctx context.Context, tenantID, project, locale, state, namespace, q string, pp store.PageParams) (store.Page[TranslationKeyView], error) {
	var empty store.Page[TranslationKeyView]
	proj, cat, err := s.projectAndCatalog(ctx, tenantID, project)
	if err != nil {
		return empty, err
	}
	locale = i18n.Normalize(locale)
	if locale == "" {
		locale = proj.FallbackLocale
	}

	overrides, err := s.store.Translations().LocaleOverrides(ctx, tenantID, project, locale)
	if err != nil {
		return empty, err
	}
	byKey := make(map[string]models.TranslationOverride, len(overrides))
	for _, o := range overrides {
		byKey[o.Key] = o
	}

	// The whole chain, so what the editor shows is what a bundle would publish.
	// The requested locale's rows are already in hand.
	ov, err := s.overridesFor(ctx, tenantID, project, proj.FallbackLocale, i18n.SourceLocale)
	if err != nil {
		return empty, err
	}
	ov[locale] = map[string]string{}
	for _, o := range overrides {
		ov[locale][o.Key] = o.Value
	}

	q = strings.ToLower(strings.TrimSpace(q))
	// The locale's own keys: a plural expanded into the categories this language
	// has, so Latvian is asked for zero and Estonian is not (docs/adr/0004).
	keys := cat.LocaleKeys(locale)
	views := make([]TranslationKeyView, 0, len(keys))
	for _, k := range keys {
		source := cat.Source(k)
		v := TranslationKeyView{
			Key:       k,
			Namespace: i18n.Namespace(k),
			Source:    source,
			Value:     cat.Resolve(ov, locale, proj.FallbackLocale, k),
			State:     TranslationDefault,
		}
		if o, ok := byKey[k]; ok {
			v.State = TranslationCustom
			if o.SourceHash != sourceHash(source) {
				v.State = TranslationNeedsReview
			}
		}
		if namespace != "" && v.Namespace != namespace {
			continue
		}
		if !matchesTranslationFilter(v, cat, locale, state, q) {
			continue
		}
		// Only for a row that survived the filters: most requests discard most keys.
		if base, category, ok := i18n.SplitPlural(k); ok && cat.IsPluralBase(base) {
			v.PluralBase, v.PluralCategory = base, category
		}
		v.Placeholders = i18n.Placeholders(source)
		views = append(views, v)
	}

	pp.Sort = store.SortID
	pp.Order = store.OrderAsc
	return store.SlicePage(views, pp, func(v *TranslationKeyView) string { return v.Key }), nil
}

func matchesTranslationFilter(v TranslationKeyView, cat *i18n.Catalog, locale, state, q string) bool {
	switch state {
	case TranslationCustom, TranslationNeedsReview:
		if v.State != state {
			return false
		}
	case "missing":
		// Nothing in this locale: no override, and Sild ships no text for it.
		if _, shipped := cat.Default(locale, v.Key); v.State != TranslationDefault || shipped {
			return false
		}
	}
	if q == "" {
		return true
	}
	return strings.Contains(strings.ToLower(v.Key), q) ||
		strings.Contains(strings.ToLower(v.Source), q) ||
		strings.Contains(strings.ToLower(v.Value), q)
}

// PutTranslationOverride writes a tenant's own text for one key.
func (s *Service) PutTranslationOverride(ctx context.Context, tenantID, project, locale, key, value string, mayPublish bool) error {
	proj, cat, err := s.projectAndCatalog(ctx, tenantID, project)
	if err != nil {
		return err
	}
	locale = i18n.Normalize(locale)
	if !slices.Contains(proj.Locales, locale) {
		return invalid("that language is not enabled for this project")
	}
	// DeclaredFor, not Declared: a language's own plural category is writable even
	// where English declares no such sibling.
	if !cat.DeclaredFor(locale, key) {
		return invalid("unknown key")
	}
	if i18n.Reserved(key) {
		return invalid("that string is Sild's own; turn the attribution off in Appearance instead")
	}
	if strings.TrimSpace(value) == "" {
		return invalid("value is required")
	}
	o := &models.TranslationOverride{
		TenantID: tenantID, Project: project, Locale: locale, Key: key,
		Value: value, SourceHash: sourceHash(cat.Source(key)), UpdatedAt: s.now(),
	}
	if err := s.store.Translations().PutOverride(ctx, o); err != nil {
		return err
	}
	if !mayPublish {
		return nil
	}
	return s.autoPublish(ctx, tenantID, proj, "")
}

// DeleteTranslationOverride resets a key to the text Sild ships.
func (s *Service) DeleteTranslationOverride(ctx context.Context, tenantID, project, locale, key string, mayPublish bool) error {
	proj, err := s.TranslationProject(ctx, tenantID, project)
	if err != nil {
		return err
	}
	locale = i18n.Normalize(locale)
	if err := s.store.Translations().DeleteOverride(ctx, tenantID, project, locale, key); err != nil {
		return err
	}
	if !mayPublish {
		return nil
	}
	return s.autoPublish(ctx, tenantID, proj, "")
}

// DeclareTranslationKey adds a key, or rewords the source of one already there —
// which is what flags its translations for review. A plural declaration carries
// one source per source-language category and lands as that many siblings.
func (s *Service) DeclareTranslationKey(ctx context.Context, tenantID, project, key, source string, plurals map[string]string) error {
	proj, _, err := s.ownProject(ctx, tenantID, project)
	if err != nil {
		return err
	}
	key = strings.TrimSpace(key)
	if !validKey(key) {
		return invalid("a key is up to 255 characters of letters, digits, dots, dashes and underscores")
	}
	rows, err := declaredRows(tenantID, project, key, source, plurals, s.now())
	if err != nil {
		return err
	}
	// A key that was ordinary and is now a plural — or the reverse — must not leave
	// the other shape behind: the editor would list both, and a bundle would carry
	// both.
	if err := s.removeShapesBut(ctx, tenantID, project, key, rows); err != nil {
		return err
	}
	for _, k := range rows {
		if err := s.store.Translations().PutKey(ctx, &k); err != nil {
			return err
		}
	}
	return s.autoPublish(ctx, tenantID, proj, "")
}

// removeShapesBut drops the rows this declaration replaces: the bare key when it
// becomes a plural, and every category sibling when it stops being one.
func (s *Service) removeShapesBut(ctx context.Context, tenantID, project, key string, rows []models.TranslationKey) error {
	keeping := make(map[string]bool, len(rows))
	for _, k := range rows {
		keeping[k.Key] = true
	}
	gone := []string{key}
	for _, c := range i18n.EveryCategory() {
		gone = append(gone, i18n.PluralKey(key, c))
	}
	for _, k := range gone {
		if keeping[k] {
			continue
		}
		if err := s.store.Translations().DeleteKey(ctx, tenantID, project, k); err != nil {
			return err
		}
	}
	return nil
}

// declaredRows is the key rows one declaration writes: one for an ordinary key,
// one per category for a plural.
func declaredRows(tenantID, project, key, source string, plurals map[string]string, now time.Time) ([]models.TranslationKey, error) {
	if len(plurals) == 0 {
		if source = strings.TrimSpace(source); source == "" {
			return nil, invalid("source is required")
		}
		return []models.TranslationKey{{
			TenantID: tenantID, Project: project, Key: key, Source: source, UpdatedAt: now,
		}}, nil
	}
	// The source language's categories and no others: a form English cannot select
	// has no English text to declare, and every other language's forms are written
	// as translations.
	want := i18n.Categories(i18n.SourceLocale)
	rows := make([]models.TranslationKey, 0, len(want))
	for _, cat := range want {
		text := strings.TrimSpace(plurals[cat])
		if text == "" {
			return nil, invalid("a plural key needs a source for " + strings.Join(want, " and "))
		}
		rows = append(rows, models.TranslationKey{
			TenantID: tenantID, Project: project, Key: i18n.PluralKey(key, cat),
			Source: text, Plural: true, UpdatedAt: now,
		})
	}
	for cat := range plurals {
		if !slices.Contains(want, cat) {
			return nil, invalid("the source language has no " + cat + " form")
		}
	}
	return rows, nil
}

// UndeclareTranslationKey removes a key and every translation of it. A plural is
// one key to a tenant, however many category rows it takes.
func (s *Service) UndeclareTranslationKey(ctx context.Context, tenantID, project, key string) error {
	proj, cat, err := s.ownProject(ctx, tenantID, project)
	if err != nil {
		return err
	}
	// A sibling names the whole plural: the editor shows one key per base, and
	// removing "cart.items.one" and leaving the rest is not a state a tenant asked
	// for.
	if base, _, ok := i18n.SplitPlural(key); ok && cat.IsPluralBase(base) {
		key = base
	}
	gone := []string{key}
	if cat.IsPluralBase(key) {
		// Every category, not the source language's: a Latvian zero form is a row of
		// its own, and leaving it behind would resurrect it if the key came back.
		gone = nil
		for _, c := range i18n.EveryCategory() {
			gone = append(gone, i18n.PluralKey(key, c))
		}
	}
	for _, k := range gone {
		if err := s.store.Translations().DeleteKey(ctx, tenantID, project, k); err != nil {
			return err
		}
	}
	return s.autoPublish(ctx, tenantID, proj, "")
}

// ownProject loads a project the tenant declares the keys of. The platform
// project's are the repo's (docs/adr/0003), so it is never one of these.
func (s *Service) ownProject(ctx context.Context, tenantID, project string) (TranslationProjectView, *i18n.Catalog, error) {
	proj, cat, err := s.projectAndCatalog(ctx, tenantID, project)
	if err != nil {
		return proj, nil, err
	}
	if proj.Platform {
		return proj, nil, invalid("Sild's own keys are declared in its releases, not here")
	}
	return proj, cat, nil
}

func (s *Service) autoPublish(ctx context.Context, tenantID string, proj TranslationProjectView, by string) error {
	if !proj.AutoPublish {
		return nil
	}
	_, err := s.publish(ctx, tenantID, proj, by)
	return err
}

// ImportTranslations reads a file into one locale's drafts. A dry run reports what
// would happen and writes nothing — the same path either way, so a preview cannot
// disagree with the write.
func (s *Service) ImportTranslations(
	ctx context.Context, tenantID, project, locale string, format i18n.Format,
	raw []byte, dryRun, createKeys, mayPublish bool,
) (TranslationImportReport, error) {
	empty := TranslationImportReport{}
	if !i18n.KnownFormat(format) {
		return empty, invalid("unsupported format " + string(format))
	}
	proj, cat, err := s.projectAndCatalog(ctx, tenantID, project)
	if err != nil {
		return empty, err
	}
	locale = i18n.Normalize(locale)
	if !slices.Contains(proj.Locales, locale) {
		return empty, invalid("that language is not enabled for this project")
	}
	if createKeys && proj.Platform {
		return empty, invalid("Sild's own keys are declared in its releases, not here")
	}
	// A key carries the English it was authored in, so a Latvian file would record
	// Latvian as its source and flag every translation against the wrong text.
	if createKeys && locale != i18n.SourceLocale {
		return empty, invalid("new keys can only be created from a " + i18n.SourceLocale + " file")
	}
	incoming, err := i18n.Parse(format, raw)
	if err != nil {
		return empty, invalid(err.Error())
	}
	// What this locale renders today, which is what a row has to differ from to be
	// worth writing: a row that merely restates the shipped text is skipped rather
	// than frozen into an override, so re-importing an export changes nothing.
	ov, err := s.overridesFor(ctx, tenantID, project, locale, proj.FallbackLocale, i18n.SourceLocale)
	if err != nil {
		return empty, err
	}
	held := ov[locale]

	report := TranslationImportReport{
		Project: project, Locale: locale, Format: string(format), DryRun: dryRun,
	}
	matcher := i18n.NewKeyMatcher(format, cat.LocaleKeys(locale))
	for _, name := range slices.Sorted(maps.Keys(incoming)) {
		// The text is what the file said: a translation may legitimately end in a
		// space, and rewriting it would make the round trip lossy.
		value := incoming[name]
		key, declared := matcher.Resolve(name)
		row := TranslationImportRow{Key: key, Value: value, declared: declared}
		switch {
		case strings.TrimSpace(value) == "":
			row.Key, row.Status, row.Reason = name, ImportSkipped, "no text"
		case !declared && !createKeys:
			row.Key, row.Status, row.Reason = name, ImportSkipped, "unknown key"
		case !declared:
			row.Key, row.Status = name, ImportNew
			report.KeysMade++
		case cat.Resolve(ov, locale, proj.FallbackLocale, key) == value:
			row.Status, row.Reason = ImportSkipped, "unchanged"
		case held[key] == "":
			row.Status = ImportNew
		default:
			row.Status = ImportChanged
		}
		switch row.Status {
		case ImportNew:
			report.New++
		case ImportChanged:
			report.Changed++
		default:
			report.Skipped++
		}
		report.Rows = append(report.Rows, row)
	}

	// Checked before any of it is written, so a dry run refuses exactly what the
	// write would refuse — and a file with one bad key late in it does not leave the
	// earlier ones applied.
	if err := checkImportPlan(report.Rows); err != nil {
		return empty, err
	}
	if dryRun {
		return report, nil
	}
	if err := s.applyImport(ctx, tenantID, project, locale, report.Rows, cat); err != nil {
		return empty, err
	}
	if !mayPublish {
		return report, nil
	}
	return report, s.autoPublish(ctx, tenantID, proj, "")
}

// checkImportPlan refuses a file that would create a key no route could address.
// The same rule a declaration goes through, applied before anything is written.
func checkImportPlan(rows []TranslationImportRow) error {
	for _, r := range rows {
		if r.Status == ImportSkipped || r.declared {
			continue
		}
		if !validKey(r.Key) {
			return invalid("cannot create the key " + r.Key +
				": a key is up to 255 characters of letters, digits, dots, dashes and underscores")
		}
	}
	return nil
}

// applyImport writes the rows a report says would change, declaring any key the
// project does not have yet. The source of a key born this way is the text
// itself: there is nothing else to write, and the tenant can reword it after.
func (s *Service) applyImport(
	ctx context.Context, tenantID, project, locale string,
	rows []TranslationImportRow, cat *i18n.Catalog,
) error {
	// A key the file spells like a category sibling is only a plural if the file
	// carries more than one form of it; one key merely ending in "other" is not.
	written := make([]string, 0, len(rows))
	for _, r := range rows {
		if r.Status != ImportSkipped {
			written = append(written, r.Key)
		}
	}
	plural := i18n.PluralBases(written)
	for _, r := range rows {
		if r.Status == ImportSkipped {
			continue
		}
		key := r.Key
		// A key born here is written against its own text, so it is current rather
		// than stale the moment it lands.
		source := cat.Source(key)
		if !r.declared {
			source = r.Value
			base, _, isSibling := i18n.SplitPlural(key)
			k := &models.TranslationKey{
				TenantID: tenantID, Project: project, Key: key,
				Source: source, Plural: isSibling && plural[base], UpdatedAt: s.now(),
			}
			if err := s.store.Translations().PutKey(ctx, k); err != nil {
				return err
			}
		}
		o := &models.TranslationOverride{
			TenantID: tenantID, Project: project, Locale: locale, Key: key,
			Value: r.Value, SourceHash: sourceHash(source), UpdatedAt: s.now(),
		}
		if err := s.store.Translations().PutOverride(ctx, o); err != nil {
			return err
		}
	}
	return nil
}

// ExportTranslations writes one locale out: the whole resolved set, so a
// translator or a build gets every key rather than the ones somebody touched.
func (s *Service) ExportTranslations(
	ctx context.Context, tenantID, project, locale string, format i18n.Format,
) ([]byte, error) {
	if !i18n.RenderableFormat(format) {
		return nil, invalid("unsupported format " + string(format))
	}
	proj, cat, err := s.projectAndCatalog(ctx, tenantID, project)
	if err != nil {
		return nil, err
	}
	locale = i18n.Normalize(locale)
	if !slices.Contains(proj.Locales, locale) {
		return nil, invalid("that language is not enabled for this project")
	}
	ov, err := s.overridesFor(ctx, tenantID, project, locale, proj.FallbackLocale, i18n.SourceLocale)
	if err != nil {
		return nil, err
	}
	keys := cat.LocaleKeys(locale)
	rows := make([]i18n.Row, 0, len(keys))
	for _, k := range keys {
		base, _, sibling := i18n.SplitPlural(k)
		rows = append(rows, i18n.Row{
			Key:    k,
			Source: cat.Source(k),
			Value:  cat.Resolve(ov, locale, proj.FallbackLocale, k),
			Plural: sibling && cat.IsPluralBase(base),
		})
	}
	out, err := i18n.Render(format, locale, rows)
	if err != nil {
		return nil, invalid(err.Error())
	}
	return out, nil
}

// TranslationDraftDiff is what publishing would change. Before a first release
// everything drafted is a change, because nothing is live.
// locales, when given, narrows the diff to the languages the caller holds: a
// translator scoped to one of ten must not cost ten bundle reads and ten
// resolutions to be shown one.
func (s *Service) TranslationDraftDiff(ctx context.Context, tenantID, project string, locales []string) (TranslationDraftDiff, error) {
	proj, cat, err := s.projectAndCatalog(ctx, tenantID, project)
	if err != nil {
		return TranslationDraftDiff{}, err
	}
	overrides, err := s.store.Translations().Overrides(ctx, tenantID, project)
	if err != nil {
		return TranslationDraftDiff{}, err
	}
	ov := i18n.Overrides{}
	for _, o := range overrides {
		if ov[o.Locale] == nil {
			ov[o.Locale] = map[string]string{}
		}
		ov[o.Locale][o.Key] = o.Value
	}
	// What the current release serves, in one read rather than one per language.
	live := map[string]map[string]string{}
	if proj.CurrentVersion != nil {
		bundles, err := s.store.Translations().ReleaseBundles(ctx, tenantID, project, *proj.CurrentVersion)
		if err != nil {
			return TranslationDraftDiff{}, err
		}
		for _, b := range bundles {
			var strs map[string]string
			if err := json.Unmarshal(b.Strings, &strs); err != nil {
				return TranslationDraftDiff{}, err
			}
			live[b.Locale] = strs
		}
	}

	diff := TranslationDraftDiff{
		Project: project, Version: proj.CurrentVersion,
		NextVersion: nextVersion(proj.CurrentVersion),
	}
	for _, locale := range proj.Locales {
		if len(locales) > 0 && !slices.Contains(locales, locale) {
			continue
		}
		live := live[locale]
		// The draft bundle, built exactly as publish would build it, so the preview
		// is the release rather than a guess at it.
		draft := cat.Bundle(ov, locale, proj.FallbackLocale)
		for key, text := range draft {
			if live[key] == text {
				continue
			}
			diff.Rows = append(diff.Rows, TranslationDraftRow{
				Locale: locale, Key: key, Live: live[key], Draft: text,
			})
		}
		// A key undeclared since the last release leaves the next one, and an
		// approval that did not mention the removal would have been approving blind.
		for key, text := range live {
			if _, kept := draft[key]; !kept {
				diff.Rows = append(diff.Rows, TranslationDraftRow{
					Locale: locale, Key: key, Live: text,
				})
			}
		}
	}
	slices.SortFunc(diff.Rows, func(a, b TranslationDraftRow) int {
		if a.Locale != b.Locale {
			return strings.Compare(a.Locale, b.Locale)
		}
		return strings.Compare(a.Key, b.Key)
	})
	return diff, nil
}

// PublishTranslations cuts a release: every enabled locale is materialized at
// this version so its content can never change afterwards.
func (s *Service) PublishTranslations(ctx context.Context, tenantID, project, by string) (TranslationReleaseView, error) {
	proj, err := s.TranslationProject(ctx, tenantID, project)
	if err != nil {
		return TranslationReleaseView{}, err
	}
	return s.publish(ctx, tenantID, proj, by)
}

// publish takes the project already loaded, so a caller that has one does not
// read it back.
func (s *Service) publish(ctx context.Context, tenantID string, proj TranslationProjectView, by string) (TranslationReleaseView, error) {
	project := proj.Slug
	overrides, err := s.store.Translations().Overrides(ctx, tenantID, project)
	if err != nil {
		return TranslationReleaseView{}, err
	}
	ov := i18n.Overrides{}
	for _, o := range overrides {
		if ov[o.Locale] == nil {
			ov[o.Locale] = map[string]string{}
		}
		ov[o.Locale][o.Key] = o.Value
	}

	cat, err := s.catalogFor(ctx, tenantID, project)
	if err != nil {
		return TranslationReleaseView{}, err
	}
	version := nextVersion(proj.CurrentVersion)
	bundles := make([]models.TranslationBundle, 0, len(proj.Locales))
	for _, locale := range proj.Locales {
		raw, err := json.Marshal(cat.Bundle(ov, locale, proj.FallbackLocale))
		if err != nil {
			return TranslationReleaseView{}, err
		}
		bundles = append(bundles, models.TranslationBundle{
			TenantID: tenantID, Project: project, Locale: locale, Version: version, Strings: raw,
		})
	}
	return s.commitRelease(ctx, tenantID, project, version, by, bundles)
}

// RollbackTranslations cuts a new version carrying an earlier one's content.
// Releases are immutable, so going back is going forward to the same bytes.
func (s *Service) RollbackTranslations(ctx context.Context, tenantID, project string, to int, by string) (TranslationReleaseView, error) {
	old, err := s.store.Translations().ReleaseBundles(ctx, tenantID, project, to)
	if err != nil {
		return TranslationReleaseView{}, err
	}
	if len(old) == 0 {
		return TranslationReleaseView{}, ErrNotFound
	}
	latest, err := s.store.Translations().LatestRelease(ctx, tenantID, project)
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		return TranslationReleaseView{}, err
	}
	var current *int
	if latest != nil {
		current = &latest.Version
	}
	bundles := make([]models.TranslationBundle, 0, len(old))
	version := nextVersion(current)
	for _, b := range old {
		b.Version = version
		bundles = append(bundles, b)
	}
	return s.commitRelease(ctx, tenantID, project, version, by, bundles)
}

// nextVersion follows the project's current version; releases start at 1.
func nextVersion(current *int) int {
	if current == nil {
		return 1
	}
	return *current + 1
}

func (s *Service) commitRelease(ctx context.Context, tenantID, project string, version int, by string, bundles []models.TranslationBundle) (TranslationReleaseView, error) {
	rel := &models.TranslationRelease{
		ID: id.New(id.Release), TenantID: tenantID, Project: project,
		Version: version, PublishedBy: by, CreatedAt: s.now(),
	}
	if err := s.store.Translations().CreateRelease(ctx, rel, bundles); err != nil {
		return TranslationReleaseView{}, err
	}
	locales := make([]string, 0, len(bundles))
	for _, b := range bundles {
		locales = append(locales, b.Locale)
	}
	return TranslationReleaseView{
		ID: rel.ID, Version: rel.Version, PublishedAt: rel.CreatedAt,
		PublishedBy: rel.PublishedBy, Locales: locales,
	}, nil
}

// TranslationReleases lists published versions, newest first.
func (s *Service) TranslationReleases(ctx context.Context, tenantID, project string, pp store.PageParams) (store.Page[TranslationReleaseView], error) {
	rs, err := s.store.Translations().ListReleases(ctx, tenantID, project)
	if err != nil {
		return store.Page[TranslationReleaseView]{}, err
	}
	views := make([]TranslationReleaseView, 0, len(rs))
	for _, r := range rs {
		views = append(views, TranslationReleaseView{
			ID: r.ID, Version: r.Version, PublishedAt: r.CreatedAt, PublishedBy: r.PublishedBy,
		})
	}
	pp.Sort = store.SortID
	pp.Order = store.OrderDesc
	return store.SlicePage(views, pp, func(v *TranslationReleaseView) string { return v.ID }), nil
}

// TranslationManifest reports the published version per locale. Empty until a
// tenant publishes; a client's bundled defaults carry the same text.
func (s *Service) TranslationManifest(ctx context.Context, tenantID, project string) (TranslationManifest, error) {
	proj, err := s.TranslationProject(ctx, tenantID, project)
	if err != nil {
		return TranslationManifest{}, err
	}
	m := TranslationManifest{
		Project:        project,
		FallbackLocale: proj.FallbackLocale,
		Locales:        map[string]int{},
	}
	if proj.CurrentVersion == nil {
		return m, nil
	}
	locales, err := s.store.Translations().ReleaseLocales(ctx, tenantID, project, *proj.CurrentVersion)
	if err != nil {
		return TranslationManifest{}, err
	}
	for _, l := range locales {
		m.Locales[l] = *proj.CurrentVersion
	}
	return m, nil
}

// TranslationBundle returns one published locale verbatim.
func (s *Service) TranslationBundle(ctx context.Context, tenantID, project, locale string, version int) (map[string]string, error) {
	b, err := s.store.Translations().Bundle(ctx, tenantID, project, i18n.Normalize(locale), version)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	var strs map[string]string
	if err := json.Unmarshal(b.Strings, &strs); err != nil {
		return nil, err
	}
	return strs, nil
}

// SetContactLocale records the language a person reads.
func (s *Service) SetContactLocale(ctx context.Context, tenantID, externalUserID, locale string) error {
	n := i18n.Normalize(locale)
	if n == "" {
		return invalid("locale must be a language tag")
	}
	return s.store.Contacts().SetLocale(ctx, tenantID, externalUserID, n)
}

// ResolveText returns Sild's own text for one key at a recipient's locale,
// applying the tenant's overrides. Used where the server composes what a person
// reads — a nudge, an outbound mail — because no client is there to do it.
func (s *Service) ResolveText(ctx context.Context, tenantID, locale, key string) string {
	cat := i18n.Platform()
	proj, err := s.TranslationProject(ctx, tenantID, i18n.PlatformProject)
	fallback := i18n.SourceLocale
	if err == nil {
		fallback = proj.FallbackLocale
	}
	locale = i18n.Normalize(locale)
	if locale == "" {
		locale = fallback
	}
	// The published release, never the drafts: an unpublished edit is not live text.
	if err == nil && proj.CurrentVersion != nil {
		for _, l := range []string{locale, fallback, i18n.SourceLocale} {
			strs, err := s.TranslationBundle(ctx, tenantID, i18n.PlatformProject, l, *proj.CurrentVersion)
			if err != nil {
				continue
			}
			if v := strs[key]; v != "" {
				return v
			}
		}
	}
	return cat.Resolve(nil, locale, fallback, key)
}

// overridesFor loads the tenant's own text for the locales a resolution may
// touch, skipping repeats.
func (s *Service) overridesFor(ctx context.Context, tenantID, project string, locales ...string) (i18n.Overrides, error) {
	ov := i18n.Overrides{}
	for _, l := range locales {
		if l == "" || ov[l] != nil {
			continue
		}
		rows, err := s.store.Translations().LocaleOverrides(ctx, tenantID, project, l)
		if err != nil {
			return nil, err
		}
		ov[l] = map[string]string{}
		for _, o := range rows {
			ov[l][o.Key] = o.Value
		}
	}
	return ov, nil
}
