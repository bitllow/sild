package gormstore_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/bitllow/sild/backend/internal/config"
	"github.com/bitllow/sild/backend/internal/id"
	"github.com/bitllow/sild/backend/internal/store"
	"github.com/bitllow/sild/backend/internal/store/models"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

// seedOutbox clears the queue first: ClaimDue drains the whole outbox, not one
// tenant's, and the dialect databases keep rows between runs.
func seedOutbox(t *testing.T, st store.Store, db *gorm.DB, n int) {
	t.Helper()
	if err := db.Where("1 = 1").Delete(&models.Outbox{}).Error; err != nil {
		t.Fatalf("reset outbox: %v", err)
	}
	tenantID := id.New(id.Tenant)
	for i := 0; i < n; i++ {
		ev := &models.Outbox{
			TenantID: tenantID, EventType: "message.created",
			Payload: datatypes.JSON(`{}`), Status: models.DeliveryPending,
		}
		if err := st.Outbox().Enqueue(context.Background(), ev); err != nil {
			t.Fatalf("enqueue: %v", err)
		}
	}
}

// The whole point of the claim: a second relay looking at the same outbox gets
// nothing, so no webhook is delivered twice.
func TestClaimedEventsAreNotHandedOutTwice(t *testing.T) {
	for _, dbc := range dialects(t) {
		t.Run(string(dbc.Driver), func(t *testing.T) {
			ctx := context.Background()
			st, db := storeFor(t, dbc)
			seedOutbox(t, st, db, 5)

			first, _, err := st.Outbox().ClaimDue(ctx, 10)
			if err != nil {
				t.Fatalf("first claim: %v", err)
			}
			if len(first) != 5 {
				t.Fatalf("first relay claimed %d events, want 5", len(first))
			}
			second, _, err := st.Outbox().ClaimDue(ctx, 10)
			if err != nil {
				t.Fatalf("second claim: %v", err)
			}
			if len(second) != 0 {
				t.Fatalf("second relay claimed %d events already held by the first", len(second))
			}
		})
	}
}

// Two relays claiming at the same moment must partition the queue between them,
// never overlap it.
func TestConcurrentRelaysPartitionTheOutbox(t *testing.T) {
	for _, dbc := range dialects(t) {
		t.Run(string(dbc.Driver), func(t *testing.T) {
			ctx := context.Background()
			st, db := storeFor(t, dbc)
			seedOutbox(t, st, db, 20)

			var mu sync.Mutex
			seen := map[string]int{}
			var wg sync.WaitGroup
			for i := 0; i < 4; i++ {
				wg.Add(1)
				go func() {
					defer wg.Done()
					events, _, err := st.Outbox().ClaimDue(ctx, 20)
					if err != nil {
						return
					}
					mu.Lock()
					defer mu.Unlock()
					for _, ev := range events {
						seen[ev.ID]++
					}
				}()
			}
			wg.Wait()

			for id, n := range seen {
				if n > 1 {
					t.Fatalf("event %s claimed by %d relays at once", id, n)
				}
			}
		})
	}
}

// Partitioning is not enough on its own: a relay that picks candidates another
// relay is already claiming loses its whole page and idles until the next tick,
// so webhook throughput stops improving as workers are added. Row locks are what
// make the pages disjoint, so this is a postgres/mysql guarantee only.
func TestConcurrentRelaysEachGetAPage(t *testing.T) {
	for _, dbc := range dialects(t) {
		if dbc.Driver == config.SQLite {
			continue // no row locks; the single writer serializes the claim
		}
		t.Run(string(dbc.Driver), func(t *testing.T) {
			ctx := context.Background()
			st, db := storeFor(t, dbc)
			const relays, page = 4, 5
			seedOutbox(t, st, db, relays*page*2)

			counts := make([]int, relays)
			errs := make([]error, relays)
			var wg sync.WaitGroup
			for i := 0; i < relays; i++ {
				wg.Add(1)
				go func() {
					defer wg.Done()
					events, _, err := st.Outbox().ClaimDue(ctx, page)
					counts[i], errs[i] = len(events), err
				}()
			}
			wg.Wait()

			for i, err := range errs {
				if err != nil {
					t.Fatalf("relay %d: %v", i, err)
				}
				if counts[i] != page {
					t.Fatalf("relay %d claimed %d of a %d-event page while the queue still had work", i, counts[i], page)
				}
			}
		})
	}
}

