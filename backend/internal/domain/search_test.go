package domain_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/bitllow/sild/backend/internal/domain"
	"github.com/bitllow/sild/backend/internal/store/models"
	"github.com/bitllow/sild/backend/internal/testutil"
)

// §4.3: mixed-token search — free keywords match body + member metadata; field
// qualifiers map to columns; all AND together.
func TestAdminSearch(t *testing.T) {
	h := testutil.New(t)
	tenant := h.SeedTenant("phone") // phone is a searchable metadata key
	ctx := context.Background()

	conv, err := h.Svc.CreateConversation(ctx, tenant.ID, domain.CreateConversationInput{
		Members: []domain.MemberInput{{
			UserID: "u_driver", ConvRole: models.RoleDriver,
			Metadata: json.RawMessage(`{"phone":"+3725512345"}`),
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
		{"refund", true},          // keyword in body
		{"5512", true},            // keyword in member metadata (member_search_text)
		{"status:open", true},     // structured filter matches
		{"status:closed", false},  // structured filter excludes
		{"role:driver", true},     // member role filter
		{"role:client", false},    // wrong role
		{"refund role:driver", true},
		{"refund status:closed", false}, // AND of keyword + filter
	}
	for _, tc := range cases {
		res, err := h.Search.Search(ctx, tenant.ID, tc.q, "", "", 25)
		if err != nil {
			t.Fatalf("search %q: %v", tc.q, err)
		}
		got := len(res.Conversations) > 0
		if got != tc.want {
			t.Errorf("search %q: got %v want %v (%d hits)", tc.q, got, tc.want, len(res.Conversations))
		}
	}
}
