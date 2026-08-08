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
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/bitllow/sild/backend/internal/archive"
	"github.com/bitllow/sild/backend/internal/auth"
	"github.com/bitllow/sild/backend/internal/config"
	"github.com/bitllow/sild/backend/internal/connector/webhook"
	"github.com/bitllow/sild/backend/internal/di"
	"github.com/bitllow/sild/backend/internal/domain"
	"github.com/bitllow/sild/backend/internal/jobs"
	"github.com/bitllow/sild/backend/internal/mail"
	"github.com/bitllow/sild/backend/internal/provision"
	"github.com/bitllow/sild/backend/internal/push"
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
		sweep *archive.Job, nudges *push.FanOut,
	) error {
		// Dev-only exception to "only sild-migrate migrates" (ARCHITECTURE §4).
		// SILD_DEV_MIGRATE=false wherever this shares a database with a deployment.
		if migrate, err := strconv.ParseBool(os.Getenv("SILD_DEV_MIGRATE")); err != nil || migrate {
			if err := gormstore.Migrate(db); err != nil {
				return err
			}
		}
		if err := km.EnsureActiveKey(ctx); err != nil {
			return err
		}
		if err := node.Run(); err != nil { // idempotent; shared with the publisher
			return err
		}

		// Mount the WS/SSE transport on the same server (single port).
		node.Mount(srv.Engine())

		// Phase 3 drop-in: /widget.js is mounted from the embedded bundle in
		// handler.Mount (so sild-api serves it too). Here we add the dev-only
		// faux host page, also embedded.
		srv.Engine().GET("/sild-demo", func(c *gin.Context) {
			c.Data(200, "text/html; charset=utf-8", webasset.Demo)
		})

		// The demo page needs the app id the same way a real host page does — it is
		// required on the anonymous brand read. A production host hardcodes the
		// value from Settings → Installation; the demo asks the dev server.
		srv.Engine().GET("/v1/dev/app-id", func(c *gin.Context) {
			ids, err := st.Tenants().AllIDs(c.Request.Context())
			if err != nil || len(ids) == 0 {
				c.JSON(500, gin.H{"error": "no tenant"})
				return
			}
			c.JSON(200, gin.H{"app_id": ids[0]})
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

		// Dev-only: ensure a peer (driver↔rider) conversation exists for a rider and
		// return its id. Stands in for a host backend creating a peer chat via the
		// API key (POST /v1/conversations, open_assignment:false). Idempotent per
		// (reference, rider) so the demo's "Open chat" card reuses one conversation.
		srv.Engine().GET("/v1/dev/peer-conversation", func(c *gin.Context) {
			rider := c.Query("user_id")
			if rider == "" {
				rider = "u_demo"
			}
			reference := c.Query("reference")
			if reference == "" {
				reference = "trip_9021"
			}
			// Optional rider metadata (URL-encoded JSON) from the demo session, so
			// the rider participant carries the same name/phone/plan the inbox shows
			// and can be found by those values in the peer search.
			riderMeta := json.RawMessage(nil)
			if m := c.Query("meta"); m != "" && json.Valid([]byte(m)) {
				riderMeta = json.RawMessage(m)
			}
			ids, err := st.Tenants().AllIDs(c.Request.Context())
			if err != nil || len(ids) == 0 {
				c.JSON(500, gin.H{"error": "no tenant"})
				return
			}
			id, err := ensurePeerConversation(c.Request.Context(), svc, ids[0], rider, reference, riderMeta)
			if err != nil {
				c.JSON(500, gin.H{"error": err.Error()})
				return
			}
			c.JSON(200, gin.H{"conversation_id": id})
		})

		devSeed(ctx, st, svc, cfg)

		// Email forwarding ingestion daemon, in-process so `make dev` exercises
		// the full loop (forwarded mail → conversation) without a separate binary.
		go func() {
			if err := mail.Serve(ctx, cfg.Email.SMTPListenAddr, svc.ForwardedMailHandler()); err != nil {
				log.Printf("sild-dev: smtp receiver: %v", err)
			}
		}()

		// The same jobs, on the same schedule, as the deployed binaries — minus the
		// archive sweep by default, since it purges hot rows.
		selected, err := jobs.Parse(cfg.Jobs.List(jobs.Webhook + "," + jobs.Push))
		if err != nil {
			return err
		}
		jobs.Start(ctx, selected, jobs.Deps{Relay: relay, Sweep: sweep, Push: nudges})

		log.Printf("sild-dev: REST+WS on %s (db=%s, broker=%s, jobs=%v) — Ctrl-C to stop", cfg.ListenAddr(), cfg.DB.Driver, cfg.Realtime.Broker, selected.Names())
		return srv.Run(ctx)
	})
	if err != nil {
		log.Fatalf("sild-dev: %v", err)
	}
}

