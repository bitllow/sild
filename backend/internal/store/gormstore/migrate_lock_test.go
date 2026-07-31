package gormstore_test

import (
	"sync"
	"testing"

	"github.com/bitllow/sild/backend/internal/config"
	"github.com/bitllow/sild/backend/internal/store/gormstore"
)

func TestConcurrentMigrationsAllSucceed(t *testing.T) {
	for _, dbc := range dialects(t) {
		t.Run(string(dbc.Driver), func(t *testing.T) {
			if dbc.Driver == config.SQLite {
				t.Skip("sqlite has no advisory lock; single-node by construction")
			}
			const migrators = 2
			var wg sync.WaitGroup
			errs := make([]error, migrators)
			start := make(chan struct{})

			for i := range migrators {
				// Its own pool, because that is what a separate process has: sharing
				// one *gorm.DB would let the pool serialize them by accident.
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

			// The second migrator finishing at all is the release; a leaked lock hangs
			// it until the test times out.
			for i, err := range errs {
				if err != nil {
					t.Errorf("migrator %d failed: %v", i, err)
				}
			}
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
