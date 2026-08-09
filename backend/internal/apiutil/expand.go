package apiutil

import (
	"slices"
	"strconv"
	"strings"

	"github.com/bitllow/sild/backend/internal/httpx"
	"github.com/gin-gonic/gin"
)

// The `expand` convention, documented in ARCHITECTURE §5: a comma-separated list
// of `resource`, `resource.field` or `resource.field.key` paths, each adding a
// top-level block beside the response. Field names are the resource's WIRE
// names, never storage ones.

// Expandable declares one resource an expand path may name. Default is what a
// bare resource name yields — the cheap identifying fields, not everything.
// Fields are what a dotted path may additionally name; a field in Keyed may be
// narrowed one level further to a key inside it.
type Expandable struct {
	Resource string
	Default  []string
	Fields   []string
	Keyed    []string
}

// Expansion is a parsed request: resource → field → the keys asked for within
// it. A nil key set means the field whole.
type Expansion map[string]map[string]map[string]bool

// Has reports whether the caller asked for this resource at all.
func (e Expansion) Has(resource string) bool { return len(e[resource]) > 0 }

// Wants reports whether the resource's block should carry this field.
func (e Expansion) Wants(resource, field string) bool { _, ok := e[resource][field]; return ok }

// Keys returns the keys asked for within a field, or nil for the field whole.
func (e Expansion) Keys(resource, field string) map[string]bool { return e[resource][field] }

// ParseExpand reads ?expand= against the resources a route offers. ok is false,
// with a 400 already written, for an unknown resource or field name.
func ParseExpand(c *gin.Context, allowed ...Expandable) (Expansion, bool) {
	out := Expansion{}
	raw := strings.TrimSpace(c.Query("expand"))
	if raw == "" {
		return out, true
	}
	for _, path := range strings.Split(raw, ",") {
		resource, rest, dotted := strings.Cut(strings.TrimSpace(path), ".")
		i := slices.IndexFunc(allowed, func(e Expandable) bool { return e.Resource == resource })
		if i < 0 {
			httpx.BadRequest(c, "expand: unknown resource "+strconv.Quote(resource))
			return nil, false
		}
		spec := allowed[i]
		if out[resource] == nil {
			out[resource] = map[string]map[string]bool{}
		}
		if !dotted {
			for _, f := range spec.Default {
				out[resource][f] = nil // the whole field
			}
			continue
		}
		field, key, keyed := strings.Cut(rest, ".")
		if !slices.Contains(spec.Fields, field) && !slices.Contains(spec.Default, field) {
			httpx.BadRequest(c, "expand: unknown field "+strconv.Quote(resource+"."+field))
			return nil, false
		}
		if keyed && !slices.Contains(spec.Keyed, field) {
			httpx.BadRequest(c, "expand: "+strconv.Quote(resource+"."+field)+" has no keys to select")
			return nil, false
		}
		if key == "" && keyed {
			httpx.BadRequest(c, "expand: empty key in "+strconv.Quote(path))
			return nil, false
		}
		// Whole beats keyed however the paths are ordered: a union that narrowed
		// an already-whole field would return less than the caller asked for.
		keys, whole := out[resource][field]
		if !keyed {
			out[resource][field] = nil
			continue
		}
		if whole && keys == nil {
			continue
		}
		if keys == nil {
			keys = map[string]bool{}
			out[resource][field] = keys
		}
		keys[key] = true
	}
	return out, true
}
