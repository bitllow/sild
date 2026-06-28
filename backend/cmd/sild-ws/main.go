// Command sild-ws serves the egress-only realtime layer (§5): Centrifuge WS+SSE.
// It holds client connections and fans out events published by sild-api via the
// broker (§3a).
package main

import (
	"context"
	"log"
	"net/http"
	"os/signal"
	"syscall"
	"time"

	"github.com/bitllow/sild/backend/internal/config"
	"github.com/bitllow/sild/backend/internal/di"
	"github.com/bitllow/sild/backend/internal/realtime"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	c, err := di.New()
	if err != nil {
		log.Fatalf("di: %v", err)
	}
	err = c.Invoke(func(cfg *config.Config, node *realtime.Node) error {
		if err := node.Run(); err != nil {
			return err
		}
		srv := &http.Server{Addr: cfg.Realtime.WSAddr, Handler: node.Handler()}
		go func() {
			if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				log.Printf("sild-ws: %v", err)
				stop()
			}
		}()
		log.Printf("sild-ws: listening on %s (broker=%s)", cfg.Realtime.WSAddr, cfg.Realtime.Broker)

		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = node.Shutdown(shutdownCtx)
		return srv.Shutdown(shutdownCtx)
	})
	if err != nil {
		log.Fatalf("sild-ws: %v", err)
	}
}
