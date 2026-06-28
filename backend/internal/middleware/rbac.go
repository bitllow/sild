package middleware

import (
	"github.com/bitllow/sild/backend/internal/httpx"
	"github.com/bitllow/sild/backend/internal/store/models"
	"github.com/gin-gonic/gin"
)

// RequireRole guards admin routes by platform role (§7). Must run after Admin().
// owner/admin manage api-keys/webhooks/team + all conversations; agent gets the
// inbox only.
func RequireRole(roles ...models.PlatformRole) gin.HandlerFunc {
	allowed := make(map[models.PlatformRole]bool, len(roles))
	for _, r := range roles {
		allowed[r] = true
	}
	return func(c *gin.Context) {
		p := Get(c)
		if p == nil || p.Kind != KindAdmin || !allowed[p.Role] {
			httpx.Forbidden(c, "insufficient platform role")
			return
		}
		c.Next()
	}
}
