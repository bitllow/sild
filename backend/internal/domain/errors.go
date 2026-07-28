// Package domain holds the use-case layer: business rules, transactions, and the
// §1 invariants. It depends only on interfaces (store, realtime.Publisher,
// storage.Bucket, auth.KeyManager) — never on gin or GORM directly.
package domain

import "errors"

// Domain errors. Handlers map these to HTTP status codes.
var (
	ErrNotFound   = errors.New("not found")
	ErrConflict   = errors.New("conflict")
	ErrForbidden  = errors.New("forbidden")
	ErrValidation = errors.New("validation")
	// ErrAlreadyIngested reports an inbound email whose Message-ID was already
	// ingested. Callers acknowledge it as success — the mail is not lost, it is
	// already a conversation.
	ErrAlreadyIngested = errors.New("already ingested")
	// ErrIngestInFlight reports that another delivery of the same mail is being
	// ingested right now. Transient: the sender must retry, because that attempt
	// may still fail and nobody has stored the mail yet.
	ErrIngestInFlight = errors.New("ingest in flight")
)

// ValidationError carries a human message for a 400.
type ValidationError struct{ Msg string }

func (e *ValidationError) Error() string { return e.Msg }
func (e *ValidationError) Unwrap() error { return ErrValidation }

func invalid(msg string) error { return &ValidationError{Msg: msg} }

// ConflictError is a conflict carrying a stable machine-readable code, so an SDK
// can branch on the reason instead of parsing the message.
type ConflictError struct {
	Code string
	Msg  string
}

func (e *ConflictError) Error() string { return e.Msg }
func (e *ConflictError) Unwrap() error { return ErrConflict }

// Conflict codes. These are API surface.
const (
	CodeAssignmentAlreadyClosed = "assignment_already_closed"
	CodeAssignmentAlreadyTaken  = "assignment_already_taken"
	CodeConversationClosed      = "conversation_closed"
	CodeLastMemberRemoval       = "last_member_removal"
	CodePeerNotQueueable        = "peer_not_queueable"
	CodeStaleVersion            = "stale_version"
)

func conflict(code, msg string) error { return &ConflictError{Code: code, Msg: msg} }
