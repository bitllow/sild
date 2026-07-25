package apiutil

import (
	"strings"

	"github.com/bitllow/sild/backend/internal/httpx"
	"github.com/gin-gonic/gin"
)

// Mutable configuration is read whole and written whole, so two operators
// editing the same screen means last-write-wins with no sign anything was lost.
// A version derived from the current state closes that: the write must name the
// state it was based on.

// RequireIfMatch insists the client named a version and returns it. The
// comparison itself belongs in the write transaction (see domain.Version) —
// doing it here would leave a window where two writes from the same version
// both pass.
//
// One status per cause: 428 when the header is absent, 400 when it is present
// but malformed; 412 comes from the write path when it is stale.
//
// Returns false having already written the response.
func RequireIfMatch(c *gin.Context) (string, bool) {
	raw := strings.TrimSpace(c.GetHeader("If-Match"))
	if raw == "" {
		httpx.PreconditionRequired(c, "If-Match is required; GET the resource for its current ETag")
		return "", false
	}
	if raw != "*" && (!strings.HasPrefix(raw, `"`) || !strings.HasSuffix(raw, `"`) || len(raw) < 3) {
		httpx.BadRequest(c, "If-Match must be a quoted entity tag")
		return "", false
	}
	return raw, true
}
