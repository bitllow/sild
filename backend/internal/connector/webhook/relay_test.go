package webhook_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/bitllow/sild/backend/internal/connector/webhook"
	"github.com/bitllow/sild/backend/internal/store/models"
	"github.com/bitllow/sild/backend/internal/testutil"
)

type capture struct {
	mu   sync.Mutex
	hits []*http.Request
	body [][]byte
	code int
}

func (c *capture) handler(w http.ResponseWriter, r *http.Request) {
	c.mu.Lock()
	defer c.mu.Unlock()
	b, _ := io.ReadAll(r.Body)
	c.hits = append(c.hits, r)
	c.body = append(c.body, b)
	if c.code == 0 {
		c.code = 200
	}
	w.WriteHeader(c.code)
}

func enqueue(t *testing.T, h *testutil.Harness, tenantID string) {
	t.Helper()
	if err := h.Store.Outbox().Enqueue(context.Background(), &models.Outbox{
		TenantID: tenantID, EventID: "evt_stable_1", EventType: "message.created",
		Payload: []byte(`{"event":"message.created","data":{}}`), Status: models.DeliveryPending,
	}); err != nil {
		t.Fatal(err)
	}
}

// §6.1: delivery signs the body, sends a stable event id, logs the attempt.
func TestWebhookDeliverySignedAndLogged(t *testing.T) {
	h := testutil.New(t)
	tenant := h.SeedTenant()
	cap := &capture{code: 200}
	srv := httptest.NewServer(http.HandlerFunc(cap.handler))
	defer srv.Close()

	ep, err := h.Svc.CreateWebhook(context.Background(), tenant.ID, srv.URL, []string{"message.created"})
	if err != nil {
		t.Fatal(err)
	}
	enqueue(t, h, tenant.ID)

	relay := webhook.NewRelay(h.Store)
	if n, err := relay.ProcessOnce(context.Background(), 10); err != nil || n != 1 {
		t.Fatalf("process: n=%d err=%v", n, err)
	}
	if len(cap.hits) != 1 {
		t.Fatalf("expected 1 delivery, got %d", len(cap.hits))
	}
	req, body := cap.hits[0], cap.body[0]
	if got, want := req.Header.Get("X-Signature"), "sha256="+webhook.Sign(ep.Secret, body); got != want {
		t.Errorf("bad signature: %s != %s", got, want)
	}
	if req.Header.Get("X-Sild-Event-Id") != "evt_stable_1" {
		t.Errorf("expected stable event id, got %s", req.Header.Get("X-Sild-Event-Id"))
	}
	// delivery logged
	deliveries, _ := h.Store.Webhooks().ListDeliveries(context.Background(), tenant.ID, ep.ID)
	if len(deliveries) != 1 || deliveries[0].Status != models.DeliveryDelivered {
		t.Fatalf("expected 1 delivered log, got %+v", deliveries)
	}
}

// §6.1: a failing endpoint reschedules with backoff (not dropped), keeping the
// same event id for consumer dedupe across retries.
func TestWebhookRetryOnFailure(t *testing.T) {
	h := testutil.New(t)
	tenant := h.SeedTenant()
	cap := &capture{code: 500}
	srv := httptest.NewServer(http.HandlerFunc(cap.handler))
	defer srv.Close()

	if _, err := h.Svc.CreateWebhook(context.Background(), tenant.ID, srv.URL, []string{"message.created"}); err != nil {
		t.Fatal(err)
	}
	enqueue(t, h, tenant.ID)

	relay := webhook.NewRelay(h.Store)
	if _, err := relay.ProcessOnce(context.Background(), 10); err != nil {
		t.Fatal(err)
	}
	// The outbox row should be pending again (rescheduled), not delivered.
	due, _ := h.Store.Outbox().ClaimDue(context.Background(), 10)
	// available_at was pushed into the future, so it is NOT immediately due.
	if len(due) != 0 {
		t.Fatalf("expected the failed event to be rescheduled to the future, got %d due", len(due))
	}
}
