package gormstore_test

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/bitllow/sild/backend/internal/id"
	"github.com/bitllow/sild/backend/internal/store"
	"github.com/bitllow/sild/backend/internal/store/models"
)

// seedAssignment makes a queued assignment with fresh ids — the dialect DBs
// persist between runs, so fixed ids would collide on the second one.
func seedAssignment(t *testing.T, st store.Store) (tenantID string, a *models.Assignment) {
	t.Helper()
	ctx := context.Background()
	tenantID = id.New(id.Tenant)
	convID := id.New(id.Conversation)
	conv := &models.Conversation{ID: convID, TenantID: tenantID, Status: models.ConversationOpen, Kind: models.KindSupport}
	if err := st.Conversations().Create(ctx, conv); err != nil {
		t.Fatalf("seed conversation: %v", err)
	}
	a = &models.Assignment{TenantID: tenantID, ConversationID: convID, Status: models.AssignmentQueued}
	if err := st.Assignments().Create(ctx, a); err != nil {
		t.Fatalf("seed assignment: %v", err)
	}
	return tenantID, a
}

func claimBy(actor string) store.AssignmentTransition {
	return store.AssignmentTransition{
		From: []models.AssignmentStatus{models.AssignmentQueued},
		To:   models.AssignmentAssigned, Assignee: &actor,
	}
}

// The guard is the fix: the expected status travels with the write, so the
// second claim changes nothing and says so.
func TestTransitionRejectsWhenStatusMovedOn(t *testing.T) {
	for _, dbc := range dialects(t) {
		t.Run(string(dbc.Driver), func(t *testing.T) {
			ctx := context.Background()
			st, _ := storeFor(t, dbc)
			tenant, a := seedAssignment(t, st)

			won, err := st.Assignments().Transition(ctx, tenant, a.ID, claimBy("agent-a"))
			if err != nil || !won {
				t.Fatalf("first claim: ok=%v err=%v", won, err)
			}
			lost, err := st.Assignments().Transition(ctx, tenant, a.ID, claimBy("agent-b"))
			if err != nil {
				t.Fatalf("second claim: %v", err)
			}
			if lost {
				t.Fatal("second claim reported success on an already-claimed assignment")
			}

			got, err := st.Assignments().Get(ctx, tenant, a.ID)
			if err != nil {
				t.Fatalf("get: %v", err)
			}
			if got.AssigneeActorID == nil || *got.AssigneeActorID != "agent-a" {
				t.Fatalf("assignee is %v, want agent-a — the loser overwrote the winner", got.AssigneeActorID)
			}
		})
	}
}

// The race the guard exists for: agents claiming the same queued assignment at
// the same moment. Exactly one may win, whatever the interleaving.
func TestConcurrentClaimsHaveOneWinner(t *testing.T) {
	for _, dbc := range dialects(t) {
		t.Run(string(dbc.Driver), func(t *testing.T) {
			ctx := context.Background()
			st, _ := storeFor(t, dbc)
			tenant, a := seedAssignment(t, st)

			const agents = 8
			var winners atomic.Int32
			var wg sync.WaitGroup
			for i := 0; i < agents; i++ {
				wg.Add(1)
				go func(i int) {
					defer wg.Done()
					ok, err := st.Assignments().Transition(ctx, tenant, a.ID, claimBy("agent"))
					if err == nil && ok {
						winners.Add(1)
					}
				}(i)
			}
			wg.Wait()

			if got := winners.Load(); got != 1 {
				t.Fatalf("%d of %d agents were told they claimed the same assignment, want 1", got, agents)
			}
		})
	}
}
