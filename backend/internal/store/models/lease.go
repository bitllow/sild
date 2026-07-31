package models

import "time"

// JobLease is a named, expiring, cluster-wide mutex held in the database, so
// work that must run once across replicas (signing-key bootstrap, periodic jobs)
// has one owner at a time without a coordinator. Expiry is what makes a crashed
// holder recoverable — and what rules it out for schema migration, which holds a
// dialect advisory lock instead (see gormstore.Migrate).
type JobLease struct {
	Name      string `gorm:"primaryKey;size:64"`
	Owner     string `gorm:"size:64;not null"`
	ExpiresAt time.Time
}