// A relay that dies mid-delivery must not strand its events: the claim expires
// and the next pass picks them up.
func TestExpiredClaimsAreRequeued(t *testing.T) {
	ctx := context.Background()
	st, db := sqliteStore(t)
	seedOutbox(t, st, db, 3)

	claimed, _, err := st.Outbox().ClaimDue(ctx, 10)
	if err != nil || len(claimed) != 3 {
		t.Fatalf("claim: %d %v", len(claimed), err)
	}
	// Nothing is claimable while the lock is live.
	if live, _, _ := st.Outbox().ClaimDue(ctx, 10); len(live) != 0 {
		t.Fatalf("another relay claimed %d live rows", len(live))
	}

	// The holder dies: its lock lapses with the rows still claimed.
	if err := db.Model(&models.Outbox{}).Where("locked_until IS NOT NULL").
		Update("locked_until", time.Now().Add(-time.Hour)).Error; err != nil {
		t.Fatalf("age claims: %v", err)
	}

	again, _, err := st.Outbox().ClaimDue(ctx, 10)
	if err != nil || len(again) != 3 {
		t.Fatalf("events stranded by a dead relay were not reclaimable: %d %v", len(again), err)
	}
}

// Rescheduling after a failed delivery returns the event to the queue, so the
// retry is not blocked by the claim that failed.
func TestRescheduleReleasesTheClaim(t *testing.T) {
	ctx := context.Background()
	st, db := sqliteStore(t)
	seedOutbox(t, st, db, 1)

	claimed, _, err := st.Outbox().ClaimDue(ctx, 10)
	if err != nil || len(claimed) != 1 {
		t.Fatalf("claim: %d %v", len(claimed), err)
	}
	if err := st.Outbox().Reschedule(ctx, claimed[0].ID, 1, 0); err != nil {
		t.Fatalf("reschedule: %v", err)
	}
	retry, _, err := st.Outbox().ClaimDue(ctx, 10)
	if err != nil || len(retry) != 1 {
		t.Fatalf("rescheduled event not claimable again: %d %v", len(retry), err)
	}
}

// A delivery pass can outlive its claim: a hundred events against endpoints that
// each burn the HTTP timeout dwarfs the lock. Renewal is what keeps the batch
// owned while it is still being delivered.
func TestClaimRenewalKeepsTheBatchOwned(t *testing.T) {
	ctx := context.Background()
	st, db := sqliteStore(t)
	seedOutbox(t, st, db, 1)

	claimed, token, err := st.Outbox().ClaimDue(ctx, 10)
	if err != nil || len(claimed) != 1 {
		t.Fatalf("claim: %d %v", len(claimed), err)
	}

	// The pass runs long enough that the original lock would have lapsed.
	if err := db.Model(&models.Outbox{}).Where("claim_token = ?", token).
		Update("locked_until", time.Now().Add(-time.Minute)).Error; err != nil {
		t.Fatalf("age lock: %v", err)
	}
	ours, err := st.Outbox().RenewClaim(ctx, claimed[0].ID, token)
	if err != nil || !ours {
		t.Fatalf("renew: ours=%v err=%v", ours, err)
	}
	// Renewed, so another relay must not be able to take it.
	if other, _, _ := st.Outbox().ClaimDue(ctx, 10); len(other) != 0 {
		t.Fatalf("another relay claimed %d renewed events", len(other))
	}
}

// Renewal is also the ownership check: once another relay has taken the row, the
// original must stop delivering it rather than send the event twice.
func TestRenewalReportsALostClaim(t *testing.T) {
	ctx := context.Background()
	st, db := sqliteStore(t)
	seedOutbox(t, st, db, 1)

	first, token, err := st.Outbox().ClaimDue(ctx, 10)
	if err != nil || len(first) != 1 {
		t.Fatalf("claim: %d %v", len(first), err)
	}

	// The holder stalls past its lock and a second relay takes the row.
	if err := db.Model(&models.Outbox{}).Where("claim_token = ?", token).
		Update("locked_until", time.Now().Add(-time.Minute)).Error; err != nil {
		t.Fatalf("age lock: %v", err)
	}
	stolen, _, err := st.Outbox().ClaimDue(ctx, 10)
	if err != nil || len(stolen) != 1 {
		t.Fatalf("takeover: %d %v", len(stolen), err)
	}
	ours, err := st.Outbox().RenewClaim(ctx, first[0].ID, token)
	if err != nil {
		t.Fatalf("renew: %v", err)
	}
	if ours {
		t.Fatal("the original relay still believes it owns a row another relay took")
	}
}
