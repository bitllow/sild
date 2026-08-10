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
	allowed := make(map[models.PlatformRole]bool, len(roles))
	for _, r := range roles {
		allowed[r] = true
	}
	return func(c *gin.Context) {
		p := Get(c)
		if p == nil || p.Kind != principal.KindAdmin || !holdsAny(allowed, p) {
			httpx.Forbidden(c, "insufficient platform role")
			return
		}
		c.Next()
	}
}

// holdsAny reports whether any role the member holds is admitted: a second role
// widens what they may reach, never narrows it.
func holdsAny(allowed map[models.PlatformRole]bool, p *principal.Principal) bool {
	for _, r := range p.Roles() {
		if allowed[r] {
			return true
		}
	}
	return false
}
