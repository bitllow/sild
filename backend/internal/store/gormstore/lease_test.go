package gormstore_test

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/bitllow/sild/backend/internal/config"
	"github.com/bitllow/sild/backend/internal/id"
	"github.com/bitllow/sild/backend/internal/store"
	"github.com/bitllow/sild/backend/internal/store/gormstore"
	"github.com/bitllow/sild/backend/internal/store/models"
	"gorm.io/gorm"
)

// storeFor opens a migrated store for one dialect, handing back the raw handle
// too so a test can age rows the store has no reason to expose.
func storeFor(t *testing.T, dbc config.DB) (store.Store, *gorm.DB) {
	t.Helper()
	db, err := gormstore.Open(&config.Config{DB: dbc})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := gormstore.Migrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return gormstore.New(db), db
}

// sqliteStore is the throwaway single-dialect store the non-dialect tests use.
func sqliteStore(t *testing.T) (store.Store, *gorm.DB) {
	t.Helper()
	return storeFor(t, config.DB{Driver: config.SQLite, DSN: t.TempDir() + "/test.db"})
}

// The lease is the primitive every once-per-cluster job rests on: two replicas
// asking for the same name, only one gets it.
func TestLeaseIsExclusive(t *testing.T) {
	for _, dbc := range dialects(t) {
		t.Run(string(dbc.Driver), func(t *testing.T) {
			ctx := context.Background()
			st, _ := storeFor(t, dbc)
			name := id.New("lease")

			first, err := st.Leases().Acquire(ctx, name, "replica-a", time.Minute)
			if err != nil || !first {
				t.Fatalf("first acquire: ok=%v err=%v", first, err)
			}
			second, err := st.Leases().Acquire(ctx, name, "replica-b", time.Minute)
			if err != nil {
				t.Fatalf("second acquire: %v", err)
			}
			if second {
				t.Fatal("two replicas hold the same lease")
			}
			// The holder may renew without losing it.
			again, err := st.Leases().Acquire(ctx, name, "replica-a", time.Minute)
			if err != nil || !again {
				t.Fatalf("renew: ok=%v err=%v", again, err)
			}
		})
	}
}

// A holder that dies must not block the job forever — expiry is what makes the
// lease recoverable without an operator.
func TestExpiredLeaseIsTakenOver(t *testing.T) {
	for _, dbc := range dialects(t) {
		t.Run(string(dbc.Driver), func(t *testing.T) {
			ctx := context.Background()
			st, _ := storeFor(t, dbc)
			name := id.New("lease")

			if ok, err := st.Leases().Acquire(ctx, name, "dead-replica", -time.Second); err != nil || !ok {
				t.Fatalf("acquire: ok=%v err=%v", ok, err)
			}
			took, err := st.Leases().Acquire(ctx, name, "live-replica", time.Minute)
			if err != nil || !took {
				t.Fatalf("takeover: ok=%v err=%v", took, err)
			}
			held, err := st.Leases().Held(ctx, name)
			if err != nil || !held {
				t.Fatalf("held after takeover: %v %v", held, err)
			}
		})
	}
}

// Release is owner-scoped: a holder that overran its TTL must not release the
// lease the next replica has already taken.
func TestReleaseOnlyByOwner(t *testing.T) {
	ctx := context.Background()
	st, _ := sqliteStore(t)

	if ok, _ := st.Leases().Acquire(ctx, "owned", "replica-a", time.Minute); !ok {
		t.Fatal("acquire")
	}
	if err := st.Leases().Release(ctx, "owned", "replica-b"); err != nil {
		t.Fatalf("release by stranger: %v", err)
	}
	held, err := st.Leases().Held(ctx, "owned")
	if err != nil || !held {
		t.Fatalf("a stranger released another replica's lease: held=%v err=%v", held, err)
	}
	if err := st.Leases().Release(ctx, "owned", "replica-a"); err != nil {
		t.Fatalf("release by owner: %v", err)
	}
	if held, _ := st.Leases().Held(ctx, "owned"); held {
		t.Fatal("owner's release did not drop the lease")
	}
}

