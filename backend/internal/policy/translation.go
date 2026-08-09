package policy

import (
	"slices"

	"github.com/bitllow/sild/backend/internal/principal"
	"github.com/bitllow/sild/backend/internal/store/models"
)

// TranslationAttrs is a translator's grant: which projects and which locales
// their role reaches. An empty set means every one of that dimension.
type TranslationAttrs struct {
	Projects []string
	Locales  []string
}

// AuthorizeTranslation decides a translation action against one project and
// locale. Locale is empty for project-wide actions. Grants narrow a translator
// only — nobody else carries a scope.
func AuthorizeTranslation(p *principal.Principal, a Action, at TranslationAttrs, project, locale string) error {
	if err := Authorize(p, a, ResourceAttrs{}); err != nil {
		return err
	}
	if p.Kind != principal.KindAdmin || p.Role != models.PlatformTranslator {
		return nil
	}
	if len(at.Projects) > 0 && !slices.Contains(at.Projects, project) {
		return denied("translation_scope", "not granted this project")
	}
	if locale != "" && len(at.Locales) > 0 && !slices.Contains(at.Locales, locale) {
		return denied("translation_scope", "not granted this language")
	}
	return nil
}
