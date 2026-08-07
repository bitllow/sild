package storage

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/bitllow/sild/backend/internal/config"
)

// New returns the configured bucket backend. dig binds the result to Bucket.
// local is the dev/OSS default; gcs/s3 are wired in the storage buildout.
func New(cfg *config.Config) (Bucket, error) {
	switch cfg.Storage.Backend {
	case "local", "":
		return &localBucket{
			publicURL: strings.TrimRight(cfg.Storage.PublicURL, "/"),
			dir:       cfg.Storage.LocalDir,
			signer:    NewLocalSigner(cfg.Storage.SigningKey),
		}, nil
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
	signer    *LocalSigner
}

func (b *localBucket) NewObjectKey(tenantID, filename string) string {
	return newObjectKey(tenantID, filename)
}

func (b *localBucket) SignPut(_ context.Context, objectKey, _ string, _ int64) (SignedUpload, error) {
	exp := time.Now().Add(signTTL)
	return SignedUpload{
		ObjectKey: objectKey,
		UploadURL: b.publicURL + "/v1/uploads/local/" + objectKey + "?" + b.signer.Sign("PUT", objectKey, exp),
		ExpiresAt: exp,
	}, nil
}

func (b *localBucket) SignGet(_ context.Context, objectKey string, ttl time.Duration) (string, error) {
	if ttl <= 0 {
		ttl = signTTL
	}
	exp := time.Now().Add(ttl)
	return b.publicURL + "/v1/uploads/local/" + objectKey + "?" + b.signer.Sign("GET", objectKey, exp), nil
}

// Signer exposes the URL signer so the local PUT/GET routes can verify grants.
func (b *localBucket) Signer() *LocalSigner { return b.signer }

func (b *localBucket) Put(_ context.Context, objectKey string, data []byte, _ string) error {
	full := b.objectPath(objectKey)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		return err
	}
	return os.WriteFile(full, data, 0o644)
}

// objectPath is the on-disk location of a key, under the same objects/ root the
// local PUT/GET routes serve — so a server-side write is readable via SignGet.
func (b *localBucket) objectPath(objectKey string) string {
	return filepath.Join(b.dir, "objects", filepath.Clean("/"+objectKey))
}

func (b *localBucket) Get(_ context.Context, objectKey string) ([]byte, error) {
	data, err := os.ReadFile(b.objectPath(objectKey))
	if errors.Is(err, os.ErrNotExist) {
		return nil, ErrObjectNotFound
	}
	return data, err
}

// LocalDir exposes the storage dir so the local PUT/GET route can read/write it.
func (b *localBucket) LocalDir() string { return b.dir }
