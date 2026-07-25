// Package httpx holds shared HTTP helpers: the standard error envelope (§4) and
// small response utilities used across all handler packages.
package httpx

import "github.com/gin-gonic/gin"

// RequestIDKey is where the request-id middleware stores the canonical id.
const RequestIDKey = "sild.request_id"

// Stable machine-readable error codes. `message` is for humans and may be
// reworded freely; `code` is API surface and changes only with the version, so
// SDKs branch on code and never parse messages.
const (
	CodeUnauthorized        = "unauthorized"
	CodeForbidden           = "forbidden"
	CodeNotFound            = "not_found"
	CodeBadRequest          = "bad_request"
	CodeUnprocessable       = "unprocessable"
	CodeConflict            = "conflict"
	CodeInternal            = "internal"
	CodePayloadTooLarge     = "payload_too_large"
	CodePreconditionMissing = "precondition_required"
	CodeStaleVersion        = "stale_version"
	CodeRateLimited         = "rate_limited"
	CodeUnknownField        = "unknown_field"
	CodeFieldNotWritable    = "field_not_writable"
)

// Error writes the standard error envelope (§4):
//
//	{"error":{"code","message","request_id","fields"}}
func Error(c *gin.Context, status int, code, message string) {
	writeError(c, status, code, message, nil)
}

// FieldError writes an error carrying per-field detail, so a client can point at
// the offending input instead of parsing prose.
func FieldError(c *gin.Context, status int, code, message string, fields map[string]string) {
	writeError(c, status, code, message, fields)
}

func writeError(c *gin.Context, status int, code, message string, fields map[string]string) {
	body := gin.H{"code": code, "message": message}
	if rid := RequestID(c); rid != "" {
		body["request_id"] = rid
	}
	if len(fields) > 0 {
		body["fields"] = fields
	}
	c.AbortWithStatusJSON(status, gin.H{"error": body})
}

// RequestID returns the canonical server-generated id for this request.
func RequestID(c *gin.Context) string {
	if v, ok := c.Get(RequestIDKey); ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

// Common error helpers.
//
// 401 vs 403: 401 when no valid credential was presented, 403 when a valid
// credential lacks the capability — never 401 for an authenticated caller.
// 400 vs 422: 400 for malformed or undecodable input, 422 for well-formed input
// failing a semantic rule.
func Unauthorized(c *gin.Context, msg string) { Error(c, 401, CodeUnauthorized, msg) }
func Forbidden(c *gin.Context, msg string)    { Error(c, 403, CodeForbidden, msg) }
func NotFound(c *gin.Context, msg string)     { Error(c, 404, CodeNotFound, msg) }
func BadRequest(c *gin.Context, msg string)   { Error(c, 400, CodeBadRequest, msg) }
func Unprocessable(c *gin.Context, msg string) {
	Error(c, 422, CodeUnprocessable, msg)
}
func Conflict(c *gin.Context, msg string) { Error(c, 409, CodeConflict, msg) }
func Internal(c *gin.Context, msg string) { Error(c, 500, CodeInternal, msg) }
func PayloadTooLarge(c *gin.Context, msg string) {
	Error(c, 413, CodePayloadTooLarge, msg)
}

// PreconditionRequired is the missing-If-Match case. One status per cause: 428
// absent, 400 malformed, 412 stale — 400 is never used for a missing
// precondition.
func PreconditionRequired(c *gin.Context, msg string) {
	Error(c, 428, CodePreconditionMissing, msg)
}

// PreconditionFailed is the stale-If-Match case.
func PreconditionFailed(c *gin.Context, msg string) {
	Error(c, 412, CodeStaleVersion, msg)
}
