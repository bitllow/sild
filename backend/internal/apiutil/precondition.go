package apiutil

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"

	"github.com/bitllow/sild/backend/internal/httpx"
	"github.com/gin-gonic/gin"
)

// Mutable configuration is read whole and written whole, so two operators
// editing the same screen means last-write-wins with no sign anything was lost.
// A version derived from the current state closes that: the write must name the
// state it was based on.

// ETagOf is the version of a configuration document: a hash of the value the
// caller last read. Content-derived rather than a stored counter, so it needs no
// column and cannot drift from what was served.
func ETagOf(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(b)
	return `"` + hex.EncodeToString(sum[:16]) + `"`
}

// SetETag stamps a configuration read with its version.
func SetETag(c *gin.Context, v any) { c.Header("ETag", ETagOf(v)) }

// RequirePreconditionMatch enforces If-Match against the current state.
//
// One status per cause: 428 when the header is absent, 400 when it is present
// but malformed, 412 when it is well-formed but stale.
//
// Returns false having already written the response.
func RequirePreconditionMatch(c *gin.Context, current any) bool {
	raw := strings.TrimSpace(c.GetHeader("If-Match"))
	if raw == "" {
		httpx.PreconditionRequired(c, "If-Match is required; GET the resource for its current ETag")
		return false
	}
	if raw == "*" { // "any current state" — the resource always exists here
		return true
	}
	if !strings.HasPrefix(raw, `"`) || !strings.HasSuffix(raw, `"`) || len(raw) < 3 {
		httpx.BadRequest(c, "If-Match must be a quoted entity tag")
		return false
	}
	if raw != ETagOf(current) {
		httpx.PreconditionFailed(c, "the resource changed since you read it; re-read and retry")
		return false
	}
	return true
}
