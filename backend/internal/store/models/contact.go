package models

import (
	"time"

	"gorm.io/datatypes"
)

// Contact is a person's stored profile, one row per (tenant, external user id),
// written by the SDK on start-up or by the host backend and shared by every
// conversation they are in. Narrow on purpose: contact search scans SearchText
// tenant-wide, so the profile blob lives in ContactMeta instead of here.
type Contact struct {
	TenantID       string `gorm:"primaryKey;size:40"`
	ExternalUserID string `gorm:"primaryKey;size:255"`
	// SearchText is the materialized concat of the tenant's searchable metadata
	// keys, rebuilt on every profile write; the trigram/fulltext index targets it.
	SearchText string `gorm:"type:text"`
	// PushOptedOutAt is Sild's own per-contact state, in a typed column so a
	// profile write can never disturb a suppression the tenant set.
	PushOptedOutAt *time.Time
	UpdatedAt      time.Time
}

// ContactMeta is the host-defined profile blob, split off so the scanned table
// stays narrow.
type ContactMeta struct {
	TenantID       string         `gorm:"primaryKey;size:40"`
	ExternalUserID string         `gorm:"primaryKey;size:255"`
	Metadata       datatypes.JSON `gorm:"type:json"`
}

func (ContactMeta) TableName() string { return "contacts_meta" }
