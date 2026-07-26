package api

import (
	"slices"
	"strings"
	"testing"

	"github.com/bitllow/sild/backend/internal/middleware"
	"github.com/bitllow/sild/backend/internal/principal"
)

// Mount derives each guard from its descriptor, so a descriptor missing a field
// is now a mounted route missing a guard. These assert the descriptors are
// complete enough to derive from.

func TestEveryRouteDeclaresAHandler(t *testing.T) {
	for _, r := range routeManifest() {
		if r.Handler == nil {
			t.Errorf("%s %s declares no handler", r.Method, r.Path)
		}
	}
}

// credentialGuard panics on a principal set it cannot translate. Reaching it for
// every declared route proves no route is mounted through that panic — or worse,
// through a set nobody noticed was unguardable.
func TestEveryActionRouteHasADerivableGuard(t *testing.T) {
	guardable := [][]principal.Kind{anyPrincipal, keyOnly, userOnly, adminOnly, signedOnly}
	for _, r := range routeManifest() {
		if r.Class != classAction {
			continue
		}
		if !slices.ContainsFunc(guardable, func(k []principal.Kind) bool { return slices.Equal(k, r.Principals) }) {
			t.Errorf("%s %s declares principals %v, which Mount cannot translate into a guard",
				r.Method, r.Path, r.Principals)
		}
	}
}

// A route reading a body larger than the JSON default must say so. The cap is
// per-route because a group cap can only lower a nested one, so an undeclared
// raw path would be silently truncated to 256 KiB.
func TestRawBodyRoutesDeclareTheirOwnCap(t *testing.T) {
	raw := map[string]int64{
		"POST /v1/email/inbound":           middleware.BodyLimitEmail,
		"PUT /v1/uploads/local/*objectKey": middleware.BodyLimitUpload,
	}
	for _, r := range routeManifest() {
		want, ok := raw[r.Method+" "+r.Path]
		if !ok {
			if r.BodyLimit != 0 && r.BodyLimit != middleware.BodyLimitJSON {
				t.Errorf("%s %s declares a non-default cap %d but is not a known raw-body route",
					r.Method, r.Path, r.BodyLimit)
			}
			continue
		}
		if r.bodyLimit() != want {
			t.Errorf("%s %s caps at %d, want %d", r.Method, r.Path, r.bodyLimit(), want)
		}
	}
}

// Rate limiting is a security control on reachable-by-anyone routes, so the set
// that declares a class is pinned rather than left to drift.
func TestRateClassesArePinnedToTheRoutesThatNeedThem(t *testing.T) {
	want := map[string]rateClass{
		"POST /v1/admin/auth/password":     rateAuth,
		"POST /v1/tokens":                  rateAuth,
		"POST /v1/email/inbound":           rateIngress,
		"PUT /v1/uploads/local/*objectKey": rateIngress,
	}
	for _, r := range routeManifest() {
		key := r.Method + " " + r.Path
		if got := r.Rate; got != want[key] {
			t.Errorf("%s declares rate class %q, want %q", key, got, want[key])
		}
	}
}

// The document is generated, so it cannot omit a served route or invent one.
func TestOpenAPIDocumentCoversEveryMountedRoute(t *testing.T) {
	h := newManifestProbeHandler(t)
	doc := h.openAPIDocument()
	paths, _ := doc["paths"].(map[string]any)
	if len(paths) == 0 {
		t.Fatal("generated document has no paths")
	}

	for _, r := range routeManifest() {
		if r.Enabled != nil && !r.Enabled(h) {
			continue
		}
		item, ok := paths[openAPIPath(r.Path)].(map[string]any)
		if !ok {
			t.Errorf("%s %s is mounted but absent from the document", r.Method, r.Path)
			continue
		}
		op, ok := item[strings.ToLower(r.Method)].(map[string]any)
		if !ok {
			t.Errorf("%s %s is mounted but its method is absent from the document", r.Method, r.Path)
			continue
		}
		if op["operationId"] == "" {
			t.Errorf("%s %s has no operationId", r.Method, r.Path)
		}
		// gin's syntax must not leak into the document.
		if tmpl := openAPIPath(r.Path); strings.ContainsAny(tmpl, ":*") {
			t.Errorf("path %q still carries gin parameter syntax", tmpl)
		}
	}
}

// The security block is what a reader trusts the document for, so it must match
// the principals the router actually enforces.
func TestOpenAPISecurityMatchesDeclaredPrincipals(t *testing.T) {
	for _, r := range routeManifest() {
		if r.Class != classAction {
			continue
		}
		sec := r.openAPISecurity()
		if len(sec) == 0 {
			t.Errorf("%s %s is action-classified but documents no credential", r.Method, r.Path)
			continue
		}
		for _, kind := range r.Principals {
			name := securitySchemeFor[kind]
			if !slices.ContainsFunc(sec, func(m map[string][]string) bool { _, ok := m[name]; return ok }) {
				t.Errorf("%s %s accepts %s but the document omits scheme %q",
					r.Method, r.Path, kind, name)
			}
		}
	}
}

// Every scheme the operations reference must be defined, or a generated client
// cannot authenticate at all.
func TestOpenAPIReferencedSchemesAreDefined(t *testing.T) {
	defined := openAPISecuritySchemes()
	for _, r := range routeManifest() {
		for _, req := range r.openAPISecurity() {
			for name := range req {
				if _, ok := defined[name]; !ok {
					t.Errorf("%s %s references undefined security scheme %q", r.Method, r.Path, name)
				}
			}
		}
	}
}
