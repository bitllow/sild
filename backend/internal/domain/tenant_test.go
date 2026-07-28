package domain_test

import (
	"context"
	"testing"

	"github.com/bitllow/sild/backend/internal/store/models"
	"github.com/bitllow/sild/backend/internal/testutil"
)

func TestCreateTenantRejectsBlankName(t *testing.T) {
	h := testutil.New(t)
	for _, name := range []string{"", "   "} {
		if _, err := h.Svc.CreateTenant(context.Background(), name); err == nil {
			t.Fatalf("created a tenant named %q", name)
		}
	}
}

func TestCreateTenantSetsTheAttachmentCap(t *testing.T) {
	h := testutil.New(t)
	tenant, err := h.Svc.CreateTenant(context.Background(), "  Acme  ")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if tenant.Name != "Acme" {
		t.Fatalf("name = %q, want trimmed", tenant.Name)
	}
	// Zero here would reject every upload as over the limit (§11).
	if tenant.MaxAttachmentBytes != 10<<20 {
		t.Fatalf("MaxAttachmentBytes = %d", tenant.MaxAttachmentBytes)
	}
}

// An unchecked role stores an operator no permission check ever matches: they
// appear on the team list and can do nothing. Reachable over HTTP as well as from
// the CLI, since neither validated it.
func TestInviteAgentRejectsAnUnknownRole(t *testing.T) {
	h := testutil.New(t)
	tenant := h.SeedTenant()
	if _, err := h.Svc.InviteAgent(context.Background(), tenant.ID, "a@b.test", "", "", models.PlatformRole("superuser")); err == nil {
		t.Fatal("invited an operator with an unknown platform role")
	}
	for _, role := range []models.PlatformRole{models.PlatformOwner, models.PlatformAdmin, models.PlatformAgent, ""} {
		email := string(role) + "@b.test"
		if _, err := h.Svc.InviteAgent(context.Background(), tenant.ID, email, "", "", role); err != nil {
			t.Fatalf("role %q rejected: %v", role, err)
		}
	}
}

func TestListTenants(t *testing.T) {
	h := testutil.New(t)
	ctx := context.Background()
	first, err := h.Svc.CreateTenant(ctx, "First")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := h.Svc.CreateTenant(ctx, "Second"); err != nil {
		t.Fatalf("create: %v", err)
	}
	tenants, err := h.Svc.ListTenants(ctx)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(tenants) != 2 {
		t.Fatalf("listed %d tenants, want 2", len(tenants))
	}
	if tenants[0].ID != first.ID {
		t.Fatalf("listing is not in creation order: %s first", tenants[0].Name)
	}
}

func TestFindAdminByEmailIsTenantScoped(t *testing.T) {
	h := testutil.New(t)
	ctx := context.Background()
	a := h.SeedTenant()
	b := h.SeedTenant()
	h.SeedAdmin(a.ID, "shared@acme.test", models.PlatformOwner)

	if _, err := h.Svc.FindAdminByEmail(ctx, a.ID, "shared@acme.test"); err != nil {
		t.Fatalf("own tenant: %v", err)
	}
	// The same email may be an operator in several tenants; an operator command
	// addressed to one must never resolve to another's record.
	if _, err := h.Svc.FindAdminByEmail(ctx, b.ID, "shared@acme.test"); err == nil {
		t.Fatal("resolved an admin from a different tenant")
	}
}
