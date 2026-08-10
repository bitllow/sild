package domain

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"time"

	"github.com/bitllow/sild/backend/internal/i18n"
	"github.com/bitllow/sild/backend/internal/id"
	"github.com/bitllow/sild/backend/internal/policy"
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

// TranslatorGrantView is one translator's scope.
type TranslatorGrantView struct {
	AdminUserID string
	Email       string
	Name        string
	Projects    []string
	Locales     []string
}

func sourceHash(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

// TranslationProject returns the project's settings, defaulted for a tenant that
// has never changed them. The platform project exists in every tenant without a
// row; a tenant-owned one has to have been created.
func (s *Service) TranslationProject(ctx context.Context, tenantID, project string) (TranslationProjectView, error) {
	p, err := s.store.Translations().GetProject(ctx, tenantID, project)
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		return TranslationProjectView{}, err
	}
	if p == nil && project != i18n.PlatformProject {
		return TranslationProjectView{}, ErrNotFound
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
	platform, err := s.projectView(ctx, tenantID, i18n.PlatformProject, byslug[i18n.PlatformProject])
	if err != nil {
		return nil, err
	}
	out := []TranslationProjectView{platform}
	for i := range rows {
		if rows[i].Slug == i18n.PlatformProject {
			continue
		}
		v, err := s.projectView(ctx, tenantID, rows[i].Slug, &rows[i])
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, nil
}

// projectView fills in what the settings row does not carry. p is nil for a
// platform project the tenant has never configured.
func (s *Service) projectView(ctx context.Context, tenantID, project string, p *models.TranslationProject) (TranslationProjectView, error) {
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
			return TranslationProjectView{}, err
		}
		if len(locales) > 0 {
			v.Locales = locales
		}
	}

	rel, err := s.store.Translations().LatestRelease(ctx, tenantID, project)
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		return TranslationProjectView{}, err
	}
	if rel != nil {
		version := rel.Version
		v.CurrentVersion = &version
	}

	cat, err := s.catalogFor(ctx, tenantID, project)
	if err != nil {
		return TranslationProjectView{}, err
	}
	v.Keys = len(cat.Keys())
	v.Namespaces = cat.Namespaces()
	v.Completion, err = s.completion(ctx, tenantID, project, cat, v.Locales)
	if err != nil {
		return TranslationProjectView{}, err
	}
	return v, nil
}

