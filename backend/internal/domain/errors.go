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
)

// ValidationError carries a human message for a 400.
type ValidationError struct{ Msg string }

func (e *ValidationError) Error() string { return e.Msg }
func (e *ValidationError) Unwrap() error { return ErrValidation }

func invalid(msg string) error { return &ValidationError{Msg: msg} }
