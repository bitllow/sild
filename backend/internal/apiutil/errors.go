package apiutil

import (
	"errors"

	"github.com/bitllow/sild/backend/internal/domain"
	"github.com/bitllow/sild/backend/internal/httpx"
	"github.com/gin-gonic/gin"
)

// Fail maps a domain error to the standard HTTP error envelope (§4).
func Fail(c *gin.Context, err error) {
	switch {
	case errors.Is(err, domain.ErrNotFound):
		httpx.NotFound(c, "not found")
	case errors.Is(err, domain.ErrForbidden):
		httpx.Forbidden(c, "forbidden")
	case errors.Is(err, domain.ErrConflict):
		httpx.Conflict(c, "operation would leave the conversation in an invalid state")
	case errors.Is(err, domain.ErrValidation):
		httpx.BadRequest(c, err.Error())
	default:
		httpx.Internal(c, "internal error")
	}
}
