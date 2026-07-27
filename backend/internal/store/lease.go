package store

import (
	"context"
	"fmt"
	"time"

	"github.com/bitllow/sild/backend/internal/id"
)

// Lease names for work that must happen once per cluster rather than once per
// replica. Held in the database (LeaseRepo) so no coordinator is needed.
const (
	LeaseSigningKey = "signing-key-bootstrap"
)

// OutboxClaimTTL bounds how long one relay may hold a claimed outbox row before
// another may take it. Relays renew within it while a batch is still running.
const OutboxClaimTTL = 5 * time.Minute

const (
	leaseTTL  = 5 * time.Minute
	leasePoll = 200 * time.Millisecond
	// Longer than leaseTTL, so a waiter outlives a holder that died still holding
	// it instead of timing out at the same moment the lease lapses.
	leaseWaitMax = 6 * time.Minute
)

// RunExclusive runs fn under a cluster-wide lease: never two replicas at once.
// Every caller ends up running fn, the losers once the holder releases — a
// holder that FAILED inside fn releases the lease too, so "lease gone" cannot be
// read as "work done". fn must therefore be idempotent and cheap when there is
// nothing left to do; it is the postcondition check, not the lease.
func RunExclusive(ctx context.Context, leases LeaseRepo, name string, fn func() error) error {
	owner := id.New(id.Holder)
	deadline := time.Now().Add(leaseWaitMax)
	for {
		ok, err := leases.Acquire(ctx, name, owner, leaseTTL)
		if err != nil {
			return err
		}
		if ok {
			defer func() { _ = leases.Release(context.WithoutCancel(ctx), name, owner) }()
			return fn()
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("timed out waiting for another replica to finish %q", name)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(leasePoll):
		}
	}
}
