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

// cursorVersion is the wire format version. It exists so the format can change
// without guessing at an old cursor's shape.
const cursorVersion = 1

// cursorDTO is the opaque wire form: base64url(JSON). Everything that must not
// change between pages is carried inside it, so a mismatched replay is a 400
// rather than a wrong answer under a 200.
type cursorDTO struct {
	V int    `json:"v"`
	R string `json:"r"` // resource — rejects cross-resource replay
	S string `json:"s"` // sort key
	O string `json:"o"` // direction
	F string `json:"f"` // fingerprint of the filter set
	// K is the position value, absent for id-only sorts. Marshalled as RFC3339
	// with its ORIGINAL offset, never normalized to UTC: SQLite compares
	// datetimes as strings, so a cursor rendered "…18:24:16+00:00" would not
	// order correctly against rows stored "…21:24:16+03:00" — same instant,
	// different string.
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
}

// PageParams reads limit/cursor/sort/order, clamps the limit, decodes the cursor
// and validates that it was minted for this resource, sort, direction and filter
// set. Writes a 400 and returns ok=false on a malformed or mismatched cursor.
//
// The filter fingerprint covers every non-paging query parameter plus the path
// parameters, computed generically from the request rather than from a
// hand-maintained field list — a hand-maintained list is wrong the first time
// someone adds a filter and forgets to add it.
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

// FilterFingerprint hashes every non-paging query parameter plus the path
// parameters. Path params matter as much as query ones: message ids are
// comparable ULIDs across conversations, so a cursor minted for conversation A
// would otherwise return a wrong-but-plausible page on conversation B.
func FilterFingerprint(c *gin.Context) string {
	parts := make([]string, 0, 8)
	for k, vs := range c.Request.URL.Query() {
		if k == "limit" || k == "cursor" {
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
