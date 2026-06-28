// Package archive moves closed, idle conversations from Postgres to a pluggable
// cold sink (§12): write-then-delete, verified. BigQuery (queryable) and
// gcs_json/s3_json (JSON blobs) implement one Sink interface.
package archive

import (
	"context"
	"fmt"

	"github.com/bitllow/sild/backend/internal/config"
)

// SerializedConversation is the whole conversation flattened for the sink: row +
// members + messages + attachment manifest (object_keys, not bytes) (§12).
type SerializedConversation struct {
	ConversationID string           `json:"conversation_id"`
	TenantID       string           `json:"tenant_id"`
	Reference      string           `json:"reference"`
	Metadata       any              `json:"metadata,omitempty"`
	Status         string           `json:"status"`
	Members        []map[string]any `json:"members"`
	Messages       []map[string]any `json:"messages"`
	MessageCount   int              `json:"message_count"`
}

// Sink persists and rehydrates archived conversations (§12).
type Sink interface {
	// Write durably persists a conversation and returns a locator (sink_ref).
	Write(ctx context.Context, c SerializedConversation) (sinkRef string, err error)
	// Read rehydrates a conversation for fallback reads / restore.
	Read(ctx context.Context, sinkRef string) (SerializedConversation, error)
	// Name reports the sink kind for the tombstone (bigquery|gcs_json|s3_json).
	Name() string
}

// New selects the configured sink. dig provides it.
func New(cfg *config.Config) (Sink, error) {
	switch cfg.Archive.Sink {
	case "gcs_json", "s3_json", "":
		return newJSONSink(cfg)
	case "bigquery":
		return newBigQuerySink(cfg)
	default:
		return nil, fmt.Errorf("unknown ARCHIVE_SINK %q", cfg.Archive.Sink)
	}
}
