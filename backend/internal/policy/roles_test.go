package policy_test

import (
	"testing"

	"github.com/bitllow/sild/backend/internal/policy"
	"github.com/bitllow/sild/backend/internal/store/models"
)

// The catalogue is what the Team screen renders. A role the capability table
// knows but the catalogue does not would reach the API and never the UI.
func TestEveryRoleIsDescribed(t *testing.T) {
	for _, r := range policy.EveryAdminRole() {
		def, ok := policy.RoleDefinition(r)
		if !ok {
			t.Errorf("%s has no description", r)
			continue
		}
		if def.Label == "" || def.Description == "" {
			t.Errorf("%s is described as %q/%q", r, def.Label, def.Description)
		}
	}
	if len(policy.Roles()) != len(policy.EveryAdminRole()) {
		t.Fatalf("catalogue has %d roles, the capability table has %d",
			len(policy.Roles()), len(policy.EveryAdminRole()))
	}
}

func TestAScopeIsRefusedWhenItsRoleHasNoSuchLimit(t *testing.T) {
	if err := policy.ValidateScope(models.PlatformAdmin, models.RoleScope{Peer: true}); err == nil {
		t.Error("the admin role accepted a peer scope")
	}
	if err := policy.ValidateScope(models.PlatformAgent, models.RoleScope{Locales: []string{"lv"}}); err == nil {
		t.Error("the agent role accepted a language scope")
	}
	if err := policy.ValidateScope(models.PlatformAgent, models.RoleScope{Peer: true}); err != nil {
		t.Errorf("the agent role refused its own dimension: %v", err)
	}
	if err := policy.ValidateScope(models.PlatformOwner, models.RoleScope{}); err != nil {
		t.Errorf("a tenant-wide role refused an empty scope: %v", err)
	}
}

// Every set dimension has to say where its options come from, or the screen
// renders an empty picker.
func TestEverySetDimensionNamesItsSource(t *testing.T) {
	for _, def := range policy.Roles() {
		for _, d := range def.Dimensions {
			if d.Kind == policy.DimensionSet && d.Source == "" {
				t.Errorf("%s.%s is a set with no source", def.Role, d.Key)
			}
			if d.Label == "" || d.Help == "" {
				t.Errorf("%s.%s is unlabelled", def.Role, d.Key)
			}
		}
	}
}
