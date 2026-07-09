package store

import (
	"context"
	"errors"
	"time"

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
	// FindByInboundToken resolves a tenant by the local part of its forwarding
	// address — the sild-mail daemon's lookup (§6.2).
	FindByInboundToken(ctx context.Context, token string) (*models.TenantEmailConfig, error)
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
	SetRole(ctx context.Context, tenantID, id string, role models.PlatformRole) error
	SetPeerAccess(ctx context.Context, tenantID, id string, peerAccess bool) error
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
	// ListPeers returns one keyset-paginated page of the tenant's peer
	// conversations (open, no assignment), newest-activity first, with optional
	// server-side role + free-text filtering. Mirrors the assignment queue's
	// pagination so the peer surface has the same limits / infinite scroll / search.
	ListPeers(ctx context.Context, tenantID string, p PeerParams) (PeerPage, error)
	// TouchLastMessage updates the denormalized last-activity timestamp + preview
	// used by the inbox queue ordering (see models.Conversation).
	TouchLastMessage(ctx context.Context, tenantID, convID string, at time.Time, preview string) error
	// CountOpen returns the number of open conversations in the tenant.
	CountOpen(ctx context.Context, tenantID string) (int64, error)
	// CountOpenSupport counts open conversations that carry an assignment — the
	// inbox's open-conversation badge (§8). Peer conversations (no assignment) are
	// a separate surface and never counted here.
	CountOpenSupport(ctx context.Context, tenantID string) (int64, error)
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
	// ListQueue returns one cursor-paginated, sorted page of inbox assignments
	// enriched with each conversation + its active members + last activity, so a
	// queue page renders from a single query (§4.3).
	ListQueue(ctx context.Context, tenantID string, p QueueParams) (QueuePage, error)
	// ConversationIDs returns every conversation in the tenant that currently
	// carries an assignment (unpaginated) — used to compute an agent's realtime
	// channel subscription set (§5.2).
	ConversationIDs(ctx context.Context, tenantID string) ([]string, error)
	// CountQueue returns the per-scope queue counts (assigned-to-me / unassigned /
	// closed) for the inbox scope-tab counters + "Show closed" toggle. Computed
	// over the representative (latest) assignment per conversation, matching
	// ListQueue's semantics.
	CountQueue(ctx context.Context, tenantID, actorID string) (QueueCounts, error)
}

// QueueCounts are the inbox scope-tab counters. You = assigned to the calling
// agent; Unassigned = queued; Closed = closed assignments.
type QueueCounts struct {
	You        int64
	Unassigned int64
	Closed     int64
}

// QueueSort selects the ordering key for the inbox queue.
type QueueSort string

const (
	QueueSortLastActivity QueueSort = "last_activity" // newest message first (default)
	QueueSortCreated      QueueSort = "created"       // conversation start (date started)
	QueueSortWaiting      QueueSort = "waiting_since" // current assignment/queue entry time
)

// QueueCursor is the keyset position: the sort value + assignment id tiebreaker
// of the last row returned. Stable under inserts (no offset drift).
type QueueCursor struct {
	Value time.Time
	ID    string
}

// QueueParams are the filters + pagination for ListQueue.
type QueueParams struct {
	Status   *models.AssignmentStatus
	Assignee *string
	// ExcludeClosed drops closed assignments from the page — the "All" scope with
	// the "Show closed" toggle off (Status is nil there, so a != filter is needed).
	ExcludeClosed bool
	Sort          QueueSort
	Desc          bool
	Limit         int
	Cursor        *QueueCursor // nil for the first page
}

// QueueItem is one enriched queue row.
type QueueItem struct {
	Assignment   models.Assignment
	Conversation models.Conversation
	Members      []models.ConversationMember
	LastActivity time.Time
}

// QueuePage is one page of the queue with the cursor for the next page.
type QueuePage struct {
	Items      []QueueItem
	NextCursor *QueueCursor
	HasMore    bool
}

// MessagePage is a page of history with a has-more flag.
type MessagePage struct {
	Messages []models.Message
	HasMore  bool
}

// PeerParams are the filter + keyset pagination for the peer-conversation LIST
// (the default, no-query view — mirrors the assignment queue's ListQueue). Role
// filters to conversations that include the given conv_role. Free-text SEARCH is
// not here: it reuses the shared search.Backend via GET /admin/search?peer=true,
// so id/metadata/keyword matching is identical to the support inbox's search.
type PeerParams struct {
	Role   string
	Limit  int
	Cursor *QueueCursor // keyset by (last_activity, conversation id); nil = first page
}

// PeerItem is one enriched peer-list row (conversation + active members, no
// message history — the thread loads on open).
type PeerItem struct {
	Conversation models.Conversation
	Members      []models.ConversationMember
	LastActivity time.Time
}

// PeerPage is one page of peer conversations with the cursor for the next page.
type PeerPage struct {
	Items      []PeerItem
	NextCursor *QueueCursor
	HasMore    bool
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
	SetActive(ctx context.Context, tenantID, id string, active bool) error
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
	// FindOpenByToken resolves a reply that carries the thread token (via the
	// Reply-To +subaddress) to its OPEN conversation — the precise threading key.
	FindOpenByToken(ctx context.Context, tenantID, token string) (*models.EmailThread, error)
	// FindOpenBySenderSubject is the no-token fallback: resolve by original sender
	// + normalized subject among OPEN conversations (§6.2 threading).
	FindOpenBySenderSubject(ctx context.Context, tenantID, sender, subjectKey string) (*models.EmailThread, error)
	Get(ctx context.Context, tenantID, convID string) (*models.EmailThread, error)
	Update(ctx context.Context, t *models.EmailThread) error
}

// BrandRepo persists per-tenant brands for the messenger appearance (Settings →
// Appearance). Brands are edited as a set and saved atomically, so the write path
// is a full replace rather than per-row CRUD.
type BrandRepo interface {
	// List returns the tenant's brands ordered by position.
	List(ctx context.Context, tenantID string) ([]models.Brand, error)
	// Active returns the tenant's active brand, or ErrNotFound if the tenant has
	// none yet.
	Active(ctx context.Context, tenantID string) (*models.Brand, error)
	// Replace atomically swaps the tenant's entire brand set for the given rows
	// (delete-all + insert). Positions and the single Active flag are taken from
	// the passed rows as-is; the domain layer normalizes them first.
	Replace(ctx context.Context, tenantID string, brands []models.Brand) error
}

type ArchiveRepo interface {
	CreateTombstone(ctx context.Context, a *models.ConversationArchive) error
	GetTombstone(ctx context.Context, tenantID, convID string) (*models.ConversationArchive, error)
	PurgeHot(ctx context.Context, tenantID, convID string) error // delete hot rows in a tx
}
