package middleware

import (
	"github.com/bitllow/sild/backend/internal/httpx"
	"github.com/bitllow/sild/backend/internal/principal"
	"github.com/bitllow/sild/backend/internal/store/models"
	"github.com/gin-gonic/gin"
)

// RequireRole guards admin routes by platform role (§7). Must run after Admin().
// owner/admin manage api-keys/webhooks/team + all conversations; agent gets the
// inbox only.
func RequireRole(roles ...models.PlatformRole) gin.HandlerFunc {
	return func(c *gin.Context) {
		p := Get(c)
		if p == nil || p.Kind != principal.KindAdmin {
			httpx.Forbidden(c, "insufficient platform role")
			return
		}
		// Any one of them: a second role widens what a member reaches, never narrows it.
		for _, r := range roles {
			if p.HasRole(r) {
				c.Next()
				return
			}
		}
		httpx.Forbidden(c, "insufficient platform role")
	}
}
