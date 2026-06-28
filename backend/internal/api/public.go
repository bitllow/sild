package api

import (
	"net/http"

	"github.com/bitllow/sild/backend/internal/httpx"
	"github.com/gin-gonic/gin"
)

// jwks serves the public verification keys (§2.5, §4.4).
func (h *Handler) jwks(c *gin.Context) {
	set, err := h.km.JWKS(c.Request.Context())
	if err != nil {
		httpx.Internal(c, "could not load keys")
		return
	}
	c.JSON(http.StatusOK, set)
}
