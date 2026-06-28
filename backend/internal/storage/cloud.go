package storage

import (
	"errors"

	"github.com/bitllow/sild/backend/internal/config"
)

// GCS / S3 backends issue real signed PUT/GET URLs so bytes go direct to the
// bucket (§11). Implemented in the storage buildout; credentials via workload
// identity (GCS) / IAM role (S3) — no static keys in the app.

func newGCSBucket(config.Storage) (Bucket, error) {
	return nil, errors.New("gcs storage backend not yet wired; use STORAGE_BACKEND=local")
}

func newS3Bucket(config.Storage) (Bucket, error) {
	return nil, errors.New("s3 storage backend not yet wired; use STORAGE_BACKEND=local")
}
