package search

import (
	"context"

	"github.com/bitllow/sild/backend/internal/config"
	"github.com/bitllow/sild/backend/internal/store/gormstore"
	"gorm.io/gorm"
)

// ConversationHit is a single search result.
type ConversationHit struct {
	ConversationID string  `json:"conversation_id"`
	Snippet        string  `json:"snippet,omitempty"`
	Score          float64 `json:"score,omitempty"`
}

// Results is the search response (§4.3).
type Results struct {
	Conversations []ConversationHit `json:"conversations"`
}

// Backend runs a parsed Query against hot data. Postgres uses trigram similarity
// ranking; other dialects use a portable LIKE fallback (no ranking).
type Backend interface {
	Search(ctx context.Context, tenantID string, q Query) (Results, error)
}

// New selects the backend for the active dialect. dig provides the result.
func New(db *gorm.DB) Backend {
	switch gormstore.Dialect(db) {
	case config.Postgres:
		return &postgresBackend{db: db}
	default:
		return &portableBackend{db: db}
	}
}
