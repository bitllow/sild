package auth_test

import (
	"context"
	"sync"
	"testing"

	"github.com/bitllow/sild/backend/internal/auth"
	"github.com/bitllow/sild/backend/internal/config"
	"github.com/bitllow/sild/backend/internal/store"
	"github.com/bitllow/sild/backend/internal/store/gormstore"
	"gorm.io/gorm"
)

func bootStore(t *testing.T) (store.Store, *gorm.DB) {
	t.Helper()
	db, err := gormstore.Open(&config.Config{DB: config.DB{Driver: config.SQLite, DSN: t.TempDir() + "/keys.db"}})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := gormstore.Migrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return gormstore.New(db), db
}

func keyCfg() *config.Config {
	return &config.Config{Auth: config.Auth{Issuer: "https://test.local"}}
}

// Replicas booting together must agree on one signing key. Two active keys is
// survivable today (JWKS publishes both) and stops being so the moment rotation
// assumes a single active key.
func TestConcurrentBootMintsOneKey(t *testing.T) {
	ctx := context.Background()
	st, _ := bootStore(t)

	var wg sync.WaitGroup
	errs := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			// Each replica builds its own KeyManager, as separate processes do.
			errs <- auth.NewKeyManager(st, keyCfg()).EnsureActiveKey(ctx)
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("EnsureActiveKey: %v", err)
		}
	}

	keys, err := st.SigningKeys().Published(ctx)
	if err != nil {
		t.Fatalf("published: %v", err)
	}
	if len(keys) != 1 {
		t.Fatalf("%d signing keys after a concurrent cold start, want 1", len(keys))
	}
}

// Booting again against an existing key is a no-op, not a second key.
func TestRepeatedBootKeepsOneKey(t *testing.T) {
	ctx := context.Background()
	st, _ := bootStore(t)

	km := auth.NewKeyManager(st, keyCfg())
	for i := 0; i < 3; i++ {
		if err := km.EnsureActiveKey(ctx); err != nil {
			t.Fatalf("boot %d: %v", i, err)
		}
	}
	keys, _ := st.SigningKeys().Published(ctx)
	if len(keys) != 1 {
		t.Fatalf("%d signing keys after 3 boots, want 1", len(keys))
	}
}

// A broken key lookup is not an absent key. Treating a database failure as "no
// key yet" would mint a second active key on top of one that already exists, so
// the bootstrap has to surface the error instead.
func TestBootstrapSurfacesLookupFailures(t *testing.T) {
	ctx := context.Background()
	st, db := bootStore(t)
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if err := sqlDB.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	if err := auth.NewKeyManager(st, keyCfg()).EnsureActiveKey(ctx); err == nil {
		t.Fatal("a failing key lookup was reported as success")
	}
}
