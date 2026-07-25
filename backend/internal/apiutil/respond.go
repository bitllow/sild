package apiutil

import (
	"net/http"

	"github.com/bitllow/sild/backend/internal/store"
	"github.com/gin-gonic/gin"
)

// RespondPage writes the uniform list envelope:
//
//	{ "items": [...], "next_cursor": "<opaque>|null", "has_more": bool }
//
// Every collection endpoint returns this shape, so a client can write one list
// parser. Resource-specific extras (the queue's counters) sit beside the
// envelope via RespondPageWith, never inside it.
//
// next_cursor is null exactly when has_more is false.
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

// RespondCatchUp writes a ?since= reconnect read. It keeps the same envelope so
// clients need only one list parser, but next_cursor is always null: the
// continuation token is the last message id, not a cursor.
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
