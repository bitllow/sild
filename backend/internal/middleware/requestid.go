package middleware

import (
	"github.com/bitllow/sild/backend/internal/httpx"
	"github.com/bitllow/sild/backend/internal/id"
	"github.com/gin-gonic/gin"
)

// InboundRequestIDHeader is accepted as a correlation hint only.
const InboundRequestIDHeader = "X-Request-Id"

// RequestID stamps every response — and every error body — with a canonical
// server-generated id.
//
// An inbound X-Request-Id is never echoed as the response id: reflecting an
// unvalidated client string into every log line and error body is log injection
// with extra steps. It is validated and carried as a separate correlation field
// instead.
func RequestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		rid := id.New(id.Request)
		c.Set(httpx.RequestIDKey, rid)
		c.Header(InboundRequestIDHeader, rid)
		if in := c.GetHeader(InboundRequestIDHeader); validCorrelationID(in) {
			c.Set("sild.correlation_id", in)
		}
		c.Next()
	}
}

// validCorrelationID accepts a conservative charset and length, so a hostile
// value cannot forge log structure.
func validCorrelationID(s string) bool {
	if s == "" || len(s) > 64 {
		return false
	}
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
		default:
			return false
		}
	}
	return true
}
