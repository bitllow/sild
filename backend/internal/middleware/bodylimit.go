package middleware

import (
	"errors"
	"net/http"

	"github.com/bitllow/sild/backend/internal/httpx"
	"github.com/gin-gonic/gin"
)

// Body size defaults, per route: a JSON mutation gets kilobytes, an attachment
// PUT the upload cap.
const (
	BodyLimitJSON    int64 = 256 << 10 // 256 KiB
	BodyLimitEmail   int64 = 25 << 20  // 25 MiB — inbound mail carries attachments
	BodyLimitUpload  int64 = 50 << 20  // 50 MiB
	BodyLimitImport  int64 = 8 << 20   // 8 MiB — a translation file is text, but a big project's is long
	BodyLimitDefault int64 = 1 << 20   // 1 MiB
)

// BodyLimit caps the request body before the handler reads it. Middleware rather
// than part of JSON decoding, because the raw upload and inbound-email paths
// never decode JSON — and those are the unbounded writes.
func BodyLimit(max int64) gin.HandlerFunc {
	return func(c *gin.Context) {
		if max <= 0 {
			c.Next()
			return
		}
		// Reject on the declared length before reading a byte; MaxBytesReader then
		// covers a chunked or lying Content-Length. Answering here keeps the 413
		// contract, since handlers map every decode failure to 400.
		if c.Request.ContentLength > max {
			FailBodyTooLarge(c)
			return
		}
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, max)
		c.Next()
	}
}

// IsBodyTooLarge reports whether an error came from the body cap (413, not 500).
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
