// Package webhook delivers outbox events to registered endpoints (§6.1) with
// HMAC signing, a stable event id for consumer dedupe, a per-attempt delivery
// log, and exponential backoff retries.
package webhook

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"time"

	"github.com/bitllow/sild/backend/internal/store"
	"github.com/bitllow/sild/backend/internal/store/models"
)

// backoffSchedule is the retry delay (seconds) per attempt (§6.1: 1m,5m,30m,2h,6h).
var backoffSchedule = []int{60, 300, 1800, 7200, 21600}

// Relay drains the outbox and delivers to subscribed endpoints.
type Relay struct {
	store  store.Store
	client *http.Client
}

// NewRelay constructs the relay. dig provides it for sild-worker.
func NewRelay(st store.Store) *Relay {
	return &Relay{store: st, client: &http.Client{Timeout: 10 * time.Second}}
}

// ProcessOnce delivers up to `limit` due events. Returns the number processed.
func (r *Relay) ProcessOnce(ctx context.Context, limit int) (int, error) {
	events, err := r.store.Outbox().ClaimDue(ctx, limit)
	if err != nil {
		return 0, err
	}
	for i := range events {
		r.deliver(ctx, &events[i])
	}
	return len(events), nil
}

// deliver attempts every subscribed endpoint for one event. On full success the
// outbox row is marked delivered; on any failure it is rescheduled (or failed
// after the schedule is exhausted). Consumers dedupe on X-Sild-Event-Id.
func (r *Relay) deliver(ctx context.Context, ev *models.Outbox) {
	endpoints, err := r.store.Webhooks().ListForEvent(ctx, ev.TenantID, ev.EventType)
	if err != nil {
		r.reschedule(ctx, ev)
		return
	}
	if len(endpoints) == 0 {
		_ = r.store.Outbox().MarkDelivered(ctx, ev.ID) // nothing subscribed
		return
	}
	allOK := true
	for i := range endpoints {
		ep := &endpoints[i]
		code, derr := r.post(ctx, ep, ev.Payload, ev.EventID)
		status := models.DeliveryDelivered
		if derr != nil || code < 200 || code >= 300 {
			status = models.DeliveryFailed
			allOK = false
		}
		_ = r.store.Webhooks().LogDelivery(ctx, &models.WebhookDelivery{
			TenantID: ev.TenantID, EndpointID: ep.ID, EventID: ev.EventID,
			EventType: ev.EventType, Attempt: ev.Attempts + 1, Status: status,
			StatusCode: code, CreatedAt: time.Now(),
		})
	}
	if allOK {
		_ = r.store.Outbox().MarkDelivered(ctx, ev.ID)
		return
	}
	r.reschedule(ctx, ev)
}

func (r *Relay) reschedule(ctx context.Context, ev *models.Outbox) {
	attempts := ev.Attempts + 1
	if attempts >= len(backoffSchedule) {
		_ = r.store.Outbox().MarkFailed(ctx, ev.ID)
		return
	}
	_ = r.store.Outbox().Reschedule(ctx, ev.ID, attempts, backoffSchedule[attempts])
}

func (r *Relay) post(ctx context.Context, ep *models.WebhookEndpoint, body []byte, eventID string) (int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, ep.URL, bytes.NewReader(body))
	if err != nil {
		return 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Signature", "sha256="+sign(ep.Secret, body))
	req.Header.Set("X-Sild-Event-Id", eventID)
	resp, err := r.client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	return resp.StatusCode, nil
}

// sign computes the HMAC-SHA256 of the raw body (§6.1).
func sign(secret string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return hex.EncodeToString(mac.Sum(nil))
}

// Sign is exported for tests/consumers to verify signatures.
func Sign(secret string, body []byte) string { return sign(secret, body) }
