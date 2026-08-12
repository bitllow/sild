package domain_test

import (
	"context"
	"testing"

	"github.com/bitllow/sild/backend/internal/domain"
	"github.com/bitllow/sild/backend/internal/policy"
	"github.com/bitllow/sild/backend/internal/principal"
	"github.com/bitllow/sild/backend/internal/store/models"
	"github.com/bitllow/sild/backend/internal/testutil"
)

// §4.3: mixed-token search — free keywords match body + member metadata; field
// qualifiers map to columns; all AND together.
func TestAdminSearch(t *testing.T) {
	h := testutil.New(t)
	tenant := h.SeedTenant("phone") // phone is a searchable metadata key
	ctx := context.Background()
	h.SeedContact(tenant.ID, "u_driver", `{"phone":"+3725512345"}`)

	conv, err := h.Svc.CreateConversation(ctx, tenant.ID, domain.CreateConversationInput{
		OpenAssignment: true,
		Members: []domain.MemberInput{{
			UserID: "u_driver", ConvRole: models.RoleDriver,
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	ext := "u_driver"
	if _, err := h.Svc.SendMessage(ctx, tenant.ID, conv.ID, domain.SendInput{
		SenderKind: models.SenderUser, External: &ext, Body: "please process my refund",
	}); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		q    string
		want bool
	}{
		{"refund", true},         // keyword in body
		{"5512", true},           // keyword in member profile (contacts.search_text)
		{"status:open", true},    // structured filter matches
		{"status:closed", false}, // structured filter excludes
		{"role:driver", true},    // member role filter
		{"role:client", false},   // wrong role
		{"refund role:driver", true},
		{"refund status:closed", false}, // AND of keyword + filter
	}
	for _, tc := range cases {
		res, err := h.Search.Search(ctx, tenant.ID, supportScope(), domain.SearchInput{Query: tc.q, Limit: 25})
		if err != nil {
			t.Fatalf("search %q: %v", tc.q, err)
		}
		got := len(res.Conversations) > 0
		if got != tc.want {
			t.Errorf("search %q: got %v want %v (%d hits)", tc.q, got, tc.want, len(res.Conversations))
		}
	}
}

// §4.3: generic meta.<key> must work via live-JSON even for a key that is NOT in
// the tenant's searchable_metadata_keys (so it isn't in contacts.search_text).
func TestAdminSearchLiveJSONFallback(t *testing.T) {
	h := testutil.New(t)
	tenant := h.SeedTenant("phone") // only "phone" is indexed; "city" is not
	ctx := context.Background()
	h.SeedContact(tenant.ID, "u_driver", `{"phone":"+3725512345","city":"Tallinn"}`)

	if _, err := h.Svc.CreateConversation(ctx, tenant.ID, domain.CreateConversationInput{
		OpenAssignment: true,
		Members: []domain.MemberInput{{
			UserID: "u_driver", ConvRole: models.RoleDriver,
		}},
	}); err != nil {
		t.Fatal(err)
	}

	// "city" is not materialized into contacts.search_text, but the live-JSON
	// fallback finds it.
	res, err := h.Search.Search(ctx, tenant.ID, supportScope(), domain.SearchInput{Query: "meta.city:tallinn", Limit: 25})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Conversations) == 0 {
		t.Fatal("expected live-JSON fallback to match an unindexed metadata key")
	}
}

// §4.3 privacy: a bare keyword in the DEFAULT (support) search must match member
// metadata only through the tenant's configured searchable_metadata_keys — never
// the raw metadata JSON. Otherwise a value the tenant deliberately kept out of
// the allowlist (here "city") would be recoverable by any agent typing it. The
// qualified meta.<key> form still reaches it (see TestAdminSearchLiveJSONFallback);
// this guards only the un-namespaced free-text path.
func TestSupportKeywordDoesNotMatchUnindexedMetadata(t *testing.T) {
	h := testutil.New(t)
	tenant := h.SeedTenant("phone") // only "phone" is searchable; "city" is not
	ctx := context.Background()
	h.SeedContact(tenant.ID, "u_driver", `{"phone":"+3725512345","city":"Tallinn"}`)

	if _, err := h.Svc.CreateConversation(ctx, tenant.ID, domain.CreateConversationInput{
		OpenAssignment: true,
		Members: []domain.MemberInput{{
			UserID: "u_driver", ConvRole: models.RoleDriver,
		}},
	}); err != nil {
		t.Fatal(err)
	}

	// A bare keyword matching only the unindexed "city" value must NOT hit.
	res, err := h.Search.Search(ctx, tenant.ID, supportScope(), domain.SearchInput{Query: "Tallinn", Limit: 25})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Conversations) != 0 {
		t.Fatalf("support keyword search leaked an unindexed metadata value: %d hits", len(res.Conversations))
	}

	// The configured key still matches, so search isn't simply broken.
	if res, err := h.Search.Search(ctx, tenant.ID, supportScope(), domain.SearchInput{Query: "5512", Limit: 25}); err != nil || len(res.Conversations) != 1 {
		t.Fatalf("configured key search: hits=%d err=%v, want 1/nil", len(res.Conversations), err)
	}
}

// supportScope is an operator without peer_access: support conversations only.
func supportScope() policy.ResourceScope {
	return policy.Scope(&principal.Principal{TenantID: "t", Kind: principal.KindAdmin,
		AdminID: "a", Assignments: principal.Held(models.PlatformOwner)}, policy.ConversationsList)
}
