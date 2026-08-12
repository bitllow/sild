package provision_test

import (
	"context"
	"strings"
	"sync"
	"testing"

	"github.com/bitllow/sild/backend/internal/provision"
	"github.com/bitllow/sild/backend/internal/store/models"
	"github.com/bitllow/sild/backend/internal/testutil"
)

// The three things a new customer needs, from one command. Before this the only
// paths to a tenant were the dev seed and hand-written SQL.
func TestTenantCreatesAUsableTenant(t *testing.T) {
	h := testutil.New(t)
	ctx := context.Background()

	res, err := provision.Tenant(ctx, h.Svc, provision.TenantSpec{
		Name: "Acme", AdminEmail: "owner@acme.test", AdminName: "Eva Marleen",
		AdminPassword: "correct-horse", APIKeyLabel: "ci",
	})
	if err != nil {
		t.Fatalf("provision: %v", err)
	}
	if res.TenantID == "" || res.APIKey == "" || res.ForwardingAddress == "" {
		t.Fatalf("incomplete result: %+v", res)
	}

	// The owner can sign in — the point of the whole command.
	if _, _, err := h.Svc.CreateSessionWithPassword(ctx, "owner@acme.test", "correct-horse"); err != nil {
		t.Fatalf("owner cannot sign in: %v", err)
	}
	admin, err := h.Svc.GetAdmin(ctx, res.TenantID, res.AdminID)
	if err != nil {
		t.Fatalf("get owner: %v", err)
	}
	held, err := h.Svc.RoleAssignments(ctx, res.TenantID, res.AdminID)
	if err != nil {
		t.Fatalf("owner assignments: %v", err)
	}
	roles := map[models.PlatformRole]models.RoleScope{}
	for _, a := range held {
		roles[a.Role] = a.Scope
	}
	if _, ok := roles[models.PlatformOwner]; !ok {
		t.Fatalf("the first member is not an owner: %v", held)
	}
	if !roles[models.PlatformAgent].Peer {
		t.Fatal("the owner cannot see peer conversations")
	}
	if admin.FirstName != "Eva" || admin.LastName != "Marleen" {
		t.Fatalf("name split wrong: %q %q", admin.FirstName, admin.LastName)
	}
}

// An owner with no password is legitimate — they sign in through Google OIDC —
// but the tenant must still come out complete.
func TestTenantWithoutPassword(t *testing.T) {
	h := testutil.New(t)
	res, err := provision.Tenant(context.Background(), h.Svc, provision.TenantSpec{
		Name: "Acme", AdminEmail: "owner@acme.test",
	})
	if err != nil {
		t.Fatalf("provision: %v", err)
	}
	if res.APIKey == "" {
		t.Fatal("no api key")
	}
	if _, _, err := h.Svc.CreateSessionWithPassword(context.Background(), "owner@acme.test", ""); err == nil {
		t.Fatal("an owner with no password signed in with an empty one")
	}
}

func TestTenantRejectsIncompleteSpec(t *testing.T) {
	h := testutil.New(t)
	cases := []struct {
		name string
		spec provision.TenantSpec
		want string
	}{
		{"no name", provision.TenantSpec{AdminEmail: "a@b.test"}, "name"},
		{"no admin email", provision.TenantSpec{Name: "Acme"}, "email"},
		{"password too short", provision.TenantSpec{Name: "Acme", AdminEmail: "a@b.test", AdminPassword: "short"}, "8 characters"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := provision.Tenant(context.Background(), h.Svc, tc.spec)
			if err == nil {
				t.Fatal("accepted")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error does not mention %q: %v", tc.want, err)
			}
		})
	}
}

