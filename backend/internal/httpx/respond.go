// Package httpx holds shared HTTP helpers: the standard error envelope (§4) and
// small response utilities used across all handler packages.
package httpx

import "github.com/gin-gonic/gin"

// Error writes the standard error envelope: {"error":{"code","message"}} (§4).
func Error(c *gin.Context, status int, code, message string) {
	c.AbortWithStatusJSON(status, gin.H{"error": gin.H{"code": code, "message": message}})
}

// Common error helpers.
func Unauthorized(c *gin.Context, msg string) { Error(c, 401, "unauthorized", msg) }
func Forbidden(c *gin.Context, msg string)    { Error(c, 403, "forbidden", msg) }
func NotFound(c *gin.Context, msg string)     { Error(c, 404, "not_found", msg) }
func BadRequest(c *gin.Context, msg string)   { Error(c, 400, "bad_request", msg) }
func Conflict(c *gin.Context, msg string)     { Error(c, 409, "conflict", msg) }
func Internal(c *gin.Context, msg string)     { Error(c, 500, "internal", msg) }
