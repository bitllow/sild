package store

import (
	"context"
	"errors"

	"github.com/bitllow/sild/backend/internal/store/models"
)

// ErrNotFound is returned by repositories when a row does not exist.
var ErrNotFound = errors.New("not found")

// Participant identifies a member/sender by namespace (exactly one id set).
type Participant struct {
	Kind            models.MemberKind
	ExternalUserID  *string
	InternalActorID *string
}

type TenantRepo interface {
	Create(ctx context.Context, t *models.Tenant) error
	Get(ctx context.Context, id string) (*models.Tenant, error)
	AllIDs(ctx context.Context) ([]string, error)
	SearchableKeys(ctx context.Context, tenantID string) ([]string, error)
	SetSearchableKeys(ctx context.Context, tenantID string, keys []string) error
	GetEmailConfig(ctx context.Context, tenantID string) (*models.TenantEmailConfig, error)
	SetEmailConfig(ctx context.Context, cfg *models.TenantEmailConfig) error
	FindByInboundDomain(ctx context.Context, domain string) (*models.TenantEmailConfig, error)
}

type APIKeyRepo interface {
	Create(ctx context.Context, k *models.APIKey) error
	FindByPrefix(ctx context.Context, prefix string) (*models.APIKey, error)
	ListByTenant(ctx context.Context, tenantID string) ([]models.APIKey, error)
	Revoke(ctx context.Context, tenantID, id string) error
}

type AdminRepo interface {
	Create(ctx context.Context, a *models.AdminUser) error
	Get(ctx context.Context, tenantID, id string) (*models.AdminUser, error)
	FindByEmail(ctx context.Context, email string) ([]models.AdminUser, error)
	List(ctx context.Context, tenantID string) ([]models.AdminUser, error)
	SetPassword(ctx context.Context, tenantID, id, passwordHash string) error
	CreateSession(ctx context.Context, s *models.AdminSession) error
	GetSession(ctx context.Context, id string) (*models.AdminSession, error)
	DeleteSession(ctx context.Context, id string) error
}

type SigningKeyRepo interface {
	Create(ctx context.Context, k *models.SigningKey) error
	Active(ctx context.Context) (*models.SigningKey, error)
	GetByKid(ctx context.Context, kid string) (*models.SigningKey, error)
	Published(ctx context.Context) ([]models.SigningKey, error) // active + not-yet-retired, for JWKS
}

type ConversationRepo interface {
	Create(ctx context.Context, c *models.Conversation) error
	Get(ctx context.Context, tenantID, id string) (*models.Conversation, error)
	UpdateStatus(ctx context.Context, tenantID, id string, status models.ConversationStatus) error
	ListForUser(ctx context.Context, tenantID, externalUserID string) ([]models.Conversation, error)
	ListArchivable(ctx context.Context, tenantID string, idleBeforeMsgID string, limit int) ([]models.Conversation, error)
}

type MemberRepo interface {
	Add(ctx context.Context, m *models.ConversationMember) error
	RemoveExternal(ctx context.Context, tenantID, convID, externalUserID string) error
	Get(ctx context.Context, tenantID, convID, externalUserID string) (*models.ConversationMember, error)
	IsActiveMember(ctx context.Context, tenantID, convID, externalUserID string) (bool, error)
	ListActive(ctx context.Context, tenantID, convID string) ([]models.ConversationMember, error)
	ListActiveForUser(ctx context.Context, tenantID, externalUserID string) ([]models.ConversationMember, error)
	CountActive(ctx context.Context, tenantID, convID string) (int, error)
	Remap(ctx context.Context, tenantID, convID, fromExternalID, toExternalID string) error
	UpdateSearchText(ctx context.Context, tenantID, memberID, text string) error
}

type AssignmentRepo interface {
	Create(ctx context.Context, a *models.Assignment) error
	Get(ctx context.Context, tenantID, id string) (*models.Assignment, error)
	GetByConversation(ctx context.Context, tenantID, convID string) (*models.Assignment, error)
	Update(ctx context.Context, a *models.Assignment) error
	ListQueue(ctx context.Context, tenantID string, status *models.AssignmentStatus, assigneeActorID *string) ([]models.Assignment, error)
}

// MessagePage is a page of history with a has-more flag.
type MessagePage struct {
	Messages []models.Message
	HasMore  bool
}

type MessageRepo interface {
	Create(ctx context.Context, m *models.Message) error
	Get(ctx context.Context, tenantID, id string) (*models.Message, error)
	FindByClientMsgID(ctx context.Context, tenantID, convID, clientMsgID string) (*models.Message, error)
	ListBefore(ctx context.Context, tenantID, convID, before string, limit int, includeInternal bool) (*MessagePage, error)
	ListAfter(ctx context.Context, tenantID, convID, after string, includeInternal bool) ([]models.Message, error)
	Last(ctx context.Context, tenantID, convID string, includeInternal bool) (*models.Message, error)
	UnreadCount(ctx context.Context, tenantID, convID, lastReadMessageID string, includeInternal bool) (int, error)
}

type ReceiptRepo interface {
	Upsert(ctx context.Context, r *models.ReadReceipt) error // monotonic
	Get(ctx context.Context, tenantID, convID string, p Participant) (*models.ReadReceipt, error)
}

type UploadRepo interface {
	Create(ctx context.Context, u *models.Upload) error
	GetByObjectKey(ctx context.Context, tenantID, objectKey string) (*models.Upload, error)
	MarkCompleted(ctx context.Context, tenantID, objectKey string) error
}

type PushTokenRepo interface {
	Upsert(ctx context.Context, t *models.PushToken) error
	DeleteByToken(ctx context.Context, tenantID, token string, owner Participant) error
	ListForUser(ctx context.Context, tenantID, externalUserID string) ([]models.PushToken, error)
}

type WebhookRepo interface {
	Create(ctx context.Context, e *models.WebhookEndpoint) error
	List(ctx context.Context, tenantID string) ([]models.WebhookEndpoint, error)
	Delete(ctx context.Context, tenantID, id string) error
	ListForEvent(ctx context.Context, tenantID, event string) ([]models.WebhookEndpoint, error)
	LogDelivery(ctx context.Context, d *models.WebhookDelivery) error
	ListDeliveries(ctx context.Context, tenantID, endpointID string) ([]models.WebhookDelivery, error)
}

type OutboxRepo interface {
	Enqueue(ctx context.Context, o *models.Outbox) error
	ClaimDue(ctx context.Context, limit int) ([]models.Outbox, error)
	MarkDelivered(ctx context.Context, id string) error
	Reschedule(ctx context.Context, id string, attempts int, availableInSeconds int) error
	MarkFailed(ctx context.Context, id string) error
}

type EmailRepo interface {
	CreateThread(ctx context.Context, t *models.EmailThread) error
	FindByToken(ctx context.Context, token string) (*models.EmailThread, error)
	Get(ctx context.Context, tenantID, convID string) (*models.EmailThread, error)
	Update(ctx context.Context, t *models.EmailThread) error
}

type ArchiveRepo interface {
	CreateTombstone(ctx context.Context, a *models.ConversationArchive) error
	GetTombstone(ctx context.Context, tenantID, convID string) (*models.ConversationArchive, error)
	PurgeHot(ctx context.Context, tenantID, convID string) error // delete hot rows in a tx
}