// Provisioning is several transactions, so a spec that fails partway would leave a
// tenant with no owner and no key — and Bootstrap skips whenever a tenant exists,
// so that deployment could never be bootstrapped again. Nothing may be written
// until the whole spec is known to be good.
func TestARejectedSpecWritesNothing(t *testing.T) {
	h := testutil.New(t)
	// A password below the minimum is the realistic operator mistake: it is only
	// rejected at the step after the tenant and the owner already exist.
	_, err := provision.Tenant(context.Background(), h.Svc, provision.TenantSpec{
		Name: "Acme", AdminEmail: "owner@acme.test", AdminPassword: "short",
	})
	if err == nil {
		t.Fatal("accepted a password below the minimum")
	}
	assertTenantCount(t, h, 0)

	// And bootstrap is still able to run, which is the property that matters.
	res, err := provision.Bootstrap(context.Background(), h.Svc, h.Store, provision.TenantSpec{
		Name: "Acme", AdminEmail: "owner@acme.test", AdminPassword: "correct-horse",
	})
	if err != nil || res == nil {
		t.Fatalf("bootstrap after a rejected spec: res=%v err=%v", res, err)
	}
}

// Bootstrap is a first-run convenience, so it must be inert everywhere else: a
// restart, a redeploy, or an extra replica must never add a second tenant or
// touch the one that is there.
func TestBootstrapOnlyFiresOnAnEmptyDatabase(t *testing.T) {
	h := testutil.New(t)
	ctx := context.Background()
	spec := provision.TenantSpec{Name: "Acme", AdminEmail: "owner@acme.test", AdminPassword: "correct-horse"}

	if res, err := provision.Bootstrap(ctx, h.Svc, h.Store, spec); err != nil || res == nil {
		t.Fatalf("first bootstrap: res=%v err=%v", res, err)
	}

	again, err := provision.Bootstrap(ctx, h.Svc, h.Store, spec)
	if err != nil {
		t.Fatalf("second bootstrap: %v", err)
	}
	if again != nil {
		t.Fatal("bootstrap ran again on a populated database")
	}
	assertTenantCount(t, h, 1)

	// And it leaves the existing tenant alone rather than resetting it.
	if _, _, err := h.Svc.CreateSessionWithPassword(ctx, "owner@acme.test", "correct-horse"); err != nil {
		t.Fatalf("the bootstrapped owner stopped working: %v", err)
	}
}

// Unconfigured bootstrap must be a no-op, not an error: every standalone replica
// runs this code path on every start, and most deployments never set the vars.
func TestBootstrapWithoutConfigDoesNothing(t *testing.T) {
	h := testutil.New(t)
	res, err := provision.Bootstrap(context.Background(), h.Svc, h.Store, provision.TenantSpec{})
	if err != nil {
		t.Fatalf("unconfigured bootstrap errored: %v", err)
	}
	if res != nil {
		t.Fatal("bootstrapped with no configuration")
	}
	assertTenantCount(t, h, 0)
}

// Replicas start together, and each sees an empty tenant table. Exactly one may
// create the tenant — the lease is what makes the others stand down.
func TestConcurrentBootstrapCreatesOneTenant(t *testing.T) {
	h := testutil.New(t)
	spec := provision.TenantSpec{Name: "Acme", AdminEmail: "owner@acme.test", AdminPassword: "correct-horse"}

	const replicas = 4
	var wg sync.WaitGroup
	created := make([]bool, replicas)
	errs := make([]error, replicas)
	start := make(chan struct{})
	for i := range replicas {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			res, err := provision.Bootstrap(context.Background(), h.Svc, h.Store, spec)
			created[i], errs[i] = res != nil, err
		}()
	}
	close(start)
	wg.Wait()

	wins := 0
	for i := range replicas {
		if errs[i] != nil {
			t.Errorf("replica %d: %v", i, errs[i])
		}
		if created[i] {
			wins++
		}
	}
	if wins != 1 {
		t.Fatalf("%d replicas created a tenant, want exactly 1", wins)
	}
	assertTenantCount(t, h, 1)
}

func assertTenantCount(t *testing.T, h *testutil.Harness, want int) {
	t.Helper()
	ids, err := h.Store.Tenants().AllIDs(context.Background())
	if err != nil {
		t.Fatalf("list tenants: %v", err)
	}
	if len(ids) != want {
		t.Fatalf("tenant count = %d, want %d", len(ids), want)
	}
}
