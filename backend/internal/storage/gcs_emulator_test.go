package storage

import (
	"bytes"
	"context"
	"errors"
	"os"
	"testing"

	gcs "cloud.google.com/go/storage"
	"github.com/bitllow/sild/backend/internal/config"
)

// Drives the real GCS client, which the signing tests stub out. Without
// -public-host the emulator serves reads at a host the client cannot resolve, and
// every Get comes back not-found.
//
//	docker run -p 4443:4443 fsouza/fake-gcs-server \
//	  -scheme http -backend memory -public-host localhost:4443
//	STORAGE_EMULATOR_HOST=localhost:4443 go test ./internal/storage/
func emulatorBucket(t *testing.T) Bucket {
	t.Helper()
	if os.Getenv("STORAGE_EMULATOR_HOST") == "" {
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

func TestGCSGetMissingObjectIsNotFound(t *testing.T) {
	bucket := emulatorBucket(t)

	_, err := bucket.Get(context.Background(), "t_round/does-not-exist.json")
	if !errors.Is(err, ErrObjectNotFound) {
		t.Fatalf("missing object gave %v, want ErrObjectNotFound", err)
	}
}

// A large object must not size the writer's retry buffer to itself: the archive
// sweep already holds the conversation and its JSON, and a third copy of a long
// history OOMs the worker.
func TestGCSPutCapsTheWriterBuffer(t *testing.T) {
	bucket := emulatorBucket(t)
	big := bytes.Repeat([]byte("x"), defaultChunkSize+4096)
	key := bucket.NewObjectKey("t_big", "big.bin")

	if err := bucket.Put(context.Background(), key, big, "application/octet-stream"); err != nil {
		t.Fatalf("put: %v", err)
	}
	got, err := bucket.Get(context.Background(), key)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if len(got) != len(big) {
		t.Fatalf("round-tripped %d bytes, wrote %d", len(got), len(big))
	}
}
