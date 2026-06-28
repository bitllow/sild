package api

import (
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/bitllow/sild/backend/internal/httpx"
	"github.com/gin-gonic/gin"
)

// The local storage backend (dev/OSS default) serves attachment bytes through
// these routes — the signed PUT/GET URLs returned by storage.localBucket point
// here. GCS/S3 issue real direct-to-bucket URLs and don't use this (§11).

// localUploadPut stores uploaded bytes for the given object key.
func (h *Handler) localUploadPut(c *gin.Context) {
	full, ok := h.localObjectPath(c.Param("objectKey"))
	if !ok {
		httpx.BadRequest(c, "invalid object key")
		return
	}
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		httpx.Internal(c, "storage error")
		return
	}
	f, err := os.Create(full)
	if err != nil {
		httpx.Internal(c, "storage error")
		return
	}
	defer f.Close()
	if _, err := io.Copy(f, c.Request.Body); err != nil {
		httpx.Internal(c, "write error")
		return
	}
	c.Status(http.StatusOK)
}

// localUploadGet serves previously uploaded bytes.
func (h *Handler) localUploadGet(c *gin.Context) {
	full, ok := h.localObjectPath(c.Param("objectKey"))
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
	key = strings.TrimPrefix(key, "/")
	clean := filepath.Clean("/" + key) // collapses any ".."
	base := filepath.Join(h.cfg.Storage.LocalDir, "objects")
	full := filepath.Join(base, clean)
	if !strings.HasPrefix(full, filepath.Clean(base)+string(os.PathSeparator)) {
		return "", false
	}
	return full, true
}
