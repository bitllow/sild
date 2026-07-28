package webhook_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/bitllow/sild/backend/internal/connector/webhook"
	"github.com/bitllow/sild/backend/internal/store/models"
	"github.com/bitllow/sild/backend/internal/testutil"
)

// countByEvent records how many times each event id was POSTed, which is the
// only thing that distinguishes "delivered once" from "delivered by every
// replica".
type countByEvent struct {
	mu     sync.Mutex
	counts map[string]int
}

func (c *countByEvent) handler(w http.ResponseWriter, r *http.Request) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.counts[r.Header.Get("X-Sild-Event-Id")]++
	w.WriteHeader(http.StatusOK)
}

// The reason the outbox needed a claim: two workers draining one queue must not
// each deliver every event. Consumers dedupe on X-Sild-Event-Id, which protects
// the consumer — it does not stop us from sending the same event twice.
func TestConcurrentRelaysDeliverEachEventOnce(t *testing.T) {
	h := testutil.New(t)
	ctx := context.Background()
	tenant := h.SeedTenant()

	seen := &countByEvent{counts: map[string]int{}}
	srv := httptest.NewServer(http.HandlerFunc(seen.handler))
	defer srv.Close()

	if _, err := h.Svc.CreateWebhook(ctx, tenant.ID, srv.URL, []string{"message.created"}); err != nil {
		t.Fatal(err)
	}
	const events = 12
	for i := 0; i < events; i++ {
		if err := h.Store.Outbox().Enqueue(ctx, &models.Outbox{
			TenantID: tenant.ID, EventType: "message.created",
			Payload: []byte(`{"event":"message.created","data":{}}`), Status: models.DeliveryPending,
		}); err != nil {
			t.Fatal(err)
		}
	}

	var wg sync.WaitGroup
	for i := 0; i < 3; i++ { // three worker replicas, one outbox
		wg.Add(1)
		go func() {
			defer wg.Done()
			relay := webhook.NewRelay(h.Store)
			_, _ = relay.ProcessOnce(ctx, events)
		}()
	}
	wg.Wait()

	seen.mu.Lock()
	defer seen.mu.Unlock()
	if len(seen.counts) != events {
		t.Fatalf("delivered %d distinct events, want %d", len(seen.counts), events)
	}
	for eventID, n := range seen.counts {
		if n != 1 {
			t.Fatalf("event %s delivered %d times by concurrent relays, want 1", eventID, n)
		}
	}
}
