package middleware

import (
	"sync"
	"time"

	"github.com/bitllow/sild/backend/internal/httpx"
	"github.com/gin-gonic/gin"
)

// Rate limiting is applied where it is a security control rather than a
// nicety: credential acquisition (password login, token mint) and
// unauthenticated ingress (inbound email, local upload). Those are reachable by
// anyone, and nothing limited them before. General per-route limits on
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

// rateLimit is a fixed-window per-client-IP counter. In-process and therefore
// per-replica: it blunts brute force and accidental floods, and is not a
// distributed quota. A shared limiter belongs with the general rate-limit work.
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
