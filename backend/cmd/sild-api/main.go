// Command sild-api serves the REST API (§4). Stateless; publishes realtime
// events to the broker after each Postgres write.
package main

import (
	"context"
	"log"
	"os/signal"
	"syscall"

	"github.com/bitllow/sild/backend/internal/auth"
	"github.com/bitllow/sild/backend/internal/config"
	"github.com/bitllow/sild/backend/internal/di"
	"github.com/bitllow/sild/backend/internal/server"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	c, err := di.New() // server.New is already registered here
	if err != nil {
		log.Fatalf("di: %v", err)
	}

	err = c.Invoke(func(cfg *config.Config, km *auth.KeyManager, srv *server.Server) error {
		if err := km.EnsureActiveKey(ctx); err != nil { // bootstrap JWT signing key
			return err
		}
		log.Printf("sild-api: listening on %s (driver=%s)", cfg.HTTPAddr, cfg.DB.Driver)
		return srv.Run(ctx)
	})
	if err != nil {
		log.Fatalf("sild-api: %v", err)
	}
}
