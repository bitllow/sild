package models

import (
	"time"

	"github.com/bitllow/sild/backend/internal/id"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

// WebhookEndpoint is a registered outbound webhook (§6.1). events[] is a child
// table (portable). secret signs the HMAC.
type WebhookEndpoint struct {
	ID        string `gorm:"primaryKey;size:40"`
	TenantID  string `gorm:"size:40;not null;index"`
	URL       string `gorm:"size:1024;not null"`
	Secret    string `gorm:"size:128;not null"`
	Active    bool   `gorm:"not null;default:true"`
	CreatedAt time.Time

	Events []WebhookEvent `gorm:"foreignKey:EndpointID;references:ID;constraint:OnDelete:CASCADE"`
}

func (w *WebhookEndpoint) BeforeCreate(*gorm.DB) error {
	if w.ID == "" {
		w.ID = id.New(id.Webhook)
	}
	return nil
}

// WebhookEvent is one subscribed event type for an endpoint (replaces text[]).
type WebhookEvent struct {
	EndpointID string `gorm:"primaryKey;size:40"`
	TenantID   string `gorm:"size:40;not null"`
	Event      string `gorm:"primaryKey;size:64"`
}

// Outbox is the transactional outbox (review finding). Domain events are written
// here inside the same tx as the state change; the webhook worker drains it,
// giving at-least-once delivery. EventID is stable across retries (dedupe key).
type Outbox struct {
	ID             string         `gorm:"primaryKey;size:40"`
	TenantID       string         `gorm:"size:40;not null;index:idx_outbox_pending,priority:2"`
	EventID        string         `gorm:"size:64;not null;uniqueIndex"` // X-Sild-Event-Id
	EventType      string         `gorm:"size:64;not null"`
	ConversationID string         `gorm:"size:40;index"`
	Payload        datatypes.JSON `gorm:"type:json;not null"`
	Status         DeliveryStatus `gorm:"size:16;not null;default:'pending';index:idx_outbox_pending,priority:1"`
	Attempts       int            `gorm:"not null;default:0"`
	AvailableAt    time.Time      `gorm:"index"` // next eligible send time (backoff)
	CreatedAt      time.Time
}

func (o *Outbox) BeforeCreate(*gorm.DB) error {
	if o.ID == "" {
		o.ID = id.New(id.Outbox)
	}
	if o.EventID == "" {
		o.EventID = id.New(id.Delivery)
	}
	return nil
}

// WebhookDelivery is the per-attempt delivery log (§6.1, §8). One row per
// (endpoint, event, attempt).
type WebhookDelivery struct {
	ID         string         `gorm:"primaryKey;size:40"`
	TenantID   string         `gorm:"size:40;not null;index"`
	EndpointID string         `gorm:"size:40;not null;index"`
	EventID    string         `gorm:"size:64;not null;index"`
	EventType  string         `gorm:"size:64;not null"`
	Attempt    int            `gorm:"not null"`
	Status     DeliveryStatus `gorm:"size:16;not null"`
	StatusCode int
	Response   string `gorm:"type:text"`
	CreatedAt  time.Time
}

func (d *WebhookDelivery) BeforeCreate(*gorm.DB) error {
	if d.ID == "" {
		d.ID = id.New(id.Delivery)
	}
	return nil
}
