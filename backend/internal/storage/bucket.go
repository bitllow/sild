// Package storage abstracts the attachment bucket (§11): clients upload direct
// via a signed PUT and download via a signed GET; bytes never transit the chat
// backend. GCS, S3, and a local dev backend implement one interface.
package storage

import (
	"context"
	"errors"
	"time"
)

// SignedUpload is a direct-to-bucket PUT grant.
type SignedUpload struct {
	ObjectKey string
	UploadURL string
	ExpiresAt time.Time
}

// Bucket is the storage backend. dig binds the configured implementation.
type Bucket interface {
	// SignPut issues a signed upload URL for a new object_key.
	SignPut(ctx context.Context, objectKey, mimeType string, sizeBytes int64) (SignedUpload, error)
	// SignGet issues a signed download URL for reading an object.
	SignGet(ctx context.Context, objectKey string, ttl time.Duration) (string, error)
	// NewObjectKey returns a fresh, tenant-scoped object key.
	NewObjectKey(tenantID, filename string) string
	// Put writes object bytes server-side. Clients still upload direct via
	// SignPut (§11); this is for ingestion paths that already hold the bytes —
	// the email forwarding daemon writing inbound attachments (§6.2).
	Put(ctx context.Context, objectKey string, data []byte, mimeType string) error
	// Get reads object bytes server-side, for the paths that consume an object
	// rather than hand it to a client — the archive sink rehydrating a
	// conversation (§12). ErrObjectNotFound when the key holds nothing.
	Get(ctx context.Context, objectKey string) ([]byte, error)
}

// ErrObjectNotFound reports a key with no object behind it, so callers can tell
// "archived conversation is gone" from "storage is unreachable".
var ErrObjectNotFound = errors.New("object not found")
