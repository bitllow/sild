package apiutil

import (
	"slices"
	"strconv"
	"strings"

	"github.com/bitllow/sild/backend/internal/httpx"
	"github.com/gin-gonic/gin"
)

// The `expand` convention, documented in ARCHITECTURE §5: a comma-separated list
// of `resource` or `resource.field` paths, each adding a top-level block beside
// the response. Field names are the resource's WIRE names, never storage ones.

// Expandable declares one resource an expand path may name, with the wire fields
// its block can carry. A resource absent here cannot be expanded.
type Expandable struct {
	Resource string
	Fields   []string
}

// Expansion is a parsed request: resource name → the fields asked for.
type Expansion map[string]map[string]bool

// Has reports whether the caller asked for this resource at all.
func (e Expansion) Has(resource string) bool { return len(e[resource]) > 0 }

// Wants reports whether the resource's block should carry this field.
func (e Expansion) Wants(resource, field string) bool { return e[resource][field] }

// ParseExpand reads ?expand= against the resources a route offers. ok is false,
// with a 400 already written, for an unknown resource or field name.
func ParseExpand(c *gin.Context, allowed ...Expandable) (Expansion, bool) {
	out := Expansion{}
	raw := strings.TrimSpace(c.Query("expand"))
	if raw == "" {
		return out, true
	}
	for _, path := range strings.Split(raw, ",") {
		resource, field, dotted := strings.Cut(strings.TrimSpace(path), ".")
		i := slices.IndexFunc(allowed, func(e Expandable) bool { return e.Resource == resource })
		if i < 0 {
			httpx.BadRequest(c, "expand: unknown resource "+strconv.Quote(resource))
			return nil, false
		}
		fields := allowed[i].Fields
		if dotted {
			if !slices.Contains(fields, field) {
				httpx.BadRequest(c, "expand: unknown field "+strconv.Quote(resource+"."+field))
				return nil, false
			}
			fields = []string{field}
		}
		if out[resource] == nil {
			out[resource] = map[string]bool{}
		}
		for _, f := range fields {
			out[resource][f] = true
		}
	}
	return out, true
}
