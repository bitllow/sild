package gormstore

import (
	"time"

	"github.com/bitllow/sild/backend/internal/store"
	"gorm.io/gorm"
)

// Paginate applies the keyset predicate, ordering and the limit+1 probe to a
// query, then splits the overflow row off into HasMore/NextCursor. This is the
// ONE place keyset SQL is written — before it, the same block was copy-pasted
// between the assignment queue and the peer list.
//
// sortExpr is the SQL expression for p.Sort and idExpr the tiebreak column. For
// store.SortID the two are the same and the predicate degenerates to a single
// comparison, which is why ULID-keyed collections need no timestamp column.
//
// valueOf recomputes the sort value from a loaded row: the computed column is
// ordered/filtered on but never SELECTed, because it loses its type on SQLite.
func Paginate[T any](
	q *gorm.DB,
	p store.PageParams,
	sortExpr, idExpr string,
	valueOf func(*T) time.Time,
	idOf func(*T) string,
) (store.Page[T], error) {
	limit := store.ClampLimit(p.Limit, 30)

	dir, cmp := "ASC", ">"
	if p.Desc() {
		dir, cmp = "DESC", "<"
	}

	idOnly := p.Sort == store.SortID
	if c := p.Cursor; c != nil {
		if idOnly {
			q = q.Where(idExpr+" "+cmp+" ?", c.ID)
		} else {
			q = q.Where(sortExpr+" "+cmp+" ? OR ("+sortExpr+" = ? AND "+idExpr+" "+cmp+" ?)",
				c.Value, c.Value, c.ID)
		}
	}

	q = q.Order(sortExpr + " " + dir)
	if !idOnly {
		q = q.Order(idExpr + " " + dir)
	}

	var rows []T
	if err := q.Limit(limit + 1).Find(&rows).Error; err != nil {
		return store.Page[T]{}, err
	}

	page := store.Page[T]{}
	if len(rows) > limit {
		page.HasMore = true
		rows = rows[:limit]
	}
	page.Items = rows
	if page.HasMore && len(rows) > 0 {
		page.NextCursor = cursorFor(&rows[len(rows)-1], p, idOnly, valueOf, idOf)
	}
	return page, nil
}

func cursorFor[T any](last *T, p store.PageParams, idOnly bool, valueOf func(*T) time.Time, idOf func(*T) string) *store.Cursor {
	c := &store.Cursor{Key: p.Sort, Order: p.Order, ID: idOf(last)}
	if !idOnly && valueOf != nil {
		v := valueOf(last)
		c.Value = &v
	}
	return c
}

// PaginateScan is Paginate for queries that need Scan (a join projecting onto a
// struct) rather than Find. gorm's Find would re-derive the table from the
// destination type and lose the join.
func PaginateScan[T any](
	q *gorm.DB,
	p store.PageParams,
	sortExpr, idExpr string,
	valueOf func(*T) time.Time,
	idOf func(*T) string,
) (store.Page[T], error) {
	limit := store.ClampLimit(p.Limit, 30)

	dir, cmp := "ASC", ">"
	if p.Desc() {
		dir, cmp = "DESC", "<"
	}

	idOnly := p.Sort == store.SortID
	if c := p.Cursor; c != nil {
		if idOnly {
			q = q.Where(idExpr+" "+cmp+" ?", c.ID)
		} else {
			q = q.Where(sortExpr+" "+cmp+" ? OR ("+sortExpr+" = ? AND "+idExpr+" "+cmp+" ?)",
				c.Value, c.Value, c.ID)
		}
	}

	q = q.Order(sortExpr + " " + dir)
	if !idOnly {
		q = q.Order(idExpr + " " + dir)
	}

	var rows []T
	if err := q.Limit(limit + 1).Scan(&rows).Error; err != nil {
		return store.Page[T]{}, err
	}

	page := store.Page[T]{}
	if len(rows) > limit {
		page.HasMore = true
		rows = rows[:limit]
	}
	page.Items = rows
	if page.HasMore && len(rows) > 0 {
		page.NextCursor = cursorFor(&rows[len(rows)-1], p, idOnly, valueOf, idOf)
	}
	return page, nil
}
