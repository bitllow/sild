package storage

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/bitllow/sild/backend/internal/config"
	"github.com/bitllow/sild/backend/internal/id"
)

// New returns the configured bucket backend. dig binds the result to Bucket.
// local is the dev/OSS default; gcs/s3 are wired in the storage buildout.
func New(cfg *config.Config) (Bucket, error) {
	switch cfg.Storage.Backend {
	case "local", "":
		return &localBucket{publicURL: strings.TrimRight(cfg.Storage.PublicURL, "/"), dir: cfg.Storage.LocalDir}, nil
	case "gcs":
		return newGCSBucket(cfg.Storage)
	case "s3":
		return newS3Bucket(cfg.Storage)
	default:
		return nil, fmt.Errorf("unknown STORAGE_BACKEND %q", cfg.Storage.Backend)
	}
}

// localBucket serves uploads through the backend's own /v1/uploads/local route
// (dev only; bytes do transit the backend here, unlike GCS/S3 direct PUT).
type localBucket struct {
	publicURL string
	dir       string
}

func (b *localBucket) NewObjectKey(tenantID, filename string) string {
	safe := url.PathEscape(filename)
	return fmt.Sprintf("%s/%s/%s", tenantID, id.New("obj"), safe)
}

func (b *localBucket) SignPut(_ context.Context, objectKey, _ string, _ int64) (SignedUpload, error) {
	return SignedUpload{
		ObjectKey: objectKey,
		UploadURL: b.publicURL + "/v1/uploads/local/" + objectKey,
		ExpiresAt: time.Now().Add(15 * time.Minute),
	}, nil
}

func (b *localBucket) SignGet(_ context.Context, objectKey string, _ time.Duration) (string, error) {
	return b.publicURL + "/v1/uploads/local/" + objectKey, nil
}

// Put writes bytes to the on-disk object store, under the same objects/ root the
// local PUT/GET routes use (so a server-side write is readable via SignGet).
func (b *localBucket) Put(_ context.Context, objectKey string, data []byte, _ string) error {
	full := filepath.Join(b.dir, "objects", filepath.Clean("/"+objectKey))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		return err
	}
	return os.WriteFile(full, data, 0o644)
}

// LocalDir exposes the storage dir so the local PUT/GET route can read/write it.
func (b *localBucket) LocalDir() string { return b.dir }
