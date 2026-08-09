package policy_test

import (
	"slices"
	"testing"

	"github.com/bitllow/sild/backend/internal/policy"
	"github.com/bitllow/sild/backend/internal/principal"
	"github.com/bitllow/sild/backend/internal/store/models"
)

func translator() *principal.Principal {
	return &principal.Principal{
		TenantID: "t_1", Kind: principal.KindAdmin,
		AdminID: "adm_1", Role: models.PlatformTranslator,
	}
}

func TestATranslatorHoldsExactlyTheTranslationCapabilities(t *testing.T) {
	want := []policy.Action{
		policy.PrincipalRead,
		policy.TranslationsRead,
		policy.TranslationsWrite,
	}
	got := policy.TranslatorActions()
	if !slices.Equal(got, want) {
		t.Fatalf("translator capabilities = %v, want %v", got, want)
	}
}

func TestEveryOtherActionRefusesATranslator(t *testing.T) {
	held := policy.TranslatorActions()
	for _, a := range policy.Actions() {
		if slices.Contains(held, a) {
			continue
		}
		if err := policy.Authorize(translator(), a, policy.ResourceAttrs{}); err == nil {
			t.Errorf("%s is allowed to a translator", a)
		}
	}
}

func TestAGrantNarrowsATranslatorToItsProjectsAndLanguages(t *testing.T) {
	at := policy.TranslationAttrs{Projects: []string{"sild"}, Locales: []string{"lv"}}

	if err := policy.AuthorizeTranslation(translator(), policy.TranslationsWrite, at, "sild", "lv"); err != nil {
		t.Fatalf("granted project and language refused: %v", err)
	}
	if err := policy.AuthorizeTranslation(translator(), policy.TranslationsWrite, at, "sild", "es"); err == nil {
		t.Error("an ungranted language was allowed")
	}
	if err := policy.AuthorizeTranslation(translator(), policy.TranslationsWrite, at, "other", "lv"); err == nil {
		t.Error("an ungranted project was allowed")
	}
}

func TestAnEmptyGrantMeansEveryProjectAndLanguage(t *testing.T) {
	if err := policy.AuthorizeTranslation(translator(), policy.TranslationsWrite,
		policy.TranslationAttrs{}, "sild", "es"); err != nil {
		t.Fatalf("ungranted translator refused: %v", err)
	}
}

// Grants narrow a translator; they must not narrow anyone else.
func TestAGrantDoesNotNarrowAnOwner(t *testing.T) {
	owner := &principal.Principal{
		TenantID: "t_1", Kind: principal.KindAdmin, AdminID: "adm_2", Role: models.PlatformOwner,
	}
	at := policy.TranslationAttrs{Locales: []string{"lv"}}
	if err := policy.AuthorizeTranslation(owner, policy.TranslationsWrite, at, "sild", "es"); err != nil {
		t.Fatalf("owner narrowed by a translator grant: %v", err)
	}
}