// devSeed creates a ready-to-use tenant + owner admin + API key on first run so
// you can log into the inbox (email/password) and call the API immediately.
func devSeed(ctx context.Context, st store.Store, svc *domain.Service, cfg *config.Config) {
	res, err := provision.Bootstrap(ctx, svc, st, provision.TenantSpec{
		Name: "Dev Tenant", AdminEmail: "admin@sild.local", AdminName: "Eva Marleen",
		AdminPassword: "password123", APIKeyLabel: "dev",
	})
	if err != nil {
		log.Printf("dev seed: %v", err)
		return
	}
	if res == nil { // already seeded
		if ids, err := st.Tenants().AllIDs(ctx); err == nil && len(ids) > 0 {
			if ch, err := svc.GetEmailChannel(ctx, ids[0]); err == nil {
				log.Printf("sild-dev: forward email to %s (SMTP %s) to open a conversation", ch.ForwardingAddress, cfg.Email.SMTPListenAddr)
			}
		}
		return
	}
	devSeedConversations(ctx, svc, res.TenantID, res.AdminID)
	devSeedPeerConversations(ctx, svc, res.TenantID, res.AdminID)
	log.Printf("┌─ dev seed ────────────────────────────────────────────")
	log.Printf("│ tenant_id : %s", res.TenantID)
	log.Printf("│ admin     : admin@sild.local / password123  (POST /v1/admin/auth/password)")
	log.Printf("│ api key   : %s", res.APIKey)
	log.Printf("│ inbox     : sample support requests + contact history seeded")
	log.Printf("│ email in  : forward to %s (SMTP %s)", res.ForwardingAddress, cfg.Email.SMTPListenAddr)
	log.Printf("└───────────────────────────────────────────────────────")
}

func strptr(s string) *string { return &s }

// seedProfile stores a person's contact profile — the seed standing in for the
// host backend, which writes it once rather than per conversation.
func seedProfile(ctx context.Context, svc *domain.Service, tenantID, uid, meta string) {
	if err := svc.UpsertContact(ctx, tenantID, uid, json.RawMessage(meta)); err != nil {
		log.Printf("dev seed contact %s: %v", uid, err)
	}
}

// ensurePeerConversation finds (by reference + rider) or creates a driver↔rider
// peer conversation and returns its id, seeding a driver message so the widget
// thread opens with content. Peer conversations carry no assignment.
func ensurePeerConversation(ctx context.Context, svc *domain.Service, tenantID, riderID, reference string, riderMeta json.RawMessage) (string, error) {
	if existing, err := svc.ListPeerConversations(ctx, tenantID, store.PeerParams{Limit: 100}); err == nil {
		for _, cv := range existing.Conversations {
			if cv["reference"] == reference && peerHasMember(cv, riderID) {
				return cv["id"].(string), nil
			}
		}
	}
	driverID := "u_driver_" + reference
	if len(riderMeta) == 0 {
		riderMeta, _ = json.Marshal(map[string]string{"name": "Rider", "role": "rider"})
	}
	driverMeta, _ := json.Marshal(map[string]string{
		"name": "Toomas Vaher", "role": "driver", "vehicle": "Silver estate · 421 KLM", "phone": "+372 5987 6543",
	})
	seedProfile(ctx, svc, tenantID, riderID, string(riderMeta))
	seedProfile(ctx, svc, tenantID, driverID, string(driverMeta))
	conv, err := svc.CreateConversation(ctx, tenantID, domain.CreateConversationInput{
		Reference: reference, OpenAssignment: false,
		Members: []domain.MemberInput{
			{UserID: riderID, ConvRole: models.ConvRole("rider")},
			{UserID: driverID, ConvRole: models.ConvRole("driver")},
		},
	})
	if err != nil {
		return "", err
	}
	_, _ = svc.SendMessage(ctx, tenantID, conv.ID, domain.SendInput{
		SenderKind: models.SenderUser, External: &driverID, Channel: models.ChannelApp,
		Body: "I'm at the main entrance now. Silver estate, plate 421 KLM.",
	})
	return conv.ID, nil
}

