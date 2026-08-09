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
}

// TranslationKeyView is one key as the editor sees it.
type TranslationKeyView struct {
	Key    string
	Source string
	Value  string
	State  string
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
// has never changed them.
func (s *Service) TranslationProject(ctx context.Context, tenantID, project string) (TranslationProjectView, error) {
	if project != i18n.PlatformProject {
		return TranslationProjectView{}, ErrNotFound
	}
	shipped := i18n.Platform().Locales()
	v := TranslationProjectView{
		Slug:             i18n.PlatformProject,
		Name:             "Sild",
		Platform:         true,
		FallbackLocale:   i18n.SourceLocale,
		Locales:          shipped,
		AvailableLocales: shipped,
	}

	p, err := s.store.Translations().GetProject(ctx, tenantID, project)
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		return TranslationProjectView{}, err
	}
	if p != nil {
		v.FallbackLocale = p.FallbackLocale
		v.AutoPublish = p.AutoPublish
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
	return v, nil
}

// SaveTranslationProject replaces the project's settings and enabled locales.
// Locales are free-form BCP-47 tags, so a tenant may add a language Sild does
// not ship.
func (s *Service) SaveTranslationProject(ctx context.Context, tenantID, project, fallback string, autoPublish bool, locales []string) error {
	if project != i18n.PlatformProject {
		return ErrNotFound
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

	p := &models.TranslationProject{
		TenantID:       tenantID,
		Slug:           project,
		FallbackLocale: fallback,
		AutoPublish:    autoPublish,
		UpdatedAt:      s.now(),
	}
	return s.store.Translations().SaveProject(ctx, p, clean)
}

// TranslationKeys pages the project's keys as they stand in one locale.
func (s *Service) TranslationKeys(ctx context.Context, tenantID, project, locale, state, q string, pp store.PageParams) (store.Page[TranslationKeyView], error) {
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

	cat := i18n.Platform()
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
			Key:    k,
			Source: source,
			Value:  cat.Resolve(ov, locale, proj.FallbackLocale, k),
			State:  TranslationDefault,
		}
		if o, ok := byKey[k]; ok {
			v.State = TranslationCustom
			if o.SourceHash != sourceHash(source) {
				v.State = TranslationNeedsReview
			}
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
	cat := i18n.Platform()
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

	cat := i18n.Platform()
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
