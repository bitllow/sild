package storage

import (
	"context"
	"strings"
	"testing"
	"time"

	gcs "cloud.google.com/go/storage"
	"github.com/bitllow/sild/backend/internal/config"
)

// stubSign records what the bucket asked GCS to sign, so the grant's shape can be
// asserted without credentials to sign with.
func stubBucket() (*gcsBucket, *gcs.SignedURLOptions) {
	var captured gcs.SignedURLOptions
	b := &gcsBucket{name: "sild-test"}
	b.sign = func(objectKey string, opts *gcs.SignedURLOptions) (string, error) {
		captured = *opts
		return "https://storage.googleapis.com/sild-test/" + objectKey + "?X-Goog-Signature=stub", nil
	}
	return b, &captured
}

// The content type is signed, so a client that uploads something else is rejected
// by the bucket rather than accepted and served with the wrong type.
func TestSignPutBindsMethodContentTypeAndExpiry(t *testing.T) {
	b, opts := stubBucket()
	before := time.Now()

	up, err := b.SignPut(context.Background(), "t_1/obj_1/a.png", "image/png", 1234)
	if err != nil {
		t.Fatalf("sign put: %v", err)
	}
	if opts.Method != "PUT" {
		t.Fatalf("method = %q", opts.Method)
	}
	if opts.ContentType != "image/png" {
		t.Fatalf("content type = %q", opts.ContentType)
	}
	if opts.Scheme != gcs.SigningSchemeV4 {
		t.Fatalf("scheme is not V4")
	}
	if up.ObjectKey != "t_1/obj_1/a.png" {
		t.Fatalf("object key = %q", up.ObjectKey)
	}
	if !strings.Contains(up.UploadURL, "X-Goog-Signature") {
		t.Fatalf("upload url is not signed: %s", up.UploadURL)
	}
	// The grant must expire, and the reported expiry must be the signed one — a
	// client caching past it would otherwise get an opaque 403.
	if up.ExpiresAt.Before(before) || up.ExpiresAt.After(before.Add(putTTL+time.Minute)) {
		t.Fatalf("expiry %v outside the grant window", up.ExpiresAt)
	}
	if !up.ExpiresAt.Equal(opts.Expires) {
		t.Fatalf("reported expiry %v != signed expiry %v", up.ExpiresAt, opts.Expires)
	}
}

func TestSignGetUsesCallerTTLAndFallsBack(t *testing.T) {
	b, opts := stubBucket()

	if _, err := b.SignGet(context.Background(), "t_1/obj_1/a.png", time.Hour); err != nil {
		t.Fatalf("sign get: %v", err)
	}
	if opts.Method != "GET" {
		t.Fatalf("method = %q", opts.Method)
	}
	if d := time.Until(opts.Expires); d < 55*time.Minute || d > 65*time.Minute {
		t.Fatalf("caller TTL ignored: expires in %v", d)
	}
	// A zero TTL must not sign an already-expired URL.
	if _, err := b.SignGet(context.Background(), "t_1/obj_1/a.png", 0); err != nil {
		t.Fatalf("sign get: %v", err)
	}
	if d := time.Until(opts.Expires); d <= 0 || d > getTTL+time.Minute {
		t.Fatalf("zero TTL did not fall back: expires in %v", d)
	}
}

// Keys must be tenant-scoped and collision-free, and a filename must not be able
// to escape its prefix.
func TestNewObjectKeyIsTenantScopedAndUnique(t *testing.T) {
	b, _ := stubBucket()

	first := b.NewObjectKey("t_1", "holiday photo.png")
	second := b.NewObjectKey("t_1", "holiday photo.png")
	if first == second {
		t.Fatal("two keys for the same filename collided")
	}
	for _, key := range []string{first, second} {
		if !strings.HasPrefix(key, "t_1/") {
			t.Fatalf("key %q is not tenant-scoped", key)
		}
		if strings.Contains(key, " ") {
			t.Fatalf("key %q is not URL-safe", key)
		}
	}
	// A filename carrying separators must stay one segment, or it would place the
	// object outside its tenant prefix.
	key := b.NewObjectKey("t_1", "../../etc/passwd")
	if parts := strings.Split(key, "/"); len(parts) != 3 {
		t.Fatalf("filename escaped its segment: %q split into %d parts", key, len(parts))
	}
}

// The bucket name is the one thing the backend cannot default, so it fails at
// construction rather than on the first upload.
func TestGCSRequiresABucketName(t *testing.T) {
	_, err := newGCSBucket(config.Storage{Backend: "gcs"})
	if err == nil {
		t.Fatal("constructed a gcs bucket with no STORAGE_BUCKET")
	}
	if !strings.Contains(err.Error(), "STORAGE_BUCKET") {
		t.Fatalf("error does not name the missing setting: %v", err)
	}
}