// completion is the share of keys carrying text in each enabled locale. A locale
// Sild ships is complete by construction, so only what the tenant wrote is counted.
func (s *Service) completion(ctx context.Context, tenantID, project string, cat *i18n.Catalog, locales []string) (map[string]int, error) {
	total := len(cat.Keys())
	shipped := cat.Locales()
	out := make(map[string]int, len(locales))
	// Counted only when some locale actually needs the figure: the platform project
	// as shipped never does, and every manifest poll resolves the project.
	var counts map[string]int
	for _, l := range locales {
		if total == 0 || slices.Contains(shipped, l) {
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
	for _, k := range keys {
		sources[k.Key] = k.Source
	}
	return i18n.NewCatalog(sources), nil
}

// CreateTranslationProject declares a project for a tenant's own strings. It
// starts with the source language on and nothing declared.
func (s *Service) CreateTranslationProject(ctx context.Context, tenantID, slug, name string) (TranslationProjectView, error) {
	slug = strings.ToLower(strings.TrimSpace(slug))
	if !validProjectSlug(slug) {
		return TranslationProjectView{}, invalid("a project id is 2-64 characters of a-z, 0-9 and dashes")
	}
	if slug == i18n.PlatformProject {
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
	return s.projectView(ctx, tenantID, slug, p)
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
	return s.store.Translations().DeleteProject(ctx, tenantID, project)
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
		if n == "" {
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
	proj, err := s.TranslationProject(ctx, tenantID, project)
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

	cat, err := s.catalogFor(ctx, tenantID, project)
	if err != nil {
		return empty, err
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
	keys := cat.Keys()
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
func (s *Service) PutTranslationOverride(ctx context.Context, tenantID, project, locale, key, value string) error {
	proj, err := s.TranslationProject(ctx, tenantID, project)
	if err != nil {
		return err
	}
	locale = i18n.Normalize(locale)
	if !slices.Contains(proj.Locales, locale) {
		return invalid("that language is not enabled for this project")
	}
	cat, err := s.catalogFor(ctx, tenantID, project)
	if err != nil {
		return err
	}
	if !cat.Declared(key) {
		return invalid("unknown key")
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
	return s.autoPublish(ctx, tenantID, proj, "")
}

// DeleteTranslationOverride resets a key to the text Sild ships.
func (s *Service) DeleteTranslationOverride(ctx context.Context, tenantID, project, locale, key string) error {
	proj, err := s.TranslationProject(ctx, tenantID, project)
	if err != nil {
		return err
	}
	locale = i18n.Normalize(locale)
	if err := s.store.Translations().DeleteOverride(ctx, tenantID, project, locale, key); err != nil {
		return err
	}
	return s.autoPublish(ctx, tenantID, proj, "")
}

// DeclareTranslationKey adds a key to a tenant's own project, or rewords the
// source of one already there — which is what flags its translations for review.
func (s *Service) DeclareTranslationKey(ctx context.Context, tenantID, project, key, source string) error {
	proj, err := s.ownProject(ctx, tenantID, project)
	if err != nil {
		return err
	}
	key = strings.TrimSpace(key)
	if !validKey(key) {
		return invalid("a key is up to 255 characters of letters, digits, dots, dashes and underscores")
	}
	source = strings.TrimSpace(source)
	if source == "" {
		return invalid("source is required")
	}
	k := &models.TranslationKey{
		TenantID: tenantID, Project: project, Key: key,
		Source: source, UpdatedAt: s.now(),
	}
	if err := s.store.Translations().PutKey(ctx, k); err != nil {
		return err
	}
	return s.autoPublish(ctx, tenantID, proj, "")
}

// UndeclareTranslationKey removes a key and every translation of it.
func (s *Service) UndeclareTranslationKey(ctx context.Context, tenantID, project, key string) error {
	proj, err := s.ownProject(ctx, tenantID, project)
	if err != nil {
		return err
	}
	if err := s.store.Translations().DeleteKey(ctx, tenantID, project, key); err != nil {
		return err
	}
	return s.autoPublish(ctx, tenantID, proj, "")
}

// ownProject loads a project the tenant declares the keys of. The platform
// project's are the repo's (docs/adr/0003), so it is never one of these.
func (s *Service) ownProject(ctx context.Context, tenantID, project string) (TranslationProjectView, error) {
	proj, err := s.TranslationProject(ctx, tenantID, project)
	if err != nil {
		return proj, err
	}
	if proj.Platform {
		return proj, invalid("Sild's own keys are declared in its releases, not here")
	}
	return proj, nil
}

func (s *Service) autoPublish(ctx context.Context, tenantID string, proj TranslationProjectView, by string) error {
	if !proj.AutoPublish {
		return nil
	}
	_, err := s.publish(ctx, tenantID, proj, by)
	return err
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

// TranslationScope is the caller's own grant, for policy to narrow writes by.
func (s *Service) TranslationScope(ctx context.Context, tenantID, adminUserID string) (policy.TranslationAttrs, error) {
	rows, err := s.store.Translations().Scopes(ctx, tenantID, adminUserID)
	if err != nil {
		return policy.TranslationAttrs{}, err
	}
	var at policy.TranslationAttrs
	for _, r := range rows {
		switch r.Kind {
		case models.ScopeProject:
			at.Projects = append(at.Projects, r.Value)
		case models.ScopeLocale:
			at.Locales = append(at.Locales, r.Value)
		}
	}
	return at, nil
}

// TranslatorGrants lists every translator and what they may touch.
func (s *Service) TranslatorGrants(ctx context.Context, tenantID string) ([]TranslatorGrantView, error) {
	admins, err := s.store.Admins().List(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	rows, err := s.store.Translations().AllScopes(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	byAdmin := map[string]*TranslatorGrantView{}
	out := []*TranslatorGrantView{}
	for _, a := range admins {
		if a.PlatformRole != models.PlatformTranslator {
			continue
		}
		g := &TranslatorGrantView{AdminUserID: a.ID, Email: a.Email, Name: a.DisplayName()}
		byAdmin[a.ID] = g
		out = append(out, g)
	}
	for _, r := range rows {
		g, ok := byAdmin[r.AdminUserID]
		if !ok {
			continue
		}
		switch r.Kind {
		case models.ScopeProject:
			g.Projects = append(g.Projects, r.Value)
		case models.ScopeLocale:
			g.Locales = append(g.Locales, r.Value)
		}
	}
	views := make([]TranslatorGrantView, 0, len(out))
	for _, g := range out {
		views = append(views, *g)
	}
	return views, nil
}

// SetTranslatorScopes replaces one translator's grant. Empty sets mean every
// project or every language.
func (s *Service) SetTranslatorScopes(ctx context.Context, tenantID, adminUserID string, projects, locales []string) error {
	a, err := s.store.Admins().Get(ctx, tenantID, adminUserID)
	if err != nil {
		return ErrNotFound
	}
	if a.PlatformRole != models.PlatformTranslator {
		return invalid("that operator is not a translator")
	}
	rows := []models.TranslatorScope{}
	for _, p := range projects {
		if p == "" {
			continue
		}
		rows = append(rows, models.TranslatorScope{TenantID: tenantID, AdminUserID: adminUserID, Kind: models.ScopeProject, Value: p})
	}
	for _, l := range locales {
		n := i18n.Normalize(l)
		if n == "" {
			continue
		}
		rows = append(rows, models.TranslatorScope{TenantID: tenantID, AdminUserID: adminUserID, Kind: models.ScopeLocale, Value: n})
	}
	return s.store.Translations().SetScopes(ctx, tenantID, adminUserID, rows)
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
