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
	"encoding/json"
	"log"
	"os/signal"
	"syscall"
	"time"

	"github.com/bitllow/sild/backend/internal/auth"
	"github.com/bitllow/sild/backend/internal/config"
	"github.com/bitllow/sild/backend/internal/connector/webhook"
	"github.com/bitllow/sild/backend/internal/di"
	"github.com/bitllow/sild/backend/internal/domain"
	"github.com/bitllow/sild/backend/internal/mail"
	"github.com/bitllow/sild/backend/internal/realtime"
	"github.com/bitllow/sild/backend/internal/server"
	"github.com/bitllow/sild/backend/internal/store"
	"github.com/bitllow/sild/backend/internal/store/gormstore"
	"github.com/bitllow/sild/backend/internal/store/models"
	"github.com/bitllow/sild/backend/internal/webasset"
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

		// Phase 3 drop-in: /widget.js is mounted from the embedded bundle in
		// handler.Mount (so sild-api serves it too). Here we add the dev-only
		// faux host page, also embedded.
		srv.Engine().GET("/sild-demo", func(c *gin.Context) {
			c.Data(200, "text/html; charset=utf-8", webasset.Demo)
		})

		// Dev-only token mint standing in for the host backend's tokenProvider
		// endpoint: mints a user JWT for a (guest) id in the dev tenant. Never
		// exposes the API key. Production hosts mint via POST /v1/tokens.
		srv.Engine().GET("/v1/dev/widget-token", func(c *gin.Context) {
			uid := c.Query("user_id")
			if uid == "" {
				uid = "guest_demo"
			}
			ids, err := st.Tenants().AllIDs(c.Request.Context())
			if err != nil || len(ids) == 0 {
				c.JSON(500, gin.H{"error": "no tenant"})
				return
			}
			tok, exp, err := km.Mint(c.Request.Context(), uid, ids[0], time.Hour)
			if err != nil {
				c.JSON(500, gin.H{"error": err.Error()})
				return
			}
			c.JSON(200, gin.H{"token": tok, "expires_at": exp})
		})

		devSeed(ctx, st, svc, cfg)

		// Email forwarding ingestion daemon, in-process so `make dev` exercises
		// the full loop (forwarded mail → conversation) without a separate binary.
		go func() {
			if err := mail.Serve(ctx, cfg.Email.SMTPListenAddr, svc.ForwardedMailHandler()); err != nil {
				log.Printf("sild-dev: smtp receiver: %v", err)
			}
		}()

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

		log.Printf("sild-dev: REST+WS on %s (db=%s, broker=%s) — Ctrl-C to stop", cfg.HTTPAddr, cfg.DB.Driver, cfg.Realtime.Broker)
		return srv.Run(ctx)
	})
	if err != nil {
		log.Fatalf("sild-dev: %v", err)
	}
}

