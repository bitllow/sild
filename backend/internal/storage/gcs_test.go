package storage

import (
	"strings"
	"testing"
	"time"

	gcs "cloud.google.com/go/storage"
	"github.com/bitllow/sild/backend/internal/config"
)

// The signing call itself needs credentials, so what is assertable here is the
// grant it covers: a wrong method, scheme or content type is a 403 at upload time.
func TestSignOptsBindsMethodContentTypeAndScheme(t *testing.T) {
	exp := time.Now().Add(SignTTL)
	put := signOpts("PUT", "image/png", exp)
	if put.Method != "PUT" || put.ContentType != "image/png" {
		t.Fatalf("put opts = %+v", put)
	}
	if put.Scheme != gcs.SigningSchemeV4 {
		t.Fatal("put is not signed V4")
	}
	if !put.Expires.Equal(exp) {
		t.Fatalf("expiry %v != %v", put.Expires, exp)
	}
	// A GET signature must not bind a content type, or the download 403s unless
	// the client happens to send the same header.
	if got := signOpts("GET", "", exp); got.Method != "GET" || got.ContentType != "" {
		t.Fatalf("get opts = %+v", got)
	}
}

func TestNewObjectKeyIsTenantScopedAndUnique(t *testing.T) {
	first := newObjectKey("t_1", "holiday photo.png")
	second := newObjectKey("t_1", "holiday photo.png")
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
	key := newObjectKey("t_1", "../../etc/passwd")
	if parts := strings.Split(key, "/"); len(parts) != 3 {
		t.Fatalf("filename escaped its segment: %q split into %d parts", key, len(parts))
	}
}

func TestGCSRequiresABucketName(t *testing.T) {
	_, err := newGCSBucket(config.Storage{Backend: "gcs"})
	if err == nil {
		t.Fatal("constructed a gcs bucket with no STORAGE_BUCKET")
	}
	if !strings.Contains(err.Error(), "STORAGE_BUCKET") {
		t.Fatalf("error does not name the missing setting: %v", err)
	}
}
