package archive_test

import (
	"context"
	"strings"
	"testing"

	"github.com/bitllow/sild/backend/internal/archive"
	"github.com/bitllow/sild/backend/internal/config"
	"github.com/bitllow/sild/backend/internal/storage"
)

// The sink writes through storage.Bucket, so the object lands wherever
// STORAGE_BACKEND points; storage/gcs_emulator_test.go covers the GCS client itself.
func TestJSONSinkRoundTripsThroughTheBucket(t *testing.T) {
	cfg := &config.Config{
		Storage: config.Storage{Backend: "local", LocalDir: t.TempDir()},
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
	if !strings.HasPrefix(ref, "archive/t_archive/") {
		t.Fatalf("sink_ref %q is not a tenant-scoped bucket key", ref)
	}

	// The tombstone's sink_ref has to be a key the bucket itself can serve.
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
