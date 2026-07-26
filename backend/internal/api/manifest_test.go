package api

import (
	"slices"
	"testing"

	"github.com/bitllow/sild/backend/internal/policy"
)

// Every mounted route is declared, and every declaration is mounted. A route
// added without a manifest entry — or a manifest entry left behind after a route
// moves — fails here, which is what makes the declared guard binding rather than
// advisory.
func TestManifestMatchesMountedRoutes(t *testing.T) {
	mounted := mountedRouteKeys(t)
	declared := RouteManifestKeys()

	for _, k := range mounted {
		if !slices.Contains(declared, k) {
			t.Errorf("route %q is mounted but not declared in the manifest", k)
		}
	}
	for _, k := range declared {
		if !slices.Contains(mounted, k) {
			t.Errorf("route %q is declared but not mounted", k)
		}
	}
}

// An action-classified route must name actions the policy catalog knows, or the
// declaration is describing a guard that cannot exist.
func TestManifestActionsExistInPolicy(t *testing.T) {
	known := policy.Actions()
	for _, r := range routeManifest() {
		if r.Class == classAction && len(r.Actions) == 0 {
			t.Errorf("%s %s is action-classified but names no action", r.Method, r.Path)
		}
		for _, a := range r.Actions {
			if !slices.Contains(known, a) {
				t.Errorf("%s %s declares unknown action %q", r.Method, r.Path, a)
			}
		}
	}
}

// Every action route names the principals that may call it — the fact the path
// used to carry and no longer does.
func TestManifestActionRoutesDeclarePrincipals(t *testing.T) {
	for _, r := range routeManifest() {
		if r.Class == classAction && len(r.Principals) == 0 {
			t.Errorf("%s %s is action-classified but names no principals", r.Method, r.Path)
		}
	}
}
