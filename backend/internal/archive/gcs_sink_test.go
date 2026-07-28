package archive_test

import (
	"context"
	"os"
	"testing"

	gcs "cloud.google.com/go/storage"
	"github.com/bitllow/sild/backend/internal/archive"
	"github.com/bitllow/sild/backend/internal/config"
	"github.com/bitllow/sild/backend/internal/storage"
)

// ARCHIVE_SINK=gcs_json used to write to a local directory despite its name, so a
// worker archived conversations into its own container and the tombstone pointed at
// bytes nobody else could read. The sink now goes through storage.Bucket; this
// drives it against a real GCS client (fake-gcs-server) to prove the object lands
// in the bucket and rehydrates from it.
//
//	docker run -p 4443:4443 fsouza/fake-gcs-server -scheme http -backend memory
//	STORAGE_EMULATOR_HOST=localhost:4443 go test ./internal/archive/
func TestGCSJSONSinkRoundTripsThroughTheBucket(t *testing.T) {
	if os.Getenv("STORAGE_EMULATOR_HOST") == "" {
		t.Skip("STORAGE_EMULATOR_HOST not set — no GCS emulator to test against")
	}
	const name = "sild-archive-test"
	client, err := gcs.NewClient(context.Background())
	if err != nil {
		t.Fatalf("emulator client: %v", err)
	}
	_ = client.Bucket(name).Create(context.Background(), "sild-test", nil)

	cfg := &config.Config{
		Storage: config.Storage{Backend: "gcs", Bucket: name},
		Archive: config.Archive{Sink: "gcs_json", IdleDays: 30},
	}
	bucket, err := storage.New(cfg)
	if err != nil {
		t.Fatalf("bucket: %v", err)
	}
	sink, err := archive.New(cfg, bucket)
	if err != nil {
		t.Fatalf("sink: %v", err)
	}
	if sink.Name() != "gcs_json" {
		t.Fatalf("sink name = %q", sink.Name())
	}

	ctx := context.Background()
	want := archive.SerializedConversation{
		ConversationID: "c_archive_1", TenantID: "t_archive", Reference: "trip_1",
		Status: "closed", Kind: "support", MessageCount: 1,
		Messages: []map[string]any{{"id": "m_1", "body": "archived body"}},
	}
	ref, err := sink.Write(ctx, want)
	if err != nil {
		t.Fatalf("write: %v", err)
	}

	// The object is really in the bucket, at the key the tombstone will carry.
	if _, err := bucket.Get(ctx, ref); err != nil {
		t.Fatalf("archived object is not readable at sink_ref %q: %v", ref, err)
	}
	got, err := sink.Read(ctx, ref)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if got.ConversationID != want.ConversationID || got.MessageCount != want.MessageCount {
		t.Fatalf("rehydrated %+v, want %+v", got, want)
	}
	if len(got.Messages) != 1 || got.Messages[0]["body"] != "archived body" {
		t.Fatalf("messages did not survive: %+v", got.Messages)
	}
}
