package policy_test

import (
	"slices"
	"testing"

	"github.com/bitllow/sild/backend/internal/policy"
	"github.com/bitllow/sild/backend/internal/principal"
	"github.com/bitllow/sild/backend/internal/store/models"
)

func translator(scope models.RoleScope) *principal.Principal {
	return &principal.Principal{
		TenantID: "t_1", Kind: principal.KindAdmin, AdminID: "adm_1",
		Assignments: []principal.Assignment{{Role: models.PlatformTranslator, Scope: scope}},
	}
}

func everything() models.RoleScope {
	return models.RoleScope{
		Projects: []string{models.ScopeAll}, Locales: []string{models.ScopeAll}, Publish: true,
	}
}

func TestATranslatorHoldsExactlyTheTranslationCapabilities(t *testing.T) {
	want := []policy.Action{
		policy.PrincipalRead,
		policy.TranslationsExport,
		policy.TranslationsImport,
		policy.TranslationsPublish,
		policy.TranslationsRead,
		policy.TranslationsWrite,
	}
	got := policy.TranslatorActions()
	if !slices.Equal(got, want) {
		t.Fatalf("translator capabilities = %v, want %v", got, want)
	}
}

// A build token holds a column of its own, so a capability added without
// considering one fails here rather than widening every CI key in the field.
func TestABuildTokenHoldsExactlyTheBuildSurface(t *testing.T) {
	want := []policy.Action{
		policy.PrincipalRead,
		policy.TranslationsExport,
		policy.TranslationsFetch,
		policy.TranslationsImport,
		policy.TranslationsPublish,
	}
	if got := policy.BuildTokenActions(); !slices.Equal(got, want) {
		t.Fatalf("build token capabilities = %v, want %v", got, want)
	}
	// And it can only ever hold what an API key holds.
	for _, a := range policy.BuildTokenActions() {
		if !policy.APIKeyHolds(a) {
			t.Errorf("%s is granted to a build token but not to an API key", a)
		}
	}
}

func TestEveryOtherActionRefusesATranslator(t *testing.T) {
	held := policy.TranslatorActions()
	for _, a := range policy.Actions() {
		if slices.Contains(held, a) {
			continue
		}
		if err := policy.Authorize(translator(everything()), a, policy.ResourceAttrs{}); err == nil {
			t.Errorf("%s is allowed to a translator", a)
		}
	}
}

func TestAScopeNarrowsATranslatorToItsProjectsAndLanguages(t *testing.T) {
	p := translator(models.RoleScope{Projects: []string{"sild"}, Locales: []string{"lv"}})

	if err := policy.AuthorizeTranslation(p, policy.TranslationsWrite, "sild", "lv"); err != nil {
		t.Fatalf("granted project and language refused: %v", err)
	}
	if err := policy.AuthorizeTranslation(p, policy.TranslationsWrite, "sild", "es"); err == nil {
		t.Error("an ungranted language was allowed")
	}
	if err := policy.AuthorizeTranslation(p, policy.TranslationsWrite, "other", "lv"); err == nil {
		t.Error("an ungranted project was allowed")
	}
}

// An empty set is what a fresh assignment carries, and it must read as nothing
// granted — the alternative hands a new translator the whole tenant.
func TestAnEmptyScopeGrantsNothing(t *testing.T) {
	if err := policy.AuthorizeTranslation(translator(models.RoleScope{}),
		policy.TranslationsWrite, "sild", "es"); err == nil {
		t.Fatal("an unscoped translator was allowed to write")
	}
}

func TestAllKeepsGrantingAProjectAddedLater(t *testing.T) {
	p := translator(models.RoleScope{Projects: []string{models.ScopeAll}, Locales: []string{"lv"}})
	if err := policy.AuthorizeTranslation(p, policy.TranslationsWrite, "a-project-invented-today", "lv"); err != nil {
		t.Fatalf("all refused a new project: %v", err)
	}
}

func TestPublishingIsItsOwnGrant(t *testing.T) {
	scoped := models.RoleScope{Projects: []string{"sild"}, Locales: []string{models.ScopeAll}}
	if err := policy.AuthorizeTranslation(translator(scoped), policy.TranslationsPublish, "sild", ""); err == nil {
		t.Fatal("a translator without the publish grant cut a release")
	}
	scoped.Publish = true
	if err := policy.AuthorizeTranslation(translator(scoped), policy.TranslationsPublish, "sild", ""); err != nil {
		t.Fatalf("a translator granted publishing was refused: %v", err)
	}
	if err := policy.AuthorizeTranslation(translator(scoped), policy.TranslationsPublish, "other", ""); err == nil {
		t.Error("publishing reached a project the assignment does not name")
	}
}

// A release is project-wide, so publishing part of one is not a thing to grant.
func TestPublishingNeedsEveryLanguageOfTheProject(t *testing.T) {
	oneLanguage := models.RoleScope{Projects: []string{"sild"}, Locales: []string{"lv"}, Publish: true}
	if err := policy.AuthorizeTranslation(translator(oneLanguage), policy.TranslationsPublish, "sild", ""); err == nil {
		t.Fatal("a translator scoped to one language published every language")
	}
	if policy.MayPublish(translator(oneLanguage), "sild") {
		t.Error("MayPublish disagrees with the decision it is meant to mirror")
	}
	// And their write must not trip a release through the project's auto-publish.
	if policy.MayPublish(translator(models.RoleScope{Projects: []string{models.ScopeAll}, Locales: []string{models.ScopeAll}}), "sild") {
		t.Error("a draft-only translator would auto-publish")
	}
}

// A scope narrows the translator role. It must not narrow a role that reaches
// the action on its own.
func TestATranslatorScopeDoesNotNarrowAnOwner(t *testing.T) {
	p := &principal.Principal{
		TenantID: "t_1", Kind: principal.KindAdmin, AdminID: "adm_2",
		Assignments: []principal.Assignment{
			{Role: models.PlatformOwner},
			{Role: models.PlatformTranslator, Scope: models.RoleScope{Locales: []string{"lv"}}},
		},
	}
	if err := policy.AuthorizeTranslation(p, policy.TranslationsWrite, "sild", "es"); err != nil {
		t.Fatalf("owner narrowed by a translator scope: %v", err)
	}
}

func TestASecondRoleOnlyWidens(t *testing.T) {
	both := &principal.Principal{
		TenantID: "t_1", Kind: principal.KindAdmin, AdminID: "adm_3",
		Assignments: append(principal.Held(models.PlatformAgent),
			principal.Assignment{Role: models.PlatformTranslator, Scope: everything()}),
	}
	if err := policy.Authorize(both, policy.ConversationsList, policy.ResourceAttrs{}); err != nil {
		t.Fatalf("the agent role stopped working next to a translator role: %v", err)
	}
	if err := policy.Authorize(both, policy.TranslationsWrite, policy.ResourceAttrs{}); err != nil {
		t.Fatalf("the translator role stopped working next to an agent role: %v", err)
	}
}
