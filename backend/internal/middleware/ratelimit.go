package middleware

import (
	"sync"
	"time"

	"github.com/bitllow/sild/backend/internal/httpx"
	"github.com/gin-gonic/gin"
)

// Applied where it is a security control: credential acquisition and
// unauthenticated ingress, both reachable by anyone. General per-route limits on
// authenticated endpoints are separate work.

// Rate-limit classes.
const (
	rateAuthPerMinute    = 10
	rateIngressPerMinute = 120
)

// RateLimitAuth guards credential acquisition.
func (a *Auth) RateLimitAuth() gin.HandlerFunc { return rateLimit(rateAuthPerMinute) }

// RateLimitIngress guards unauthenticated write ingress.
func (a *Auth) RateLimitIngress() gin.HandlerFunc { return rateLimit(rateIngressPerMinute) }

// rateLimit is a fixed-window per-IP counter, in-process and so per-replica: it
// blunts brute force, it is not a distributed quota.
func rateLimit(perMinute int) gin.HandlerFunc {
	var (
		mu      sync.Mutex
		window  time.Time
		counter = map[string]int{}
	)
	return func(c *gin.Context) {
		now := time.Now().Truncate(time.Minute)
		key := c.ClientIP()

		mu.Lock()
		if now.After(window) {
			window = now
			counter = map[string]int{}
		}
		counter[key]++
		over := counter[key] > perMinute
		mu.Unlock()

		if over {
			c.Header("Retry-After", "60")
			httpx.Error(c, 429, httpx.CodeRateLimited, "too many requests")
			return
		}
		c.Next()
	}
}
