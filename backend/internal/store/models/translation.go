package models

import (
	"time"

	"gorm.io/datatypes"
)

// TranslationProject is a tenant's settings for one project. The platform
// project's keys come from the repo (docs/adr/0003), so there are no key rows —
// a tenant holds only what it changed. A tenant-owned project declares its own.
type TranslationProject struct {
	TenantID       string `gorm:"primaryKey;size:40"`
	Slug           string `gorm:"primaryKey;size:64"`
	Name           string `gorm:"size:128"`
	FallbackLocale string `gorm:"size:16;not null"`
	AutoPublish    bool
	UpdatedAt      time.Time
}

// TranslationKey is one string a tenant-owned project declares: the key and the
// source text every translation of it is written against.
type TranslationKey struct {
	TenantID  string `gorm:"primaryKey;size:40"`
	Project   string `gorm:"primaryKey;size:64"`
	Key       string `gorm:"primaryKey;size:255;column:string_key"`
	Source    string `gorm:"type:text"`
	UpdatedAt time.Time
}

// TranslationProjectLocale is a locale the tenant has turned on for a project.
type TranslationProjectLocale struct {
	TenantID string `gorm:"primaryKey;size:40"`
	Project  string `gorm:"primaryKey;size:64"`
	Locale   string `gorm:"primaryKey;size:16"`
}

// TranslationOverride is a tenant's own text for one key in one locale, and a
// draft until a release is published. Column is string_key because `key` is
// reserved in MySQL.
type TranslationOverride struct {
	TenantID string `gorm:"primaryKey;size:40"`
	Project  string `gorm:"primaryKey;size:64"`
	Locale   string `gorm:"primaryKey;size:16"`
	Key      string `gorm:"primaryKey;size:255;column:string_key"`
	Value    string `gorm:"type:text"`
	// SourceHash is the source text this was written against. A mismatch against
	// the current source is what flags the row for review.
	SourceHash string `gorm:"size:64"`
	UpdatedAt  time.Time
}

// TranslationRelease is a published version of a project. Immutable once cut.
type TranslationRelease struct {
	ID          string `gorm:"primaryKey;size:40"`
	TenantID    string `gorm:"size:40;not null;index:idx_trelease_project,priority:1"`
	Project     string `gorm:"size:64;not null;index:idx_trelease_project,priority:2"`
	Version     int    `gorm:"not null;index:idx_trelease_project,priority:3"`
	PublishedBy string `gorm:"size:40"`
	CreatedAt   time.Time
}

// TranslationBundle is one locale of one release, materialized at publish time
// so a version's content cannot change afterwards.
type TranslationBundle struct {
	TenantID string         `gorm:"primaryKey;size:40"`
	Project  string         `gorm:"primaryKey;size:64"`
	Locale   string         `gorm:"primaryKey;size:16"`
	Version  int            `gorm:"primaryKey"`
	Strings  datatypes.JSON `gorm:"type:json"`
}

// TranslatorScope limits a translator to one project or one locale. No rows of a
// kind means every value of it.
type TranslatorScope struct {
	TenantID    string `gorm:"primaryKey;size:40"`
	AdminUserID string `gorm:"primaryKey;size:40"`
	Kind        string `gorm:"primaryKey;size:16"`
	Value       string `gorm:"primaryKey;size:64"`
}

// Scope kinds for TranslatorScope.
const (
	ScopeProject = "project"
	ScopeLocale  = "locale"
)
