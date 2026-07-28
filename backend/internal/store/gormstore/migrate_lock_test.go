package gormstore_test

import (
	"sync"
	"testing"

	"github.com/bitllow/sild/backend/internal/config"
	"github.com/bitllow/sild/backend/internal/store/gormstore"
)

// Two overlapping deploys — `make deploy` twice, a retried CI job, an operator
// running it by hand while the Job is live — put two AutoMigrate passes on one
// database. Unserialized that is concurrent DDL on the same tables: lock
// contention on Postgres, and on MySQL two CREATE INDEX statements racing on one
// table, where the loser fails the migration Job.
//
// Each migrator opens its own pool, because that is what separate processes do:
// sharing one *gorm.DB would let the pool serialize them by accident.
func TestConcurrentMigrationsAllSucceed(t *testing.T) {
	for _, dbc := range dialects(t) {
		t.Run(string(dbc.Driver), func(t *testing.T) {
			if dbc.Driver == config.SQLite {
				// SQLite has no advisory lock, so the guard genuinely does not cover
				// it: run this and the second migrator fails with "table tenants
				// already exists". Two processes migrating one SQLite file is a
				// local-dev accident, not a deployment (ARCHITECTURE §4).
				t.Skip("no advisory lock on sqlite — single-node by construction")
			}
			const migrators = 4
			var wg sync.WaitGroup
			errs := make([]error, migrators)
			start := make(chan struct{})

			for i := range migrators {
				db, err := gormstore.Open(&config.Config{DB: dbc})
				if err != nil {
					t.Fatalf("open %d: %v", i, err)
				}
				wg.Add(1)
				go func() {
					defer wg.Done()
					<-start
					errs[i] = gormstore.Migrate(db)
				}()
			}
			close(start)
			wg.Wait()

			for i, err := range errs {
				if err != nil {
					t.Errorf("migrator %d failed: %v", i, err)
				}
			}
			// And the schema is actually usable afterwards, not half-built.
			db, err := gormstore.Open(&config.Config{DB: dbc})
			if err != nil {
				t.Fatalf("reopen: %v", err)
			}
			var n int64
			if err := db.Table("tenants").Count(&n).Error; err != nil {
				t.Fatalf("tenants table unusable after concurrent migration: %v", err)
			}
		})
	}
}

// The lock must not leak: a second Migrate after the first returned has to acquire
// it again rather than block forever on a lock nobody released.
func TestMigrationLockIsReleased(t *testing.T) {
	for _, dbc := range dialects(t) {
		t.Run(string(dbc.Driver), func(t *testing.T) {
			for pass := range 3 {
				db, err := gormstore.Open(&config.Config{DB: dbc})
				if err != nil {
					t.Fatalf("open: %v", err)
				}
				if err := gormstore.Migrate(db); err != nil {
					t.Fatalf("pass %d: %v", pass, err)
				}
				if sqlDB, err := db.DB(); err == nil {
					_ = sqlDB.Close() // a real migrator exits here
				}
			}
		})
	}
}
