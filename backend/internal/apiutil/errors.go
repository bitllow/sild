package apiutil

import (
	"errors"
	"net/http"

	"github.com/bitllow/sild/backend/internal/domain"
	"github.com/bitllow/sild/backend/internal/httpx"
	"github.com/gin-gonic/gin"
)

// Fail maps a domain error to the standard HTTP error envelope (§4).
//
// `code` is API surface and SDKs branch on it, so a conflict carrying its own
// code keeps that code rather than collapsing to a generic one.
func Fail(c *gin.Context, err error) {
	var conflict *domain.ConflictError
	switch {
	case errors.Is(err, domain.ErrNotFound):
		httpx.NotFound(c, "not found")
	case errors.Is(err, domain.ErrForbidden):
		httpx.Forbidden(c, "forbidden")
	case errors.As(err, &conflict):
		// A failed precondition is 412; other conflicts are 409.
		if conflict.Code == domain.CodeStaleVersion {
			httpx.PreconditionFailed(c, conflict.Msg)
			return
		}
		httpx.Error(c, http.StatusConflict, conflict.Code, conflict.Msg)
	case errors.Is(err, domain.ErrConflict):
		httpx.Conflict(c, "operation would leave the conversation in an invalid state")
	case errors.Is(err, domain.ErrValidation):
		// 422: the body parsed, but a semantic rule rejected it.
		httpx.Unprocessable(c, err.Error())
	default:
		httpx.Internal(c, "internal error")
	}
}