// What signing-key bootstrap depends on: eight replicas call RunExclusive
// together and the body is never entered twice at once. Mutual exclusion, not a
// total run count — a replica arriving after the holder released runs it again,
// which is why every body under it must be idempotent.
func TestRunExclusiveNeverOverlaps(t *testing.T) {
	for _, dbc := range dialects(t) {
		t.Run(string(dbc.Driver), func(t *testing.T) {
			ctx := context.Background()
			st, _ := storeFor(t, dbc)
			name := id.New("lease")

			var inside, maxInside atomic.Int32
			var wg sync.WaitGroup
			errs := make(chan error, 8)
			for i := 0; i < 8; i++ {
				wg.Add(1)
				go func() {
					defer wg.Done()
					errs <- store.RunExclusive(ctx, st.Leases(), name, func() error {
						n := inside.Add(1)
						for {
							m := maxInside.Load()
							if n <= m || maxInside.CompareAndSwap(m, n) {
								break
							}
						}
						time.Sleep(10 * time.Millisecond)
						inside.Add(-1)
						return nil
					})
				}()
			}
			wg.Wait()
			close(errs)
			for err := range errs {
				if err != nil {
					t.Fatalf("RunExclusive: %v", err)
				}
			}
			if got := maxInside.Load(); got != 1 {
				t.Fatalf("%d replicas ran the guarded work at the same time, want 1", got)
			}
			if held, _ := st.Leases().Held(ctx, name); held {
				t.Fatal("lease still held after every caller returned")
			}
		})
	}
}

// A holder that FAILS inside fn releases the lease just like one that succeeded.
// A waiter that read "lease gone" as "work done" would return success with the
// job never done — on a cold start that means API pods serving with no signing
// key. Every waiter must therefore end up running the (idempotent) work itself.
func TestWaiterRunsTheWorkWhenTheHolderFails(t *testing.T) {
	ctx := context.Background()
	st, _ := sqliteStore(t)
	name := id.New("lease")

	var runs atomic.Int32
	failing := func() error {
		runs.Add(1)
		return errors.New("boom")
	}
	if err := store.RunExclusive(ctx, st.Leases(), name, failing); err == nil {
		t.Fatal("the holder's failure was swallowed")
	}

	// The next replica must not treat the released lease as a completed job.
	var second atomic.Int32
	if err := store.RunExclusive(ctx, st.Leases(), name, func() error {
		second.Add(1)
		return nil
	}); err != nil {
		t.Fatalf("second replica: %v", err)
	}
	if second.Load() != 1 {
		t.Fatalf("the work ran %d times on the second replica, want 1", second.Load())
	}
}

// Renewal is what a long job leans on: the archive sweep re-acquires between
// tenants so a pass longer than the TTL keeps its lease instead of lapsing into
// an overlapping second sweep.
func TestRenewalExtendsTheExpiry(t *testing.T) {
	ctx := context.Background()
	st, db := sqliteStore(t)
	name := id.New("lease")

	if ok, _ := st.Leases().Acquire(ctx, name, "worker-a", time.Minute); !ok {
		t.Fatal("acquire")
	}
	expiry := func() time.Time {
		var l models.JobLease
		if err := db.First(&l, "name = ?", name).Error; err != nil {
			t.Fatalf("read lease: %v", err)
		}
		return l.ExpiresAt
	}
	before := expiry()
	if ok, err := st.Leases().Acquire(ctx, name, "worker-a", time.Hour); err != nil || !ok {
		t.Fatalf("renew: ok=%v err=%v", ok, err)
	}
	if !expiry().After(before) {
		t.Fatal("renewing did not push the expiry out, so a long job still lapses")
	}
}
