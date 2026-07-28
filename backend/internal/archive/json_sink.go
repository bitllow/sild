package archive

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path"

	"github.com/bitllow/sild/backend/internal/config"
	"github.com/bitllow/sild/backend/internal/storage"
)

// jsonSink writes archive/{tenant}/{conversation}.json through the configured
// storage bucket (§12), so the sink follows STORAGE_BACKEND rather than keeping a
// second notion of where objects live: gcs/s3 in a deployment, the local dir in
// dev. sink_ref is the object key. JSON sinks are NOT queryable — the accepted
// tradeoff (§12).
type jsonSink struct {
	bucket storage.Bucket
	kind   string
}

func newJSONSink(cfg *config.Config, bucket storage.Bucket) (Sink, error) {
	kind := cfg.Archive.Sink
	if kind == "" {
		kind = "gcs_json"
	}
	return &jsonSink{bucket: bucket, kind: kind}, nil
}

func (s *jsonSink) Name() string { return s.kind }

func (s *jsonSink) Write(ctx context.Context, c SerializedConversation) (string, error) {
	objectKey := path.Join("archive", c.TenantID, c.ConversationID+".json")
	b, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return "", err
	}
	if err := s.bucket.Put(ctx, objectKey, b, "application/json"); err != nil {
		return "", err
	}
	return objectKey, nil
}

func (s *jsonSink) Read(ctx context.Context, sinkRef string) (SerializedConversation, error) {
	b, err := s.bucket.Get(ctx, sinkRef)
	if err != nil {
		return SerializedConversation{}, err
	}
	var c SerializedConversation
	if err := json.Unmarshal(b, &c); err != nil {
		return SerializedConversation{}, fmt.Errorf("archive %s: %w", sinkRef, err)
	}
	return c, nil
}

// bigQuerySink is the queryable sink (§12). Real impl inserts flat rows into
// partitioned tables; wired in the BigQuery buildout.
func newBigQuerySink(*config.Config) (Sink, error) {
	return nil, errors.New("bigquery sink not yet wired; use ARCHIVE_SINK=gcs_json")
}
