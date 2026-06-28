package domain

import (
	"context"
	"encoding/json"
	"time"

	"github.com/bitllow/sild/backend/internal/archive"
	"github.com/bitllow/sild/backend/internal/auth"
	"github.com/bitllow/sild/backend/internal/config"
	"github.com/bitllow/sild/backend/internal/mail"
	"github.com/bitllow/sild/backend/internal/realtime"
	"github.com/bitllow/sild/backend/internal/storage"
	"github.com/bitllow/sild/backend/internal/store"
	"github.com/bitllow/sild/backend/internal/store/models"
)

// Service is the use-case aggregate. dig provides it to the handlers.
type Service struct {
	store    store.Store
	pub      realtime.Publisher
	keys     *auth.KeyManager
	bucket   storage.Bucket
	mailer   mail.Mailer
	verifier mail.SignatureVerifier
	sink     archive.Sink // cold-storage read fallback (§12)
	cfg      *config.Config
	now      func() time.Time
}

// New constructs the domain service.
func New(st store.Store, pub realtime.Publisher, km *auth.KeyManager, bucket storage.Bucket, mailer mail.Mailer, sink archive.Sink, cfg *config.Config) *Service {
	if mailer == nil {
		mailer = mail.NoopMailer{}
	}
	return &Service{store: st, pub: pub, keys: km, bucket: bucket, mailer: mailer, verifier: mail.HMACVerifier{}, sink: sink, cfg: cfg, now: time.Now}
}

// SetClock overrides the service clock (tests).
func (s *Service) SetClock(fn func() time.Time) { s.now = fn }

// emit publishes a realtime envelope after a successful commit. Best-effort:
// failures are swallowed (reconnect catch-up is the correctness mechanism, §5.4).
func (s *Service) emit(ctx context.Context, t realtime.Target, eventType, convID string, data any) {
	_ = s.pub.Publish(ctx, t, realtime.Envelope{
		Type:           eventType,
		ConversationID: convID,
		Data:           data,
		Ts:             s.now().Unix(),
	})
}

// enqueueWebhook writes a webhook event to the outbox INSIDE the caller's tx, so
// it commits atomically with the state change (at-least-once, §6.1).
func (s *Service) enqueueWebhook(ctx context.Context, tx store.Store, tenantID, convID, eventType string, data any) error {
	body, err := json.Marshal(map[string]any{
		"event":     eventType,
		"tenant_id": tenantID,
		"ts":        s.now().Unix(),
		"data":      data,
	})
	if err != nil {
		return err
	}
	return tx.Outbox().Enqueue(ctx, &models.Outbox{
		TenantID:       tenantID,
		EventType:      eventType,
		ConversationID: convID,
		Payload:        body,
		Status:         models.DeliveryPending,
		AvailableAt:    s.now(),
	})
}

// searchText materializes member_search_text from the tenant's searchable keys
// (§3): concat the values of those keys from the member's metadata.
func (s *Service) searchText(ctx context.Context, tenantID string, metadata []byte) (string, error) {
	keys, err := s.store.Tenants().SearchableKeys(ctx, tenantID)
	if err != nil || len(keys) == 0 || len(metadata) == 0 {
		return "", err
	}
	var m map[string]any
	if err := json.Unmarshal(metadata, &m); err != nil {
		return "", nil // opaque metadata; skip on parse failure
	}
	var out string
	for _, k := range keys {
		if v, ok := m[k]; ok {
			if out != "" {
				out += " "
			}
			out += toString(v)
		}
	}
	return out, nil
}

func toString(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case json.Number:
		return t.String()
	default:
		b, _ := json.Marshal(v)
		return string(b)
	}
}
