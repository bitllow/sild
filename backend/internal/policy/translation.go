package policy

import (
	"slices"

	"github.com/bitllow/sild/backend/internal/principal"
	"github.com/bitllow/sild/backend/internal/store/models"
)

// AuthorizeTranslation decides a translation action against one project and
// locale. Locale is empty for project-wide actions. Only the translator role
// carries a scope; every other role reaching the action is tenant-wide.
func AuthorizeTranslation(p *principal.Principal, a Action, project, locale string) error {
	if err := Authorize(p, a, ResourceAttrs{}); err != nil {
		return err
	}
	scope, ok := p.ScopeOf(models.PlatformTranslator)
	if !ok || holdsWithoutTranslator(p, a) {
		return nil
	}
	if !models.Allows(scope.Projects, project) {
		return denied("translation_scope", "not granted this project")
	}
	if locale != "" && !models.Allows(scope.Locales, locale) {
		return denied("translation_scope", "not granted this language")
	}
	if a == TranslationsPublish {
		if !scope.Publish {
			return denied("translation_scope", "not granted publishing")
		}
		// A release is project-wide, so cutting one takes every language of the
		// project — a translator narrowed to some of them cannot publish the rest.
		if !slices.Contains(scope.Locales, models.ScopeAll) {
			return denied("translation_scope", "publishing takes every language of the project")
		}
	}
	return nil
}

// MayPublish reports whether this caller could cut a release of the project. It
// is what decides whether their write auto-publishes: a draft-only translator
// must not trip a release through a project setting.
func MayPublish(p *principal.Principal, project string) bool {
	return AuthorizeTranslation(p, TranslationsPublish, project, "") == nil
}

// TranslationNarrowing is the scope a collection has to filter by, and whether
// it filters at all — a translator granted one project must land on it and must
// not learn the others exist.
func TranslationNarrowing(p *principal.Principal, a Action) (models.RoleScope, bool) {
	scope, ok := p.ScopeOf(models.PlatformTranslator)
	if !ok || holdsWithoutTranslator(p, a) {
		return models.RoleScope{}, false
	}
	return scope, true
}

// holdsWithoutTranslator reports whether another of the member's roles carries
// the action on its own, in which case the translator scope narrows nothing —
// an admin who is also a Spanish translator is still an admin.
func holdsWithoutTranslator(p *principal.Principal, a Action) bool {
	g, ok := capabilities[a]
	if !ok {
		return false
	}
	for _, r := range p.Roles() {
		if r != models.PlatformTranslator && roleHolds(g, r) {
			return true
		}
	}
	return false
}
