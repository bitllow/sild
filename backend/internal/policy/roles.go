package policy

import (
	"fmt"

	"github.com/bitllow/sild/backend/internal/store/models"
)

// Dimension kinds. A set takes ids or the literal models.ScopeAll; a toggle
// takes a boolean.
const (
	DimensionSet    = "set"
	DimensionToggle = "toggle"
)

// Dimension is one limit a role's scope may carry. Source names where the UI
// fetches the options for a set — this package will not join a service to
// enumerate them.
type Dimension struct {
	Key    string `json:"key"`
	Kind   string `json:"kind"`
	Label  string `json:"label"`
	Help   string `json:"help"`
	Source string `json:"source,omitempty"`
}

// RoleDef describes one role: what it is for, and the dimensions its scope has.
// A role with no dimensions is tenant-wide.
type RoleDef struct {
	Role        models.PlatformRole `json:"role"`
	Label       string              `json:"label"`
	Description string              `json:"description"`
	Dimensions  []Dimension         `json:"dimensions"`
}

// Scope dimension keys, so a validator and a wire projection cannot drift.
const (
	DimPeer     = "peer"
	DimProjects = "projects"
	DimLocales  = "locales"
	DimPublish  = "publish"
)

// roleDefs is the role catalogue, beside the capability table it describes: a
// role added to one without the other fails the contract test rather than
// shipping unlabelled.
var roleDefs = []RoleDef{
	{
		Role: models.PlatformOwner, Label: "owner", Dimensions: []Dimension{},
		Description: "Full control of the tenant, including team, billing and deletion.",
	},
	{
		Role: models.PlatformAdmin, Label: "admin", Dimensions: []Dimension{},
		Description: "Manages settings, keys, webhooks and members.",
	},
	{
		Role: models.PlatformAgent, Label: "agent", Dimensions: []Dimension{
			{Key: DimPeer, Kind: DimensionToggle, Label: "Peer conversations",
				Help: "Direct chats between parties with no assigned agent."},
		},
		Description: "Works the assignment queue.",
	},
	{
		Role: models.PlatformTranslator, Label: "translator", Dimensions: []Dimension{
			{Key: DimProjects, Kind: DimensionSet, Label: "Projects", Source: "translation_projects",
				Help: "Translation projects this assignment may write."},
			{Key: DimLocales, Kind: DimensionSet, Label: "Languages", Source: "translation_locales",
				Help: "Languages this assignment may write."},
			{Key: DimPublish, Kind: DimensionToggle, Label: "Publish",
				Help: "Cut a release, rather than leaving edits as drafts."},
		},
		Description: "Edits strings only. Reaches no conversation, contact or message.",
	},
}

// Roles returns the catalogue, for the endpoint the Team screen renders from.
func Roles() []RoleDef { return roleDefs }

// RoleDefinition returns one role's description, if the role exists.
func RoleDefinition(r models.PlatformRole) (RoleDef, bool) {
	for _, d := range roleDefs {
		if d.Role == r {
			return d, true
		}
	}
	return RoleDef{}, false
}

// ValidateScope refuses a scope carrying a dimension its role does not define,
// so a client cannot store a limit no decision will ever read.
func ValidateScope(r models.PlatformRole, s models.RoleScope) error {
	def, ok := RoleDefinition(r)
	if !ok {
		return fmt.Errorf("unknown role %q", r)
	}
	for _, set := range []struct {
		key  string
		used bool
	}{
		{DimPeer, s.Peer},
		{DimProjects, len(s.Projects) > 0},
		{DimLocales, len(s.Locales) > 0},
		{DimPublish, s.Publish},
	} {
		if set.used && !def.defines(set.key) {
			return fmt.Errorf("the %s role has no %s", r, set.key)
		}
	}
	return nil
}

func (d RoleDef) defines(key string) bool {
	for _, dim := range d.Dimensions {
		if dim.Key == key {
			return true
		}
	}
	return false
}
