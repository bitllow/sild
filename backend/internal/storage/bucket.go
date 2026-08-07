// Package storage abstracts the object bucket holding attachments (§11) and
// archived conversations (§12): clients upload direct via a signed PUT and
// download via a signed GET; bytes never transit the chat backend. GCS and a
// local dev backend implement one interface; s3 is not wired.
package storage

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"time"

	"github.com/bitllow/sild/backend/internal/id"
)

// SignTTL is how long an upload or download grant stays valid. Exported so the
// render paths asking for a default grant agree with the backends issuing it.
const SignTTL = 15 * time.Minute

// newObjectKey is shared by the backends so a key is portable between them; the
// local PUT/GET routes parse this shape (see api/uploads_local.go).
func newObjectKey(tenantID, filename string) string {
	return fmt.Sprintf("%s/%s/%s", tenantID, id.New("obj"), url.PathEscape(filename))
}

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
	// Get reads object bytes server-side; ErrObjectNotFound when the key holds
	// nothing.
	Get(ctx context.Context, objectKey string) ([]byte, error)
}

// ErrObjectNotFound distinguishes a missing object from unreachable storage.
var ErrObjectNotFound = errors.New("object not found")
