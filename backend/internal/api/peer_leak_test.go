package api_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/bitllow/sild/backend/internal/domain"
	"github.com/bitllow/sild/backend/internal/store/models"
	"github.com/bitllow/sild/backend/internal/testutil"
)

// An operator without peer_access must see zero peer rows, by every door into
// the list — the guard on the kind boundary policy.Scope owns.
func TestUnifiedListNeverLeaksPeerToNonPeerOperator(t *testing.T) {
	h := testutil.New(t)
	tenant := h.SeedTenant("plate")
	ctx := context.Background()

	h.SeedAdmin(tenant.ID, "owner@test", models.PlatformOwner)
	h.SeedAdmin(tenant.ID, "agent@test", models.PlatformAgent)
	agent := loginAs(t, h, "agent@test")

	peer := mkPeer(t, h, tenant.ID, "trip_secret")
	rider := "u_rider_trip_secret"
	if _, err := h.Svc.SendMessage(ctx, tenant.ID, peer.ID, domain.SendInput{
		Body: "peerneedle", SenderKind: models.SenderUser, External: &rider,
	}); err != nil {
		t.Fatalf("seed peer message: %v", err)
	}
	mkSupport(t, h, tenant.ID, "support_visible")

	probes := []struct {
		name string
		path string
	}{
		{"explicit kind=peer", "/v1/conversations?kind=peer"},
		{"unfiltered list", "/v1/conversations"},
		{"search matching a peer message body", "/v1/conversations?q=peerneedle"},
		{"search matching peer member metadata", "/v1/conversations?q=trip_secret"},
		{"contacts list", "/v1/contacts"},
		{"contacts search", "/v1/contacts?q=trip_secret"},
	}

	for _, p := range probes {
		t.Run(p.name, func(t *testing.T) {
			w := h.Request("GET", p.path).Cookie("sild_admin", agent).Do()
			if w.Code != http.StatusOK {
				t.Fatalf("%s: %d %s", p.path, w.Code, w.Body)
			}
			var page struct {
				Items []map[string]any `json:"items"`
			}
			testutil.DecodeJSON(t, w, &page)
			for _, it := range page.Items {
				if id, _ := it["id"].(string); id == peer.ID {
					t.Fatalf("peer conversation leaked via %s", p.path)
				}
				if kind, _ := it["kind"].(string); kind == string(models.KindPeer) {
					t.Fatalf("peer-kind row leaked via %s: %v", p.path, it)
				}
			}
		})
	}
}

// Proves the probes above reach real data rather than passing on an empty set.
func TestPeerAccessOperatorSeesPeerRows(t *testing.T) {
	h := testutil.New(t)
	tenant := h.SeedTenant()

	admin := h.SeedAdmin(tenant.ID, "owner@test", models.PlatformOwner)
	h.GrantPeer(tenant.ID, admin.ID)
	owner := loginAs(t, h, "owner@test")
	peer := mkPeer(t, h, tenant.ID, "trip_visible")

	w := h.Request("GET", "/v1/conversations?kind=peer").Cookie("sild_admin", owner).Do()
	if w.Code != http.StatusOK {
		t.Fatalf("peer list: %d %s", w.Code, w.Body)
	}
	var page struct {
		Items []map[string]any `json:"items"`
	}
	testutil.DecodeJSON(t, w, &page)
	found := false
	for _, it := range page.Items {
		if id, _ := it["id"].(string); id == peer.ID {
			found = true
		}
	}
	if !found {
		t.Fatalf("peer_access operator cannot see the peer conversation: %+v", page.Items)
	}
}
