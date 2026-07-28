package storage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"time"

	gcs "cloud.google.com/go/storage"
	"github.com/bitllow/sild/backend/internal/config"
	"github.com/bitllow/sild/backend/internal/id"
)

// putTTL bounds a direct upload grant; getTTL is the fallback read window when a
// caller asks for none.
const (
	putTTL = 15 * time.Minute
	getTTL = 15 * time.Minute
)

// gcsBucket issues V4 signed URLs so bytes go direct to the bucket (§11).
//
// Credentials come from ADC and signing goes through the IAM SignBlob API, which
// BucketHandle.SignedURL selects on its own when no private key is configured —
// so a Cloud Run or GKE workload identity needs no key file, only
// roles/iam.serviceAccountTokenCreator on its own service account.
type gcsBucket struct {
	name   string
	handle *gcs.BucketHandle
	// sign is BucketHandle.SignedURL in production and a stub in tests, which
	// have no credentials to sign with.
	sign func(objectKey string, opts *gcs.SignedURLOptions) (string, error)
}

func newGCSBucket(cfg config.Storage) (Bucket, error) {
	if cfg.Bucket == "" {
		return nil, errors.New("STORAGE_BUCKET is required with STORAGE_BACKEND=gcs")
	}
	client, err := gcs.NewClient(context.Background())
	if err != nil {
		return nil, fmt.Errorf("gcs: %w", err)
	}
	handle := client.Bucket(cfg.Bucket)
	return &gcsBucket{name: cfg.Bucket, handle: handle, sign: handle.SignedURL}, nil
}

// NewObjectKey mirrors the local backend so a key is portable between backends.
func (b *gcsBucket) NewObjectKey(tenantID, filename string) string {
	return fmt.Sprintf("%s/%s/%s", tenantID, id.New("obj"), url.PathEscape(filename))
}

// SignPut grants a direct PUT. The content type is part of the signature, so a
// client that sends a different one is rejected by GCS rather than by us.
func (b *gcsBucket) SignPut(_ context.Context, objectKey, mimeType string, _ int64) (SignedUpload, error) {
	exp := time.Now().Add(putTTL)
	signed, err := b.sign(objectKey, &gcs.SignedURLOptions{
		Scheme:      gcs.SigningSchemeV4,
		Method:      "PUT",
		ContentType: mimeType,
		Expires:     exp,
	})
	if err != nil {
		return SignedUpload{}, fmt.Errorf("gcs sign put %s: %w", objectKey, err)
	}
	return SignedUpload{ObjectKey: objectKey, UploadURL: signed, ExpiresAt: exp}, nil
}

func (b *gcsBucket) SignGet(_ context.Context, objectKey string, ttl time.Duration) (string, error) {
	if ttl <= 0 {
		ttl = getTTL
	}
	signed, err := b.sign(objectKey, &gcs.SignedURLOptions{
		Scheme:  gcs.SigningSchemeV4,
		Method:  "GET",
		Expires: time.Now().Add(ttl),
	})
	if err != nil {
		return "", fmt.Errorf("gcs sign get %s: %w", objectKey, err)
	}
	return signed, nil
}

func (b *gcsBucket) Put(ctx context.Context, objectKey string, data []byte, mimeType string) error {
	w := b.handle.Object(objectKey).NewWriter(ctx)
	w.ContentType = mimeType
	if _, err := w.Write(data); err != nil {
		_ = w.Close()
		return fmt.Errorf("gcs put %s: %w", objectKey, err)
	}
	if err := w.Close(); err != nil { // the upload completes on Close
		return fmt.Errorf("gcs put %s: %w", objectKey, err)
	}
	return nil
}

func (b *gcsBucket) Get(ctx context.Context, objectKey string) ([]byte, error) {
	r, err := b.handle.Object(objectKey).NewReader(ctx)
	if errors.Is(err, gcs.ErrObjectNotExist) {
		return nil, ErrObjectNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("gcs get %s: %w", objectKey, err)
	}
	defer r.Close()
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("gcs get %s: %w", objectKey, err)
	}
	return data, nil
}
