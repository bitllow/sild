package models

import (
	"regexp"
	"time"
)

// QueueSupport is the built-in queue every support conversation joins unless
// it names another. It has no settings row and cannot be changed.
const QueueSupport = "support"

var queueKeyPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,31}$`)

// ValidQueueKey reports whether a key may name a queue. Keys reach realtime
// channel names, so the alphabet is closed.
func ValidQueueKey(key string) bool { return queueKeyPattern.MatchString(key) }

// Queue is one tenant-defined queue beside the built-in support queue.
type Queue struct {
	TenantID string `gorm:"primaryKey;size:40"`
	Key      string `gorm:"column:queue_key;primaryKey;size:32"`
	Label    string `gorm:"size:120"`
	// RequeueOnReply re-queues an open conversation whose assignment was closed
	// when its client writes again.
	RequeueOnReply bool `gorm:"not null;default:false"`
	// Webhooks controls whether this queue's conversation events reach the
	// tenant's webhook endpoints.
	Webhooks  bool `gorm:"not null"`
	CreatedAt time.Time
}
