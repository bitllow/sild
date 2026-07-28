package storage

import (
	"context"
	"errors"
	"os"
	"testing"

	gcs "cloud.google.com/go/storage"
	"github.com/bitllow/sild/backend/internal/config"
)

// The signing tests use a stub, so nothing there drives the real GCS client. This
// does: against fake-gcs-server (STORAGE_EMULATOR_HOST) it exercises the same
// Put/Get code a deployment runs, including the not-found mapping the archive read
// path depends on. Skipped when no emulator is configured.
//
//	docker run -p 4443:4443 fsouza/fake-gcs-server -scheme http -backend memory
//	STORAGE_EMULATOR_HOST=localhost:4443 go test ./internal/storage/
func emulatorBucket(t *testing.T) Bucket {
	t.Helper()
	host := os.Getenv("STORAGE_EMULATOR_HOST")
	if host == "" {
		t.Skip("STORAGE_EMULATOR_HOST not set — no GCS emulator to test against")
	}
	const name = "sild-emulator-test"
	client, err := gcs.NewClient(context.Background())
	if err != nil {
		t.Fatalf("emulator client: %v", err)
	}
	// Ignore an already-exists error so the test is re-runnable against a
	// persistent emulator.
	_ = client.Bucket(name).Create(context.Background(), "sild-test", nil)

	bucket, err := New(&config.Config{Storage: config.Storage{Backend: "gcs", Bucket: name}})
	if err != nil {
		t.Fatalf("gcs bucket: %v", err)
	}
	return bucket
}

func TestGCSPutGetRoundTrip(t *testing.T) {
	bucket := emulatorBucket(t)
	ctx := context.Background()
	key := bucket.NewObjectKey("t_round", "note.json")
	body := []byte(`{"hello":"bucket"}`)

	if err := bucket.Put(ctx, key, body, "application/json"); err != nil {
		t.Fatalf("put: %v", err)
	}
	got, err := bucket.Get(ctx, key)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if string(got) != string(body) {
		t.Fatalf("got %q, want %q", got, body)
	}
}

// A missing object must be distinguishable from an unreachable bucket: the
// archive read path shows "gone" for one and must not for the other (§12).
func TestGCSGetMissingObjectIsNotFound(t *testing.T) {
	bucket := emulatorBucket(t)

	_, err := bucket.Get(context.Background(), "t_round/does-not-exist.json")
	if !errors.Is(err, ErrObjectNotFound) {
		t.Fatalf("missing object gave %v, want ErrObjectNotFound", err)
	}
}