func peerHasMember(cv map[string]any, userID string) bool {
	members, ok := cv["members"].([]map[string]any)
	if !ok {
		return false
	}
	for _, m := range members {
		if m["external_user_id"] == userID {
			return true
		}
	}
	return false
}

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

	seedProfile(ctx, svc, tenantID, "u_mari", `{"name":"Mari Tamm","phone":"+372 5123 4567","app_version":"2.3.1","role":"client"}`)
	seedProfile(ctx, svc, tenantID, "u_driver9", `{"name":"Driver 9","phone":"+372 5987 6543","app_version":"2.3.0","role":"driver"}`)
	seedProfile(ctx, svc, tenantID, "support@acme.com", `{"name":"support@acme.com","email":"support@acme.com","role":"email contact"}`)
	seedProfile(ctx, svc, tenantID, "u_jaan", `{"name":"Jaan Kask","phone":"+372 5444 1212","app_version":"2.2.9","role":"client"}`)
	seedProfile(ctx, svc, tenantID, "guest_7f3a", `{"name":"Guest · web","guest":"true","app_version":"web 1.0"}`)
	seedProfile(ctx, svc, tenantID, "u_pille", `{"name":"Pille Saar","phone":"+372 5333 9090","role":"client"}`)

	// Mari's earlier, resolved threads — seed real contact history so the Details
	// panel's "Earlier from Mari" list (and the "View all from" filter) has data.
	past1 := create("trip_8109", nil, []domain.MemberInput{
		{UserID: "u_mari", ConvRole: models.RoleClient},
	})
	if past1 != nil {
		user(past1.ID, "u_mari", "I was charged twice for last night's ride.", models.ChannelApp)
		agent(past1.ID, "You're right — I've refunded the duplicate charge. It'll land in 3–5 days.")
		user(past1.ID, "u_mari", "Perfect, thanks!", models.ChannelApp)
		claim(past1)
		_ = svc.CloseConversation(ctx, tenantID, past1.ID)
	}
	past2 := create("trip_7640", nil, []domain.MemberInput{
		{UserID: "u_mari", ConvRole: models.RoleClient},
	})
	if past2 != nil {
		user(past2.ID, "u_mari", "Can I get a receipt for my trip to the airport?", models.ChannelApp)
		agent(past2.ID, "Sent it to your email just now.")
		claim(past2)
		_ = svc.CloseConversation(ctx, tenantID, past2.ID)
	}

	// 1. Mari Tamm — claimed (assigned), client + driver, internal note.
	mari := create("trip_8842", json.RawMessage(`{"kind":"ride"}`), []domain.MemberInput{
		{UserID: "u_mari", ConvRole: models.RoleClient},
		{UserID: "u_driver9", ConvRole: models.RoleDriver},
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
		{UserID: "support@acme.com", Kind: models.MemberEmail, ConvRole: models.RoleClient},
	})
	if email != nil {
		user(email.ID, "support@acme.com", "Following up — the refund for order 5512 still hasn't landed. It's been 6 business days.", models.ChannelEmail)
	}

	// 3. Jaan Kask — claimed (assigned).
	jaan := create("trip_7731", nil, []domain.MemberInput{
		{UserID: "u_jaan", ConvRole: models.RoleClient},
	})
	if jaan != nil {
		user(jaan.ID, "u_jaan", "I can't add a payment card — it keeps failing.", models.ChannelApp)
		agent(jaan.ID, "Try removing the old card first, then re-adding. There was a stale token on your account.")
		user(jaan.ID, "u_jaan", "Thanks, that worked", models.ChannelApp)
		claim(jaan)
	}

	// 4. Guest · web — queued.
	guest := create("guest_7f3a", nil, []domain.MemberInput{
		{UserID: "guest_7f3a", ConvRole: models.RoleClient},
	})
	if guest != nil {
		user(guest.ID, "guest_7f3a", "How do I change my pickup address after booking?", models.ChannelApp)
	}

	// 5. Pille Saar — closed.
	pille := create("trip_2014", nil, []domain.MemberInput{
		{UserID: "u_pille", ConvRole: models.RoleClient},
	})
	if pille != nil {
		user(pille.ID, "u_pille", "Driver was great, thank you", models.ChannelApp)
		_ = svc.CloseConversation(ctx, tenantID, pille.ID)
	}
}

