// Package models holds the GORM models that define the §3 schema.
//
// The schema is portable across Postgres, MySQL, and SQLite (see ARCHITECTURE
// §4): no native arrays (child tables instead), jsonb via datatypes.JSON, and
// tenant_id on every table (the §1 invariant). IDs are sortable prefixed ULIDs.
package models

// ── Member / sender taxonomy ───────────────────────────────────────────────

// MemberKind identifies a conversation participant's namespace.
// Exactly one of external_user_id / internal_actor_id is set per member.
type MemberKind string

const (
	MemberUser  MemberKind = "user"  // host end-user (client/driver/dispatcher/guest)
	MemberAgent MemberKind = "agent" // our admin_users.id
	MemberBot   MemberKind = "bot"   // reserved synthetic actor
	MemberEmail MemberKind = "email" // external party reachable by email (address in external_user_id)
)

// SenderKind identifies who authored a message.
type SenderKind string

const (
	SenderUser   SenderKind = "user"
	SenderAgent  SenderKind = "agent"
	SenderBot    SenderKind = "bot"
	SenderSystem SenderKind = "system"
)

// ConvRole is the per-conversation role (RBAC within a conversation).
type ConvRole string

const (
	RoleDispatcher ConvRole = "dispatcher"
	RoleClient     ConvRole = "client"
	RoleDriver     ConvRole = "driver"
	RoleAgent      ConvRole = "agent"
)

// ── Lifecycle / visibility ─────────────────────────────────────────────────

// ConversationStatus: open|closed. closed is terminal (§1, no reopen).
type ConversationStatus string

const (
	ConversationOpen   ConversationStatus = "open"
	ConversationClosed ConversationStatus = "closed"
)

// ConversationKind distinguishes a support conversation (has an agent assignment,
// lives in the queue) from a peer conversation (direct end-user chat, no
// assignment, observed by peer_access operators). It is set once at creation and
// is the single durable source of truth — every surface (queue, badge, search,
// peer list, authorization) reads this column rather than re-deriving peer-ness
// from assignment presence, so the classification can't drift or silently
// reclassify an assignment-less conversation.
type ConversationKind string

const (
	KindSupport ConversationKind = "support"
	KindPeer    ConversationKind = "peer"
)

// AssignmentStatus state machine: queued → assigned → closed; assigned → queued
// (return to queue). closed is TERMINAL.
type AssignmentStatus string

const (
	AssignmentQueued   AssignmentStatus = "queued"
	AssignmentAssigned AssignmentStatus = "assigned"
	AssignmentClosed   AssignmentStatus = "closed"
)

// Visibility: participants (delivered out) | internal (agent-only note, §5.6).
type Visibility string

const (
	VisibilityParticipants Visibility = "participants"
	VisibilityInternal     Visibility = "internal"
)

// Valid reports whether v is a known visibility. Empty is valid and means "use the
// default" — an UNKNOWN value must never reach storage: reads disagree about it
// (thread history matches `= participants`, unread counts match `<> internal`), so
// it would arrive, chime and bump a badge while being absent from the thread.
func (v Visibility) Valid() bool {
	return v == "" || v == VisibilityParticipants || v == VisibilityInternal
}

// Channel: how a message entered/left — app (WS/SDK) | email (mail connector).
type Channel string

const (
	ChannelApp   Channel = "app"
	ChannelEmail Channel = "email"
)

// Valid reports whether c is a known channel. Empty means "use the default"; an
// unknown value would slip past the outbound-mail gate, which suppresses only the
// exact email channel.
func (c Channel) Valid() bool {
	return c == "" || c == ChannelApp || c == ChannelEmail
}

// Disposition is an attachment render hint, not storage (§11).
type Disposition string

const (
	DispositionInline     Disposition = "inline"
	DispositionAttachment Disposition = "attachment"
)

// ── Platform roles (admin_users) ───────────────────────────────────────────

// PlatformRole guards API/inbox access (§7).
type PlatformRole string

const (
	PlatformOwner PlatformRole = "owner"
	PlatformAdmin PlatformRole = "admin"
	PlatformAgent PlatformRole = "agent"
)

// ── Push / upload / archive ────────────────────────────────────────────────

// PushPlatform: ios|android|web.
type PushPlatform string

const (
	PushIOS     PushPlatform = "ios"
	PushAndroid PushPlatform = "android"
	PushWeb     PushPlatform = "web"
)

// UploadStatus tracks the direct-to-bucket lifecycle (§11 + ownership record).
type UploadStatus string

const (
	UploadPending   UploadStatus = "pending"
	UploadCompleted UploadStatus = "completed"
)

// ArchiveSink: bigquery|gcs_json|s3_json (§12).
type ArchiveSink string

const (
	SinkBigQuery ArchiveSink = "bigquery"
	SinkGCSJSON  ArchiveSink = "gcs_json"
	SinkS3JSON   ArchiveSink = "s3_json"
)

// OutboxStatus / DeliveryStatus for the webhook outbox + delivery log (§6.1).
type DeliveryStatus string

const (
	DeliveryPending   DeliveryStatus = "pending"
	DeliveryDelivered DeliveryStatus = "delivered"
	DeliveryFailed    DeliveryStatus = "failed"
)
