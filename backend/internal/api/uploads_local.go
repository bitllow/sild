package api

import (
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/bitllow/sild/backend/internal/httpx"
	"github.com/bitllow/sild/backend/internal/middleware"
	"github.com/bitllow/sild/backend/internal/policy"
	"github.com/bitllow/sild/backend/internal/principal"
	"github.com/bitllow/sild/backend/internal/storage"
	"github.com/gin-gonic/gin"
)

// The local storage backend (dev/OSS default) serves attachment bytes through
// these routes — the signed PUT/GET URLs returned by storage.localBucket point
// here. GCS/S3 issue real direct-to-bucket URLs and don't use this (§11).
//
// These routes carry no credential, so the signature IS the credential: it is
// verified into a signed-capability principal scoped to one object key and one
// verb, and policy authorizes uploads.read/uploads.write against that.

// signedPrincipal verifies the URL signature and returns the capability it
// grants. The tenant comes from the object key's first segment, so a signature
// for one tenant's key says nothing about another's.
func (h *Handler) signedPrincipal(c *gin.Context, method, objectKey string) (*principal.Principal, bool) {
	signer, ok := h.localSigner()
	if !ok {
		return nil, false
	}
	if !signer.Verify(method, objectKey, c.Query(storage.ParamExpires), c.Query(storage.ParamSignature)) {
		return nil, false
	}
	tenant := storage.TenantForObjectKey(objectKey)
	if tenant == "" {
		return nil, false
	}
	return &principal.Principal{TenantID: tenant, Kind: principal.KindSigned, ObjectKey: objectKey}, true
}

func (h *Handler) localSigner() (*storage.LocalSigner, bool) {
	type signerBucket interface{ Signer() *storage.LocalSigner }
	b, ok := h.bucket.(signerBucket)
	if !ok {
		return nil, false
	}
	return b.Signer(), true
}

// localObjectPrefix is the route prefix the object key follows.
const localObjectPrefix = "/v1/uploads/local/"

// signedObjectKey is the key exactly as stored and signed.
//
// Object keys are URL-escaped at mint (storage.NewObjectKey escapes the
// filename), so a key for "my photo.png" contains %20. gin's c.Param decodes
// that, which would not match the signature or the stored key — so read the raw
// escaped path instead.
func signedObjectKey(c *gin.Context) string {
	raw := c.Request.URL.EscapedPath()
	if i := strings.Index(raw, localObjectPrefix); i >= 0 {
		return strings.TrimPrefix(raw[i+len(localObjectPrefix):], "/")
	}
	return strings.TrimPrefix(c.Param("objectKey"), "/")
}

// authorizeSigned resolves the signed grant and checks it against policy.
func (h *Handler) authorizeSigned(c *gin.Context, method string, action policy.Action) (string, bool) {
	key := signedObjectKey(c)
	p, ok := h.signedPrincipal(c, method, key)
	if !ok {
		httpx.Unauthorized(c, "invalid or expired upload signature")
		return "", false
	}
	if err := policy.Authorize(p, action, policy.ResourceAttrs{}); err != nil {
		httpx.Forbidden(c, "signature does not grant this action")
		return "", false
	}
	return key, true
}

// localUploadPut stores uploaded bytes for the given object key.
func (h *Handler) localUploadPut(c *gin.Context) {
	key, ok := h.authorizeSigned(c, http.MethodPut, policy.UploadsWrite)
	if !ok {
		return
	}
	full, ok := h.localObjectPath(key)
	if !ok {
		httpx.BadRequest(c, "invalid object key")
		return
	}
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		httpx.Internal(c, "storage error")
		return
	}

	// Write to a temp file and rename into place. os.Create would truncate the
	// target first, so a body rejected mid-stream by the size cap would destroy
	// an existing object — turning "upload something huge" into a way to delete
	// any attachment whose key you can guess.
	tmp, err := os.CreateTemp(filepath.Dir(full), ".upload-*")
	if err != nil {
		httpx.Internal(c, "storage error")
		return
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName) // no-op once the rename succeeds

	if _, err := io.Copy(tmp, c.Request.Body); err != nil {
		tmp.Close()
		if middleware.IsBodyTooLarge(err) {
			middleware.FailBodyTooLarge(c)
			return
		}
		httpx.Internal(c, "write error")
		return
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		httpx.Internal(c, "write error")
		return
	}
	if err := tmp.Close(); err != nil {
		httpx.Internal(c, "write error")
		return
	}
	if err := os.Rename(tmpName, full); err != nil {
		httpx.Internal(c, "storage error")
		return
	}
	c.Status(http.StatusOK)
}

// localUploadGet serves previously uploaded bytes.
func (h *Handler) localUploadGet(c *gin.Context) {
	key, ok := h.authorizeSigned(c, http.MethodGet, policy.UploadsRead)
	if !ok {
		return
	}
	full, ok := h.localObjectPath(key)
	if !ok {
		httpx.BadRequest(c, "invalid object key")
		return
	}
	if _, err := os.Stat(full); err != nil {
		httpx.NotFound(c, "object not found")
		return
	}
	c.File(full)
}

// localObjectPath resolves an object key to an on-disk path, rejecting traversal.
func (h *Handler) localObjectPath(key string) (string, bool) {
	if decoded, err := url.PathUnescape(key); err == nil {
		key = decoded
	}
	key = strings.TrimPrefix(key, "/")
	clean := filepath.Clean("/" + key) // collapses any ".."
	base := filepath.Join(h.cfg.Storage.LocalDir, "objects")
	full := filepath.Join(base, clean)
	if !strings.HasPrefix(full, filepath.Clean(base)+string(os.PathSeparator)) {
		return "", false
	}
	return full, true
}
