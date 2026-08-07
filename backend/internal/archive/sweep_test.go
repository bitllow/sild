package archive_test

import (
	"context"
	"testing"
	"time"

	"github.com/bitllow/sild/backend/internal/archive"
	"github.com/bitllow/sild/backend/internal/testutil"
)

func newJob(t *testing.T, h *testutil.Harness) *archive.Job {
	t.Helper()
	return archive.NewJob(h.Store, h.Sink, h.Cfg)
}

// Every worker replica runs the archive ticker, so the sweep itself has to be
// the thing that is single-owner — otherwise two passes walk the same tenant.
func TestSweepSkipsWhenAnotherWorkerHoldsTheLease(t *testing.T) {
	h := testutil.New(t)
	ctx := context.Background()
	job := newJob(t, h)

	if ok, err := h.Store.Leases().Acquire(ctx, "archive-sweep", "other-worker", time.Minute); err != nil || !ok {
		t.Fatalf("simulate another worker: ok=%v err=%v", ok, err)
	}
	_, ran, err := job.RunSweep(ctx, 10)
	if err != nil {
		t.Fatalf("sweep: %v", err)
	}
	if ran {
		t.Fatal("swept while another worker held the lease")
	}
}

// With the lease free, the sweep runs and hands it back for the next tick.
func TestSweepRunsAndReleasesTheLease(t *testing.T) {
	h := testutil.New(t)
	ctx := context.Background()
	job := newJob(t, h)

	_, ran, err := job.RunSweep(ctx, 10)
	if err != nil || !ran {
		t.Fatalf("sweep: ran=%v err=%v", ran, err)
	}
	held, err := h.Store.Leases().Held(ctx, "archive-sweep")
	if err != nil {
		t.Fatalf("held: %v", err)
	}
	if held {
		t.Fatal("sweep kept the lease, so no worker can sweep on the next tick")
	}
}
