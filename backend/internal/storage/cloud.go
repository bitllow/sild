package storage

import (
	"errors"

	"github.com/bitllow/sild/backend/internal/config"
)

// The S3 backend is not wired. GCS lives in gcs.go.
func newS3Bucket(config.Storage) (Bucket, error) {
	return nil, errors.New("s3 storage backend not yet wired; use STORAGE_BACKEND=gcs or local")
}