// devSeed creates a ready-to-use tenant + owner admin + API key on first run so
// you can log into the inbox (email/password) and call the API immediately.
func devSeed(ctx context.Context, st store.Store, svc *domain.Service, cfg *config.Config) {
	ids, err := st.Tenants().AllIDs(ctx)
	if err != nil || len(ids) > 0 {
		if len(ids) > 0 {
			if ch, err := svc.GetEmailChannel(ctx, ids[0]); err == nil {
				log.Printf("sild-dev: forward email to %s (SMTP %s) to open a conversation", ch.ForwardingAddress, cfg.Email.SMTPListenAddr)
			}
		}
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
	devSeedConversations(ctx, svc, t.ID, admin.ID)
	fwd := ""
	if ch, err := svc.GetEmailChannel(ctx, t.ID); err == nil {
		fwd = ch.ForwardingAddress
	}
	log.Printf("┌─ dev seed ────────────────────────────────────────────")
	log.Printf("│ tenant_id : %s", t.ID)
	log.Printf("│ admin     : admin@sild.local / password123  (POST /v1/admin/auth/password)")
	log.Printf("│ api key   : %s", key)
	log.Printf("│ inbox     : 5 sample support requests seeded")
	log.Printf("│ email in  : forward to %s (SMTP %s)", fwd, cfg.Email.SMTPListenAddr)
	log.Printf("└───────────────────────────────────────────────────────")
}

func strptr(s string) *string { return &s }

// devSeedConversations populates the assignment queue with the sample
// conversations from the design so the inbox shows data on first run.
func devSeedConversations(ctx context.Context, svc *domain.Service, tenantID, adminID string) {
	user := func(convID, uid, body string, ch models.Channel) {
		_, _ = svc.SendMessage(ctx, tenantID, convID, domain.SendInput{
			SenderKind: models.SenderUser, External: strptr(uid), Body: body, Channel: ch,
		})
	}
	agent := func(convID, body string) {
		_, _ = svc.SendMessage(ctx, tenantID, convID, domain.SendInput{
			SenderKind: models.SenderAgent, Internal: strptr(adminID), Body: body,
		})
	}
	note := func(convID, body string) {
		_, _ = svc.SendMessage(ctx, tenantID, convID, domain.SendInput{
			SenderKind: models.SenderAgent, Internal: strptr(adminID), Body: body,
			Visibility: models.VisibilityInternal, AllowInternal: true,
		})
	}
	create := func(ref string, meta json.RawMessage, members []domain.MemberInput) *models.Conversation {
		conv, err := svc.CreateConversation(ctx, tenantID, domain.CreateConversationInput{
			Reference: ref, Metadata: meta, Members: members, OpenAssignment: true,
		})
		if err != nil {
			log.Printf("dev seed conv %s: %v", ref, err)
			return nil
		}
		return conv
	}
	claim := func(conv *models.Conversation) {
		if conv != nil && conv.Assignment != nil {
			_, _ = svc.ClaimAssignment(ctx, tenantID, conv.Assignment.ID, adminID)
		}
	}

	// 1. Mari Tamm — claimed (assigned), client + driver, internal note.
	mari := create("trip_8842", json.RawMessage(`{"kind":"ride"}`), []domain.MemberInput{
		{UserID: "u_mari", ConvRole: models.RoleClient, Metadata: json.RawMessage(`{"name":"Mari Tamm","phone":"+372 5123 4567","app_version":"2.3.1","role":"client"}`)},
		{UserID: "u_driver9", ConvRole: models.RoleDriver, Metadata: json.RawMessage(`{"name":"Driver 9","phone":"+372 5987 6543","app_version":"2.3.0","role":"driver"}`)},
	})
	if mari != nil {
		user(mari.ID, "u_mari", "Hi — my driver still hasn't arrived and the app says they're 2 min away for the last 10 minutes.", models.ChannelApp)
		agent(mari.ID, "Hi Mari, sorry about that. Let me check with the driver right now.")
		note(mari.ID, "VIP rider — escalate to dispatch if not moving in 5 min.")
		user(mari.ID, "u_mari", "Thank you. I have a flight to catch.", models.ChannelApp)
		claim(mari)
	}

	// 2. Email party — queued.
	email := create("order_5512", nil, []domain.MemberInput{
		{UserID: "support@acme.com", Kind: models.MemberEmail, ConvRole: models.RoleClient, Metadata: json.RawMessage(`{"name":"support@acme.com","email":"support@acme.com","role":"email contact"}`)},
	})
	if email != nil {
		user(email.ID, "support@acme.com", "Following up — the refund for order 5512 still hasn't landed. It's been 6 business days.", models.ChannelEmail)
	}

	// 3. Jaan Kask — claimed (assigned).
	jaan := create("trip_7731", nil, []domain.MemberInput{
		{UserID: "u_jaan", ConvRole: models.RoleClient, Metadata: json.RawMessage(`{"name":"Jaan Kask","phone":"+372 5444 1212","app_version":"2.2.9","role":"client"}`)},
	})
	if jaan != nil {
		user(jaan.ID, "u_jaan", "I can't add a payment card — it keeps failing.", models.ChannelApp)
		agent(jaan.ID, "Try removing the old card first, then re-adding. There was a stale token on your account.")
		user(jaan.ID, "u_jaan", "Thanks, that worked", models.ChannelApp)
		claim(jaan)
	}

	// 4. Guest · web — queued.
	guest := create("guest_7f3a", nil, []domain.MemberInput{
		{UserID: "guest_7f3a", ConvRole: models.RoleClient, Metadata: json.RawMessage(`{"name":"Guest · web","guest":"true","app_version":"web 1.0"}`)},
	})
	if guest != nil {
		user(guest.ID, "guest_7f3a", "How do I change my pickup address after booking?", models.ChannelApp)
	}

	// 5. Pille Saar — closed.
	pille := create("trip_2014", nil, []domain.MemberInput{
		{UserID: "u_pille", ConvRole: models.RoleClient, Metadata: json.RawMessage(`{"name":"Pille Saar","phone":"+372 5333 9090","role":"client"}`)},
	})
	if pille != nil {
		user(pille.ID, "u_pille", "Driver was great, thank you", models.ChannelApp)
		_ = svc.CloseConversation(ctx, tenantID, pille.ID)
	}
}
