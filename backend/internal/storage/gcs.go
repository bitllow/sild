package storage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	gcs "cloud.google.com/go/storage"
	"github.com/bitllow/sild/backend/internal/config"
)

// gcsBucket issues V4 signed URLs so bytes go direct to the bucket (§11). With no
// private key configured SignedURL signs via the IAM SignBlob API, so a workload
// identity needs roles/iam.serviceAccountTokenCreator on its own service account
// rather than a key file.
type gcsBucket struct {
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
	return &gcsBucket{handle: handle, sign: handle.SignedURL}, nil
}

func (b *gcsBucket) NewObjectKey(tenantID, filename string) string {
	return newObjectKey(tenantID, filename)
}

// SignPut grants a direct PUT. The content type is part of the signature.
func (b *gcsBucket) SignPut(_ context.Context, objectKey, mimeType string, _ int64) (SignedUpload, error) {
	exp := time.Now().Add(signTTL)
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
		ttl = signTTL
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
