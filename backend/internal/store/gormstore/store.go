package gormstore

import (
	"context"

	"github.com/bitllow/sild/backend/internal/store"
	"gorm.io/gorm"
)

// Store implements store.Store over GORM.
type Store struct {
	db *gorm.DB
}

// New builds a store.Store. dig binds this to the store.Store interface.
func New(db *gorm.DB) store.Store { return &Store{db: db} }

func (s *Store) Health(ctx context.Context) error {
	sqlDB, err := s.db.DB()
	if err != nil {
		return err
	}
	return sqlDB.PingContext(ctx)
}

func (s *Store) Tx(ctx context.Context, fn func(tx store.Store) error) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return fn(&Store{db: tx})
	})
}

func (s *Store) Tenants() store.TenantRepo             { return &tenantRepo{s.db} }
func (s *Store) APIKeys() store.APIKeyRepo             { return &apiKeyRepo{s.db} }
func (s *Store) Admins() store.AdminRepo               { return &adminRepo{s.db} }
func (s *Store) SigningKeys() store.SigningKeyRepo     { return &signingKeyRepo{s.db} }
func (s *Store) Conversations() store.ConversationRepo { return &conversationRepo{s.db} }
func (s *Store) Members() store.MemberRepo             { return &memberRepo{s.db} }
func (s *Store) Assignments() store.AssignmentRepo     { return &assignmentRepo{s.db} }
func (s *Store) Messages() store.MessageRepo           { return &messageRepo{s.db} }
func (s *Store) Receipts() store.ReceiptRepo           { return &receiptRepo{s.db} }
func (s *Store) Uploads() store.UploadRepo             { return &uploadRepo{s.db} }
func (s *Store) PushTokens() store.PushTokenRepo       { return &pushTokenRepo{s.db} }
func (s *Store) Webhooks() store.WebhookRepo           { return &webhookRepo{s.db} }
func (s *Store) Outbox() store.OutboxRepo              { return &outboxRepo{s.db} }
func (s *Store) Email() store.EmailRepo                { return &emailRepo{s.db} }
func (s *Store) Archives() store.ArchiveRepo           { return &archiveRepo{s.db} }

// translateErr maps GORM's not-found to the store sentinel.
func translateErr(err error) error {
	if err == gorm.ErrRecordNotFound {
		return store.ErrNotFound
	}
	return err
}
