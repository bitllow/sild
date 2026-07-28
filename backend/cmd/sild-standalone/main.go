// Command sild-standalone runs every serving role — REST, realtime, and the
// background jobs — in ONE process on ONE listener. It is a packaging choice, not
// a different architecture: the same image, the same code, and the same broker as
// the split deployment (§3a), so N standalone replicas behind a load balancer
// scale exactly like N api + M ws pods.
//
// It is a production binary. It never migrates (sild-migrate is the only schema
// path, §4), serves none of sild-dev's conveniences, and refuses to start on a
// configuration that is wrong above one replica.
//
//	sild-migrate && sild-standalone
package main

import (
	"context"
	"log"
	"os/signal"
	"syscall"

	"github.com/bitllow/sild/backend/internal/archive"
	"github.com/bitllow/sild/backend/internal/auth"
	"github.com/bitllow/sild/backend/internal/config"
	"github.com/bitllow/sild/backend/internal/connector/webhook"
	"github.com/bitllow/sild/backend/internal/di"
	"github.com/bitllow/sild/backend/internal/domain"
	"github.com/bitllow/sild/backend/internal/jobs"
	"github.com/bitllow/sild/backend/internal/mail"
	"github.com/bitllow/sild/backend/internal/provision"
	"github.com/bitllow/sild/backend/internal/realtime"
	"github.com/bitllow/sild/backend/internal/server"
	"github.com/bitllow/sild/backend/internal/store"
	"github.com/gin-gonic/gin"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	c, err := di.New()
	if err != nil {
		log.Fatalf("di: %v", err)
	}

	// Config alone, so a rejected one touches nothing: dig builds a dependency
	// before the callback asking for it, and *domain.Service dials the broker.
	var selected jobs.Set
	err = c.Invoke(func(cfg *config.Config) error {
		if err := cfg.RequireProduction(); err != nil {
			return err
		}
		selected, err = jobs.Parse(cfg.Jobs.Enabled)
		return err
	})
	if err != nil {
		log.Fatalf("sild-standalone: %v", err)
	}

	err = c.Invoke(func(
		cfg *config.Config, km *auth.KeyManager, svc *domain.Service, st store.Store,
		srv *server.Server, node *realtime.Node, relay *webhook.Relay, sweep *archive.Job,
	) error {
		if err := km.EnsureActiveKey(ctx); err != nil {
			return err
		}
		if err := node.Run(); err != nil { // idempotent; shared with the publisher
			return err
		}

		// The whole product on one listener — what makes a single container enough.
		srv.Engine().GET("/v1/ws", gin.WrapH(node.WSHandler()))
		srv.Engine().GET("/v1/ws/sse", gin.WrapH(node.SSEHandler()))

		if err := bootstrap(ctx, cfg, svc, st); err != nil {
			return err
		}

		jobs.Start(ctx, selected, jobs.Deps{Relay: relay, Sweep: sweep})

		if cfg.Jobs.SMTPIngest {
			go func() {
				if err := mail.Serve(ctx, cfg.Email.SMTPListenAddr, svc.ForwardedMailHandler()); err != nil {
					log.Printf("sild-standalone: smtp ingest: %v", err)
				}
			}()
		}

		log.Printf("sild-standalone: REST+WS on %s — db=%s broker=%s storage=%s jobs=%v smtp=%s",
			cfg.ListenAddr(), cfg.DB.Driver, cfg.Realtime.Broker, cfg.Storage.Backend,
			selected.Names(), smtpState(cfg))
		return srv.Run(ctx)
	})
	if err != nil {
		log.Fatalf("sild-standalone: %v", err)
	}
}

// bootstrap creates the first tenant from env on an empty database, so a platform
// with no shell still has a way in. Every later restart is a no-op.
func bootstrap(ctx context.Context, cfg *config.Config, svc *domain.Service, st store.Store) error {
	res, done, err := provision.Bootstrap(ctx, svc, st, provision.TenantSpec{
		Name:          cfg.Bootstrap.TenantName,
		AdminEmail:    cfg.Bootstrap.AdminEmail,
		AdminName:     cfg.Bootstrap.AdminName,
		AdminPassword: cfg.Bootstrap.AdminPassword,
		APIKeyLabel:   "bootstrap",
	})
	if err != nil {
		return err
	}
	if !done {
		return nil
	}
	// One entry, not six: Cloud Run and k8s render each log call separately.
	log.Printf("bootstrap created the first tenant\n"+
		"  tenant_id           %s\n"+
		"  owner               %s\n"+
		"  api_key             %s   (shown once)\n"+
		"  forwarding_address  %s\n"+
		"  clear SILD_BOOTSTRAP_ADMIN_PASSWORD once you have signed in",
		res.TenantID, cfg.Bootstrap.AdminEmail, res.APIKey, res.ForwardingAddress)
	return nil
}

func smtpState(cfg *config.Config) string {
	if !cfg.Jobs.SMTPIngest {
		return "off (inbound mail via POST /v1/email/inbound)"
	}
	return cfg.Email.SMTPListenAddr
}
