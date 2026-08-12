package models

import (
	"time"

	"github.com/bitllow/sild/backend/internal/id"
	"gorm.io/gorm"
)

// ScopeAll is the set member that keeps granting as projects and languages are
// added. An empty set grants nothing — a fresh assignment reaches nothing until
// someone scopes it.
const ScopeAll = "all"

// RoleScope is the limits one role assignment carries. Every dimension of every
// role lives here; which of them a role may set is the role's own declaration,
// enforced on write.
type RoleScope struct {
	// agent
	Peer bool `json:"peer,omitempty"`
	// translator
	Projects []string `json:"projects,omitempty"`
	Locales  []string `json:"locales,omitempty"`
	Publish  bool     `json:"publish,omitempty"`
}

// Allows reports whether a set dimension admits one value.
func Allows(set []string, value string) bool {
	for _, s := range set {
		if s == ScopeAll || s == value {
			return true
		}
	}
	return false
}

// RoleAssignment is one member holding one role, with that role's scope. The
// member/role pair is unique, so widening someone is an edit rather than a
// second row, and a capability is held when any assignment carries it.
type RoleAssignment struct {
	ID          string       `gorm:"primaryKey;size:40"`
	TenantID    string       `gorm:"size:40;not null;uniqueIndex:idx_role_assignment_member"`
	AdminUserID string       `gorm:"size:40;not null;uniqueIndex:idx_role_assignment_member"`
	Role        PlatformRole `gorm:"size:16;not null;uniqueIndex:idx_role_assignment_member"`
	Scope       RoleScope    `gorm:"serializer:json"`
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

func (a *RoleAssignment) BeforeCreate(*gorm.DB) error {
	if a.ID == "" {
		a.ID = id.New(id.RoleAssign)
	}
	return nil
}
