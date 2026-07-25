package middleware

import (
	"errors"
	"net/http"

	"github.com/bitllow/sild/backend/internal/httpx"
	"github.com/gin-gonic/gin"
)

// Body size defaults. Limits differ by route — a JSON mutation gets kilobytes,
// an attachment PUT gets the upload cap — which is why the limit belongs in the
// per-route descriptor rather than inside one decoder.
const (
	BodyLimitJSON    int64 = 256 << 10 // 256 KiB
	BodyLimitEmail   int64 = 25 << 20  // 25 MiB — inbound mail carries attachments
	BodyLimitUpload  int64 = 50 << 20  // 50 MiB
	BodyLimitDefault int64 = 1 << 20   // 1 MiB
)

// BodyLimit caps the request body before the handler reads it.
//
// This is middleware and not part of JSON decoding on purpose: localUploadPut
// copies the raw body and never decodes JSON, so a decoder-level cap would not
// run on precisely the unauthenticated unbounded write it was meant to close.
// Inbound email has the same shape.
func BodyLimit(max int64) gin.HandlerFunc {
	return func(c *gin.Context) {
		if max > 0 {
			c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, max)
		}
		c.Next()
	}
}

// IsBodyTooLarge reports whether an error came from the body cap, so handlers
// can answer 413 rather than a generic read failure.
func IsBodyTooLarge(err error) bool {
	if err == nil {
		return false
	}
	var maxErr *http.MaxBytesError
	return errors.As(err, &maxErr)
}

// FailBodyTooLarge writes the 413.
func FailBodyTooLarge(c *gin.Context) {
	httpx.PayloadTooLarge(c, "request body exceeds the limit for this route")
}