// devSeedPeerConversations populates the peer-conversations surface with the
// sample direct chats from the design: rider↔driver, rider↔rider, and one the
// agent has already stepped into. These carry NO assignment, so they never enter
// the support queue — only the peer inbox. Roles are free-form tenant strings
// ("rider"/"driver"); the UI derives all labels/colors from them.
func devSeedPeerConversations(ctx context.Context, svc *domain.Service, tenantID, adminID string) {
	msg := func(convID, uid, body string) {
		u := uid
		_, _ = svc.SendMessage(ctx, tenantID, convID, domain.SendInput{
			SenderKind: models.SenderUser, External: &u, Body: body, Channel: models.ChannelApp,
		})
	}
	peer := func(ref string, members []domain.MemberInput) *models.Conversation {
		conv, err := svc.CreateConversation(ctx, tenantID, domain.CreateConversationInput{
			Reference: ref, Members: members, OpenAssignment: false,
		})
		if err != nil {
			log.Printf("dev seed peer %s: %v", ref, err)
			return nil
		}
		return conv
	}
	role := func(r string) models.ConvRole { return models.ConvRole(r) }

	seedProfile(ctx, svc, tenantID, "p_mari", `{"name":"Mari Tamm","phone":"+372 5123 4567","app_version":"2.3.1","role":"rider"}`)
	seedProfile(ctx, svc, tenantID, "p_toomas", `{"name":"Toomas Vaher","phone":"+372 5987 6543","vehicle":"Silver estate · 421 KLM","app_version":"2.3.0","role":"driver"}`)
	seedProfile(ctx, svc, tenantID, "p_jaan", `{"name":"Jaan Kask","phone":"+372 5444 1212","app_version":"2.2.9","role":"rider"}`)
	seedProfile(ctx, svc, tenantID, "p_pille", `{"name":"Pille Saar","phone":"+372 5333 9090","app_version":"2.3.0","role":"rider"}`)
	seedProfile(ctx, svc, tenantID, "p_andres", `{"name":"Andres Laan","phone":"+372 5661 2020","vehicle":"Black hatchback · 118 TRE","role":"driver"}`)

	// Peer participants use their own ids (p_*), distinct from the support-seed
	// contacts (u_mari/u_pille/…), so these direct chats stay a separate persona
	// space and don't bleed into a support contact's history panel.

	// 1. rider↔driver — trip in progress (default active).
	if c := peer("trip_9021", []domain.MemberInput{
		{UserID: "p_mari", ConvRole: role("rider")},
		{UserID: "p_toomas", ConvRole: role("driver")},
	}); c != nil {
		msg(c.ID, "p_mari", "Hi, I'm the passenger for trip 9021 — waiting outside the north doors.")
		msg(c.ID, "p_toomas", "Hi Mari, 1 minute away. There's construction at the north side — I'll pull up to the main entrance instead.")
		msg(c.ID, "p_mari", "Got it, walking over now.")
		msg(c.ID, "p_toomas", "I'm at the main entrance now. Silver estate, plate 421 KLM.")
	}

	// 2. rider↔rider — shared ride (unread).
	if c := peer("trip_8830 · shared ride", []domain.MemberInput{
		{UserID: "p_mari", ConvRole: role("rider")},
		{UserID: "p_jaan", ConvRole: role("rider")},
	}); c != nil {
		msg(c.ID, "p_mari", "Hey — we're matched for the shared ride to the airport. Terminal 1 or 2?")
		msg(c.ID, "p_jaan", "Terminal 2. I can meet you at the taxi rank if that's easier.")
	}

	// 3. rider↔driver — lost item; the agent has already stepped in ("You joined").
	if c := peer("trip_7788 · lost item", []domain.MemberInput{
		{UserID: "p_pille", ConvRole: role("rider")},
		{UserID: "p_andres", ConvRole: role("driver")},
	}); c != nil {
		msg(c.ID, "p_pille", "I think I left my scarf in the back seat.")
		msg(c.ID, "p_andres", "Found a scarf — I can drop it at your address after my next trip.")
		// The agent steps in (implicit join): adds the agent participant + join-note.
		_, _ = svc.PeerAgentSend(ctx, tenantID, c.ID, adminID,
			"Andres, please mark it as a lost-item return so the detour is covered.", nil, "")
	}
}
