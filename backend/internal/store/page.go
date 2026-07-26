package store

import "time"

// SortKey names the column a cursor is positioned on. It travels inside the
// cursor with the direction, so a cursor cannot be replayed under an ordering it
// was not minted for — reversing direction would return the preceding rows.
type SortKey string

const (
	// SortID is a ULID: chronological and unique, so a complete keyset alone.
	SortID           SortKey = "id"
	SortLastActivity SortKey = "last_activity" // COALESCE(last_message_at, created_at)
	SortCreated      SortKey = "created"
	SortWaitingSince SortKey = "waiting_since"
)

// Order is the scan direction.
type Order string

const (
	OrderAsc  Order = "asc"
	OrderDesc Order = "desc"
)

// Cursor is the keyset position of the last row returned. Value is nil for
// SortID, where the id alone positions the scan.
type Cursor struct {
	Key   SortKey
	Order Order
	Value *time.Time
	ID    string
}

// PageParams are the paging inputs shared by every collection.
type PageParams struct {
	Limit  int
	Sort   SortKey
	Order  Order
	Cursor *Cursor // nil for the first page
}

// Desc reports whether the scan runs newest-first.
func (p PageParams) Desc() bool { return p.Order != OrderAsc }

// Page is one page of a collection plus the position to continue from.
// NextCursor is nil exactly when HasMore is false.
type Page[T any] struct {
	Items      []T
	NextCursor *Cursor
	HasMore    bool
}

// ClampLimit bounds a limit to [1,100]. Limits clamp rather than error.
func ClampLimit(limit, def int) int {
	if limit <= 0 {
		return def
	}
	if limit > 100 {
		return 100
	}
	return limit
}

// SlicePage keyset-paginates an already-loaded slice, ordered per p — the
// in-memory twin of gormstore.Paginate.
func SlicePage[T any](items []T, p PageParams, idOf func(*T) string) Page[T] {
	limit := ClampLimit(p.Limit, 50)

	start := 0
	if p.Cursor != nil {
		// Skip forward to just past the cursor position.
		for i := range items {
			id := idOf(&items[i])
			if p.Desc() && id < p.Cursor.ID || !p.Desc() && id > p.Cursor.ID {
				start = i
				break
			}
			start = i + 1
		}
	}
	items = items[min(start, len(items)):]

	page := Page[T]{}
	if len(items) > limit {
		page.HasMore = true
		items = items[:limit]
	}
	page.Items = items
	if page.HasMore && len(items) > 0 {
		last := &items[len(items)-1]
		page.NextCursor = &Cursor{Key: p.Sort, Order: p.Order, ID: idOf(last)}
	}
	return page
}
