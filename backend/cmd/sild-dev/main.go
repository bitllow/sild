// Command sild-dev runs the whole backend — REST + realtime + worker — in ONE
// process with the in-memory broker and SQLite. Zero infra, no external tools:
//
//	make dev      # or: go run ./cmd/sild-dev
//
// Because it's a single process, the realtime publisher and the WS handler share
// one in-memory Centrifuge node, so realtime works end-to-end without Redis.
// Production uses the four separate binaries (ARCHITECTURE §3a); this is dev-only.
package main

import (
	"context"
	"log"
	"os/signal"
	"syscall"
	"time"

	"github.com/bitllow/sild/backend/internal/auth"
	"github.com/bitllow/sild/backend/internal/config"
	"github.com/bitllow/sild/backend/internal/connector/webhook"
	"github.com/bitllow/sild/backend/internal/di"
	"github.com/bitllow/sild/backend/internal/domain"
	"github.com/bitllow/sild/backend/internal/realtime"
	"github.com/bitllow/sild/backend/internal/server"
	"github.com/bitllow/sild/backend/internal/store"
	"github.com/bitllow/sild/backend/internal/store/gormstore"
	"github.com/bitllow/sild/backend/internal/store/models"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	c, err := di.New()
	if err != nil {
		log.Fatalf("di: %v", err)
	}

	err = c.Invoke(func(
		cfg *config.Config, db *gorm.DB, km *auth.KeyManager, svc *domain.Service,
		srv *server.Server, node *realtime.Node, relay *webhook.Relay, st store.Store,
	) error {
		if err := gormstore.Migrate(db); err != nil {
			return err
		}
		if err := km.EnsureActiveKey(ctx); err != nil {
			return err
		}
		if err := node.Run(); err != nil { // idempotent; shared with the publisher
			return err
		}

		// Mount the WS/SSE transport on the same server (single port).
		srv.Engine().GET("/v1/ws", gin.WrapH(node.WSHandler()))
		srv.Engine().GET("/v1/ws/sse", gin.WrapH(node.SSEHandler()))

		devSeed(ctx, st, svc)

		// Background webhook relay (in-process worker).
		go func() {
			t := time.NewTicker(5 * time.Second)
			defer t.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-t.C:
					_, _ = relay.ProcessOnce(ctx, 100)
				}
			}
		}()

		log.Printf("sild-dev: REST+WS on %s (sqlite, in-memory broker) — Ctrl-C to stop", cfg.HTTPAddr)
		return srv.Run(ctx)
	})
	if err != nil {
		log.Fatalf("sild-dev: %v", err)
	}
}

// devSeed creates a ready-to-use tenant + owner admin + API key on first run so
// you can log into the inbox (email/password) and call the API immediately.
func devSeed(ctx context.Context, st store.Store, svc *domain.Service) {
	ids, err := st.Tenants().AllIDs(ctx)
	if err != nil || len(ids) > 0 {
		return // already seeded
	}
	t := &models.Tenant{Name: "Dev Tenant", MaxAttachmentBytes: 10 << 20}
	if err := st.Tenants().Create(ctx, t); err != nil {
		log.Printf("dev seed: %v", err)
		return
	}
	admin, err := svc.InviteAgent(ctx, t.ID, "admin@sild.local", models.PlatformOwner)
	if err != nil {
		log.Printf("dev seed admin: %v", err)
		return
	}
	_ = svc.SetAdminPassword(ctx, t.ID, admin.ID, "password123")
	key, _, err := svc.CreateAPIKey(ctx, t.ID, "dev")
	if err != nil {
		log.Printf("dev seed key: %v", err)
		return
	}
	log.Printf("┌─ dev seed ────────────────────────────────────────────")
	log.Printf("│ tenant_id : %s", t.ID)
	log.Printf("│ admin     : admin@sild.local / password123  (POST /v1/admin/auth/password)")
	log.Printf("│ api key   : %s", key)
	log.Printf("└───────────────────────────────────────────────────────")
}
