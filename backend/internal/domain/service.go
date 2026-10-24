package domain

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/bitllow/sild/backend/internal/archive"
	"github.com/bitllow/sild/backend/internal/auth"
	"github.com/bitllow/sild/backend/internal/config"
	"github.com/bitllow/sild/backend/internal/mail"
	"github.com/bitllow/sild/backend/internal/push"
	"github.com/bitllow/sild/backend/internal/realtime"
	"github.com/bitllow/sild/backend/internal/secrets"
	"github.com/bitllow/sild/backend/internal/storage"
	"github.com/bitllow/sild/backend/internal/store"
	"github.com/bitllow/sild/backend/internal/store/models"
)

// Service is the use-case aggregate. dig provides it to the handlers.
type Service struct {
	store     store.Store
	pub       realtime.Publisher
	keys      *auth.KeyManager
	bucket    storage.Bucket
	mailer    mail.Mailer
	verifier  mail.SignatureVerifier
	sink      archive.Sink // cold-storage read fallback (§12)
	notifier  push.Notifier
	secrets   *secrets.Box
	validator *auth.Validator
	cfg       *config.Config
	now       func() time.Time
	// search backs the ?q= form of ListConversations. Set after construction:
	// SearchService is built from the same store, so wiring it through New would
	// make the two dig providers mutually dependent.
	search *SearchService
}

// UseSearch attaches the search service. dig calls it once at startup.
func (s *Service) UseSearch(ss *SearchService) { s.search = ss }

// New constructs the domain service.
func New(st store.Store, pub realtime.Publisher, km *auth.KeyManager, bucket storage.Bucket, mailer mail.Mailer, sink archive.Sink, notifier push.Notifier, box *secrets.Box, cfg *config.Config) *Service {
	if mailer == nil {
		mailer = mail.NoopMailer{}
	}
	return &Service{store: st, pub: pub, keys: km, bucket: bucket, mailer: mailer, verifier: mail.HMACVerifier{}, sink: sink, notifier: notifier, secrets: box, validator: auth.NewValidator(auth.ValidatorTimeout), cfg: cfg, now: time.Now}
}

// SetClock overrides the service clock (tests).
func (s *Service) SetClock(fn func() time.Time) { s.now = fn }

// SetValidatorTimeout overrides how long an identity validator may take (tests).
func (s *Service) SetValidatorTimeout(d time.Duration) { s.validator = auth.NewValidator(d) }

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

// operators adds the channel of the operators who work conv's queue to t.
func operators(t realtime.Target, conv *models.Conversation) realtime.Target {
	if conv.Queue == models.QueueSupport {
		t.Tenant = conv.TenantID
	} else {
		t.Queue = realtime.QueueRef{Tenant: conv.TenantID, Key: conv.Queue}
	}
	return t
}

// emitObserved emits to t plus the tenant channel watched by the operators who
// may read this conversation, so an agent inbox sees background conversations
// without subscribing to each one (§5.1). Which channel that is comes from
// ClassifyAgentAccess — the same decision REST authorizes with, so the socket
// cannot fan out what an endpoint refuses.
//
// conv may be nil when the caller has not read the row; it is read here then.
func (s *Service) emitObserved(ctx context.Context, t realtime.Target, tenantID string, conv *models.Conversation, eventType string, data any) {
	if conv == nil {
		conv, _ = s.store.Conversations().Get(ctx, tenantID, t.Conversation)
	}
	if conv != nil {
		switch access := s.classifyConv(ctx, tenantID, conv, true); {
		case access.Peer:
			t.Peer = tenantID
		case access.SupportOK:
			t = operators(t, conv)
		}
	}
	s.emit(ctx, t, eventType, t.Conversation, data)
}

// enqueueWebhook writes a webhook event to the outbox INSIDE the caller's tx, so
// it commits atomically with the state change (at-least-once, §6.1). The queue
// gate reads the conversation through tx, so one created in the same tx is seen.
func (s *Service) enqueueWebhook(ctx context.Context, tx store.Store, tenantID, convID, eventType string, data any) error {
	if convID != "" {
		if conv, err := tx.Conversations().Get(ctx, tenantID, convID); err == nil && !sendsWebhooks(convQueue(ctx, tx, conv)) {
			return nil
		}
	}
	return s.writeWebhook(ctx, tx, tenantID, convID, eventType, data)
}

// fireConvWebhook enqueues a webhook event for a conversation the caller has
// already read, outside a caller tx.
func (s *Service) fireConvWebhook(ctx context.Context, conv *models.Conversation, eventType string, data any) error {
	if !sendsWebhooks(convQueue(ctx, s.store, conv)) {
		return nil
	}
	return s.writeWebhook(ctx, s.store, conv.TenantID, conv.ID, eventType, data)
}

// writeWebhook writes the outbox row; the caller has applied the queue gate.
func (s *Service) writeWebhook(ctx context.Context, tx store.Store, tenantID, convID, eventType string, data any) error {
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

// searchText materializes contacts.search_text from the tenant's searchable keys
// (§3): concat the values of those keys from the stored profile.
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

// QueueSettings returns a queue's settings. The support queue is built in.
func (s *Service) QueueSettings(ctx context.Context, tenantID, key string) (*models.Queue, error) {
	if key == "" || key == models.QueueSupport {
		return supportQueue(tenantID), nil
	}
	q, err := s.store.Queues().Get(ctx, tenantID, key)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, invalid("unknown queue " + key)
		}
		return nil, err
	}
	return q, nil
}

func supportQueue(tenantID string) *models.Queue {
	return &models.Queue{TenantID: tenantID, Key: models.QueueSupport, Webhooks: true}
}

// convQueue reads conv's queue settings through st; nil when they cannot be read.
func convQueue(ctx context.Context, st store.Store, conv *models.Conversation) *models.Queue {
	if conv.Queue == models.QueueSupport {
		return supportQueue(conv.TenantID)
	}
	q, err := st.Queues().Get(ctx, conv.TenantID, conv.Queue)
	if err != nil {
		return nil
	}
	return q
}

// sendsWebhooks reports whether a queue sends tenant webhooks; settings that
// could not be read (nil) keep sending.
func sendsWebhooks(q *models.Queue) bool { return q == nil || q.Webhooks }

// SetQueue creates a tenant-defined queue or replaces its settings.
func (s *Service) SetQueue(ctx context.Context, q *models.Queue) error {
	if !models.ValidQueueKey(q.Key) {
		return invalid("queue key must be lowercase letters, digits, - or _")
	}
	if q.Key == models.QueueSupport {
		return invalid("the support queue is built in")
	}
	if _, err := s.store.Tenants().Get(ctx, q.TenantID); err != nil {
		return mapStoreErr(err)
	}
	q.CreatedAt = s.now()
	return s.store.Queues().Upsert(ctx, q)
}

// ListQueues returns the tenant-defined queues; the built-in support queue is not among them.
func (s *Service) ListQueues(ctx context.Context, tenantID string) ([]models.Queue, error) {
	return s.store.Queues().List(ctx, tenantID)
}
