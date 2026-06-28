package archive

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"

	"github.com/bitllow/sild/backend/internal/config"
)

// jsonSink writes archive/{tenant}/{conversation}.json (§12). In a real
// deployment the object lands in GCS/S3; here it lands in a local dir so the
// archival path is fully exercised without cloud creds. sink_ref is the object
// key. JSON sinks are NOT queryable — the accepted tradeoff (§12).
type jsonSink struct {
	dir  string
	kind string
}

func newJSONSink(cfg *config.Config) (Sink, error) {
	dir := cfg.Storage.LocalDir
	if dir == "" {
		dir = "./.archive"
	}
	kind := cfg.Archive.Sink
	if kind == "" {
		kind = "gcs_json"
	}
	return &jsonSink{dir: filepath.Join(dir, "archive"), kind: kind}, nil
}

func (s *jsonSink) Name() string { return s.kind }

func (s *jsonSink) Write(_ context.Context, c SerializedConversation) (string, error) {
	objectKey := filepath.Join(c.TenantID, c.ConversationID+".json")
	full := filepath.Join(s.dir, objectKey)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		return "", err
	}
	b, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(full, b, 0o644); err != nil {
		return "", err
	}
	return objectKey, nil
}

func (s *jsonSink) Read(_ context.Context, sinkRef string) (SerializedConversation, error) {
	b, err := os.ReadFile(filepath.Join(s.dir, sinkRef))
	if err != nil {
		return SerializedConversation{}, err
	}
	var c SerializedConversation
	if err := json.Unmarshal(b, &c); err != nil {
		return SerializedConversation{}, err
	}
	return c, nil
}

// bigQuerySink is the queryable sink (§12). Real impl inserts flat rows into
// partitioned tables; wired in the BigQuery buildout.
type bigQuerySink struct{}

func newBigQuerySink(*config.Config) (Sink, error) {
	return nil, errors.New("bigquery sink not yet wired; use ARCHIVE_SINK=gcs_json")
}
