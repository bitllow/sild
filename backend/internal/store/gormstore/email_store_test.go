package gormstore_test

import (
	"context"
	"errors"
	"testing"

	"github.com/bitllow/sild/backend/internal/store"
	"github.com/bitllow/sild/backend/internal/store/models"
)

// FindByInboundToken resolves a tenant by its forwarding token; an unknown or
// empty token is a clean not-found.
func TestFindByInboundToken(t *testing.T) {
	for _, dbc := range dialects(t) {
		t.Run(string(dbc.Driver), func(t *testing.T) {
			const tenant = "t_inbound_token"
			st := newQueueStore(t, dbc, tenant)
			ctx := context.Background()
			if err := st.Tenants().SetEmailConfig(ctx, &models.TenantEmailConfig{
				TenantID: tenant, InboundToken: "eml_abc123",
			}); err != nil {
				t.Fatalf("set email config: %v", err)
			}
			t.Cleanup(func() { _ = st.Tenants().SetEmailConfig(ctx, &models.TenantEmailConfig{TenantID: tenant}) })

			got, err := st.Tenants().FindByInboundToken(ctx, "eml_abc123")
			if err != nil {
				t.Fatalf("find: %v", err)
			}
			if got.TenantID != tenant {
				t.Fatalf("resolved tenant %q, want %q", got.TenantID, tenant)
			}
			if _, err := st.Tenants().FindByInboundToken(ctx, "eml_nope"); !errors.Is(err, store.ErrNotFound) {
				t.Fatalf("unknown token: want ErrNotFound, got %v", err)
			}
			if _, err := st.Tenants().FindByInboundToken(ctx, ""); !errors.Is(err, store.ErrNotFound) {
				t.Fatalf("empty token: want ErrNotFound, got %v", err)
			}
		})
	}
}

// CountOpen counts only open conversations in the tenant.
func TestCountOpenConversations(t *testing.T) {
	for _, dbc := range dialects(t) {
		t.Run(string(dbc.Driver), func(t *testing.T) {
			const tenant = "t_count_open"
			st := newQueueStore(t, dbc, tenant)
			ctx := context.Background()

			mk := func(id string, status models.ConversationStatus) {
				if err := st.Conversations().Create(ctx, &models.Conversation{ID: id, TenantID: tenant, Status: status}); err != nil {
					t.Fatalf("create conv: %v", err)
				}
			}
			mk(tenant+"_o1", models.ConversationOpen)
			mk(tenant+"_o2", models.ConversationOpen)
			mk(tenant+"_c1", models.ConversationClosed)

			n, err := st.Conversations().CountOpen(ctx, tenant)
			if err != nil {
				t.Fatalf("count: %v", err)
			}
			if n != 2 {
				t.Fatalf("open count = %d, want 2", n)
			}
		})
	}
}
