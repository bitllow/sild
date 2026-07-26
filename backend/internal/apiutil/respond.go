package apiutil

import (
	"net/http"

	"github.com/bitllow/sild/backend/internal/store"
	"github.com/gin-gonic/gin"
)

// RespondPage writes the uniform list envelope — {items, next_cursor, has_more}
// — so a client needs one list parser. next_cursor is null exactly when has_more
// is false; resource extras go beside it via RespondPageWith, never inside it.
func RespondPage[T any](c *gin.Context, resource string, page store.Page[T]) {
	RespondPageWith(c, resource, page, nil)
}

// RespondPageWith writes the list envelope plus top-level extras.
func RespondPageWith[T any](c *gin.Context, resource string, page store.Page[T], extra gin.H) {
	items := page.Items
	if items == nil {
		items = []T{}
	}
	body := gin.H{
		"items":       items,
		"next_cursor": EncodeCursor(c, resource, page.NextCursor),
		"has_more":    page.HasMore,
	}
	for k, v := range extra {
		body[k] = v
	}
	c.JSON(http.StatusOK, body)
}

// RespondCatchUp writes a ?since= reconnect read. Same envelope, but next_cursor
// is always null: the continuation token is the last message id.
func RespondCatchUp[T any](c *gin.Context, page store.Page[T]) {
	items := page.Items
	if items == nil {
		items = []T{}
	}
	c.JSON(http.StatusOK, gin.H{
		"items":       items,
		"next_cursor": nil,
		"has_more":    page.HasMore,
	})
}
