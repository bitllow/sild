package domain

import (
	"context"

	"github.com/bitllow/sild/backend/internal/storage"
	"github.com/bitllow/sild/backend/internal/store"
	"github.com/bitllow/sild/backend/internal/store/models"
)

// IssueUploadInput describes a requested upload (§4.1/§4.2).
type IssueUploadInput struct {
	MimeType  string
	SizeBytes int64
	Filename  string
	Uploader  store.Participant
}

// IssueUpload validates the size against the tenant cap (§11), records an
// ownership row (review finding), and returns a signed direct-to-bucket PUT.
func (s *Service) IssueUpload(ctx context.Context, tenantID string, in IssueUploadInput) (*storage.SignedUpload, error) {
	if in.SizeBytes <= 0 {
		return nil, invalid("size_bytes is required")
	}
	tenant, err := s.store.Tenants().Get(ctx, tenantID)
	if err != nil {
		return nil, mapStoreErr(err)
	}
	limit := tenant.MaxAttachmentBytes
	if limit <= 0 {
		limit = 10 << 20
	}
	if in.SizeBytes > limit {
		return nil, invalid("attachment exceeds the tenant size limit")
	}

	objectKey := s.bucket.NewObjectKey(tenantID, in.Filename)
	signed, err := s.bucket.SignPut(ctx, objectKey, in.MimeType, in.SizeBytes)
	if err != nil {
		return nil, err
	}

	up := &models.Upload{
		TenantID: tenantID, UploaderKind: in.Uploader.Kind,
		ExternalUserID: in.Uploader.ExternalUserID, InternalActorID: in.Uploader.InternalActorID,
		ObjectKey: objectKey, MimeType: in.MimeType, SizeBytes: in.SizeBytes,
		Filename: in.Filename, Status: models.UploadPending, CreatedAt: s.now(),
	}
	if err := s.store.Uploads().Create(ctx, up); err != nil {
		return nil, err
	}
	return &signed, nil
}

// CompleteUpload marks an upload completed (called after the client confirms the
// PUT, or lazily when the object_key is first attached). Idempotent.
func (s *Service) CompleteUpload(ctx context.Context, tenantID, objectKey string) error {
	return s.store.Uploads().MarkCompleted(ctx, tenantID, objectKey)
}
