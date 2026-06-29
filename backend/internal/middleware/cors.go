package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// CORS lets browser clients on other origins call the API and SSE. The web
// drop-in (§9) runs on the customer's own site, so its requests to chat.sild.io
// are cross-origin. Auth is Bearer-token (user JWT), never cookies — so the
// origin is reflected without Allow-Credentials. The admin inbox reaches the API
// same-origin through its own proxy and is unaffected.
func CORS() gin.HandlerFunc {
	return func(c *gin.Context) {
		if origin := c.GetHeader("Origin"); origin != "" {
			c.Header("Access-Control-Allow-Origin", origin)
			c.Header("Vary", "Origin")
			c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, PATCH, OPTIONS")
			c.Header("Access-Control-Allow-Headers", "Authorization, Content-Type")
			c.Header("Access-Control-Max-Age", "600")
		}
		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}
