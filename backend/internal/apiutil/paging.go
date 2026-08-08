package apiutil

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/bitllow/sild/backend/internal/httpx"
	"github.com/bitllow/sild/backend/internal/store"
	"github.com/gin-gonic/gin"
)

// cursorVersion lets the wire format change without guessing at an old shape.
const cursorVersion = 1

// cursorDTO is the opaque wire form. It carries everything that must not change
// between pages, so a mismatched replay is a 400 rather than a wrong page.
type cursorDTO struct {
	V int    `json:"v"`
	R string `json:"r"` // resource — rejects cross-resource replay
	S string `json:"s"` // sort key
	O string `json:"o"` // direction
	F string `json:"f"` // fingerprint of the filter set
	// K keeps its ORIGINAL offset, never normalized to UTC: SQLite compares
	// datetimes as strings, so the same instant in another zone sorts wrong.
	K  *time.Time `json:"k,omitempty"`
	ID string     `json:"id"`
}

// PageDefaults describe a resource's paging contract.
type PageDefaults struct {
	Resource string
	Limit    int
	Sort     store.SortKey
	Order    store.Order
	// Sorts lists the sort keys this resource accepts from a client. Empty means
	// only the default.
	Sorts []store.SortKey
	// FixedOrder rejects ?order= — set for collections whose storage queries in
	// one direction, so a cursor cannot be labelled with an order the query
	// never applied.
	FixedOrder bool
}

// PageParams reads limit/cursor/sort/order and validates that the cursor was
// minted for this resource, sort, direction and filter set. Writes a 400 and
// returns ok=false on a mismatch.
func PageParams(c *gin.Context, d PageDefaults) (store.PageParams, bool) {
	p := store.PageParams{
		Limit: store.ClampLimit(atoi(c.Query("limit")), d.Limit),
		Sort:  d.Sort,
		Order: d.Order,
	}
	if p.Order == "" {
		p.Order = store.OrderDesc
	}

	if raw := c.Query("sort"); raw != "" {
		key := store.SortKey(raw)
		if !sortAllowed(key, d) {
			httpx.BadRequest(c, "unsupported sort key")
			return p, false
		}
		p.Sort = key
	}
	if raw := c.Query("order"); raw != "" {
		if raw != string(store.OrderAsc) && raw != string(store.OrderDesc) {
			httpx.BadRequest(c, "order must be asc or desc")
			return p, false
		}
		if d.FixedOrder && store.Order(raw) != p.Order {
			httpx.BadRequest(c, "this collection does not support an order override")
			return p, false
		}
		p.Order = store.Order(raw)
	}

	raw := c.Query("cursor")
	if raw == "" {
		return p, true
	}
	cur, err := decodeCursor(raw)
	if err != nil {
		httpx.BadRequest(c, "invalid cursor")
		return p, false
	}
	if cur.V != cursorVersion || cur.R != d.Resource {
		httpx.BadRequest(c, "cursor does not belong to this resource")
		return p, false
	}
	if cur.S != string(p.Sort) || cur.O != string(p.Order) {
		httpx.BadRequest(c, "cursor was minted for a different ordering")
		return p, false
	}
	if cur.F != FilterFingerprint(c) {
		httpx.BadRequest(c, "cursor was minted for a different filter set")
		return p, false
	}
	p.Cursor = &store.Cursor{Key: p.Sort, Order: p.Order, ID: cur.ID}
	p.Cursor.Value = cur.K
	return p, true
}

func sortAllowed(k store.SortKey, d PageDefaults) bool {
	if k == d.Sort {
		return true
	}
	for _, s := range d.Sorts {
		if s == k {
			return true
		}
	}
	return false
}

const fingerprintKey = "sild.filter_fp"

// FilterFingerprint hashes every non-paging query and path parameter — path ones
// too, since message ids are comparable ULIDs across conversations. Memoized:
// validating and minting a cursor would otherwise each hash the query.
func FilterFingerprint(c *gin.Context) string {
	if v, ok := c.Get(fingerprintKey); ok {
		if fp, ok := v.(string); ok {
			return fp
		}
	}
	fp := computeFingerprint(c)
	c.Set(fingerprintKey, fp)
	return fp
}

func computeFingerprint(c *gin.Context) string {
	parts := make([]string, 0, 8)
	for k, vs := range c.Request.URL.Query() {
		// Paging position, and expansion, which decides what a row CARRIES rather
		// than which rows there are — binding it would 400 a client that turned an
		// expansion off mid-scroll.
		if k == "limit" || k == "cursor" || k == "expand" {
			continue
		}
		sorted := append([]string(nil), vs...)
		sort.Strings(sorted)
		parts = append(parts, k+"="+strings.Join(sorted, ","))
	}
	for _, pp := range c.Params {
		parts = append(parts, "path:"+pp.Key+"="+pp.Value)
	}
	sort.Strings(parts)
	sum := sha256.Sum256([]byte(strings.Join(parts, "&")))
	return hex.EncodeToString(sum[:8])
}

// EncodeCursor renders a cursor for the wire, bound to this request's resource
// and filter set.
func EncodeCursor(c *gin.Context, resource string, cur *store.Cursor) any {
	if cur == nil {
		return nil
	}
	dto := cursorDTO{
		V: cursorVersion, R: resource,
		S: string(cur.Key), O: string(cur.Order),
		F: FilterFingerprint(c), ID: cur.ID,
	}
	dto.K = cur.Value
	b, err := json.Marshal(dto)
	if err != nil {
		return nil
	}
	return base64.RawURLEncoding.EncodeToString(b)
}

func decodeCursor(raw string) (cursorDTO, error) {
	var dto cursorDTO
	b, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		return dto, err
	}
	err = json.Unmarshal(b, &dto)
	return dto, err
}

func atoi(s string) int {
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0
	}
	return n
}
