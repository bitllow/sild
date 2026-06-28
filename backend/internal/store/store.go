// Package store defines the persistence boundary: repository interfaces the
// domain layer depends on, with no knowledge of GORM or the dialect.
// gormstore provides the concrete implementation, bound by di.
package store

import "context"

// Store is the aggregate root of all repositories. Every repository method takes
// tenantID explicitly so tenant scoping is enforced at the boundary (§1).
type Store interface {
	// Health verifies the database is reachable.
	Health(ctx context.Context) error
	// Tx runs fn inside a single transaction; all-or-nothing (§1 atomic creates).
	// The Store passed to fn is transaction-scoped.
	Tx(ctx context.Context, fn func(tx Store) error) error

	Tenants() TenantRepo
	APIKeys() APIKeyRepo
	Admins() AdminRepo
	SigningKeys() SigningKeyRepo
	Conversations() ConversationRepo
	Members() MemberRepo
	Assignments() AssignmentRepo
	Messages() MessageRepo
	Receipts() ReceiptRepo
	Uploads() UploadRepo
	PushTokens() PushTokenRepo
	Webhooks() WebhookRepo
	Outbox() OutboxRepo
	Email() EmailRepo
	Archives() ArchiveRepo
}
