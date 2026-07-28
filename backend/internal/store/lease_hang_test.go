package store_test

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/bitllow/sild/backend/internal/store"
)

// hangingLeases grants the lease once, then never answers again — a database
// that stops responding rather than one that refuses.
type hangingLeases struct{ calls atomic.Int32 }

func (h *hangingLeases) Acquire(ctx context.Context, _, _ string, _ time.Duration) (bool, error) {
	if h.calls.Add(1) == 1 {
		return true, nil
	}
	<-ctx.Done() // every renewal hangs until its own bound expires
	return false, ctx.Err()
}

func (h *hangingLeases) Release(context.Context, string, string) error { return nil }
func (h *hangingLeases) Held(context.Context, string) (bool, error)    { return false, nil }

// The lease lapses on schedule whether or not the database answers, so an
// unresponsive one must still stop the work: another replica is free to take the
// lease the moment it expires, and a run that carried on would overlap it.
func TestRunLeasedStopsWhenRenewalCannotReachTheDatabase(t *testing.T) {
	const ttl = 300 * time.Millisecond
	var observed atomic.Bool

	start := time.Now()
	ran, err := store.RunLeased(context.Background(), &hangingLeases{}, "stuck", ttl, func(runCtx context.Context) error {
		select {
		case <-runCtx.Done():
			observed.Store(true)
		case <-time.After(10 * time.Second):
		}
		return nil
	})
	elapsed := time.Since(start)

	if !ran {
		t.Fatal("the work never ran")
	}
	if !observed.Load() {
		t.Fatal("a hung renewal left the work running past its lease")
	}
	if !errors.Is(err, store.ErrLeaseLost) {
		t.Fatalf("reported %v, want ErrLeaseLost", err)
	}
	// Bounded by the expiry, not by however long the database takes to fail.
	if max := 3 * ttl; elapsed > max {
		t.Fatalf("took %v to give up the lease, want under %v", elapsed, max)
	}
}
