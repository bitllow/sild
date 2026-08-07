package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/bitllow/sild/backend/internal/id"
)

// Lease names for work that must happen once per cluster rather than once per
// replica. Held in the database (LeaseRepo) so no coordinator is needed.
const (
	LeaseSigningKey = "signing-key-bootstrap"
)

// ClaimTTL bounds how long one worker may hold a claimed delivery-queue row before
// another may take it. Relays renew within it while a batch is still running.
const ClaimTTL = 5 * time.Minute

const (
	leaseTTL  = 5 * time.Minute
	leasePoll = 200 * time.Millisecond
	// Longer than leaseTTL, so a waiter outlives a holder that died still holding
	// it instead of timing out at the same moment the lease lapses.
	leaseWaitMax = 6 * time.Minute
	// Renew this often relative to the TTL, so a transient database error still
	// leaves two attempts before the lease lapses. Also bounds one renewal: a
	// database that hangs must not park the heartbeat past the expiry it is
	// guarding, because the lease lapses on time regardless.
	leaseRenewDivisor = 3
	// Releasing deliberately ignores the caller's cancellation, so it needs its
	// own bound or an unreachable database strands the goroutine.
	leaseReleaseTimeout = 5 * time.Second
)

// ErrLeaseLost reports that another replica took the lease while fn was still
// running. The work is unfinished and was cut short — it is not a success.
var ErrLeaseLost = errors.New("lease taken by another replica mid-run")

// RunLeased runs fn under a cluster-wide lease, renewing it in the background so
// work longer than ttl keeps its single owner. ran is false when another replica
// already holds the lease: this caller skips the turn rather than waiting.
//
// fn's context is cancelled the moment the lease is lost, and fn must honour it
// — past that point another replica is entitled to start, so continuing is the
// overlap the lease exists to prevent. A lost lease surfaces as ErrLeaseLost.
func RunLeased(ctx context.Context, leases LeaseRepo, name string, ttl time.Duration, fn func(context.Context) error) (ran bool, err error) {
	owner := id.New(id.Holder)
	start := time.Now()
	ok, err := leases.Acquire(ctx, name, owner, ttl)
	if err != nil || !ok {
		return false, err
	}
	// Release is owner-scoped, so this cannot drop a lease someone else has taken.
	defer func() {
		rctx, rcancel := context.WithTimeout(context.WithoutCancel(ctx), leaseReleaseTimeout)
		defer rcancel()
		_ = leases.Release(rctx, name, owner)
	}()

	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	lost := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		renew(runCtx, leases, name, owner, ttl, start.Add(ttl), func() { close(lost); cancel() })
	}()

	ferr := fn(runCtx)
	cancel()
	<-done
	select {
	case <-lost:
		return true, errors.Join(ferr, ErrLeaseLost)
	default:
		return true, ferr
	}
}

// renew heartbeats the lease until the run ends or it is lost, calling onLost
// once. A failed renewal is only fatal past expires: until then the lease is
// still ours and a database blip should not abandon the work. Each attempt is
// bounded, so a hung database cannot hold the heartbeat past the expiry — the
// lease lapses on time whether or not this call ever returns.
func renew(ctx context.Context, leases LeaseRepo, name, owner string, ttl time.Duration, expires time.Time, onLost func()) {
	every := ttl / leaseRenewDivisor
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
		at := time.Now()
		actx, acancel := context.WithTimeout(ctx, every)
		held, err := leases.Acquire(actx, name, owner, ttl)
		acancel()
		switch {
		case err == nil && held:
			expires = at.Add(ttl)
		case err != nil && time.Now().Before(expires):
		default:
			onLost()
			return
		}
	}
}

// RunExclusive runs fn under a cluster-wide lease: never two replicas at once.
// Unlike RunLeased every caller ends up running fn, the losers once the holder
// releases — a holder that FAILED inside fn releases the lease too, so "lease
// gone" cannot be read as "work done". fn must therefore be idempotent and cheap
// when there is nothing left to do; it is the postcondition check, not the lease.
//
// fn takes the run's context, cancelled if the lease is lost, for the same
// reason RunLeased does: work that carries on unleased can overlap its successor.
func RunExclusive(ctx context.Context, leases LeaseRepo, name string, fn func(context.Context) error) error {
	deadline := time.Now().Add(leaseWaitMax)
	for {
		ran, err := RunLeased(ctx, leases, name, leaseTTL, fn)
		if err != nil || ran {
			return err
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
