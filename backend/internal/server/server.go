// Package server builds the gin engine, router, and middleware for sild-api.
// Handlers themselves live under internal/api, grouped by audience.
package server

import (
	"context"
	"net/http"
	"time"

	"github.com/bitllow/sild/backend/internal/api"
	"github.com/bitllow/sild/backend/internal/config"
	"github.com/bitllow/sild/backend/internal/store"
	"github.com/gin-gonic/gin"
)

// Server wraps the gin engine and its dependencies.
type Server struct {
	cfg    *config.Config
	store  store.Store
	engine *gin.Engine
}

// New constructs the server, registers base routes, and mounts the REST API.
func New(cfg *config.Config, st store.Store, h *api.Handler) *Server {
	if cfg.Env == "production" {
		gin.SetMode(gin.ReleaseMode)
	}
	e := gin.New()
	e.Use(gin.Recovery())

	s := &Server{cfg: cfg, store: st, engine: e}
	s.registerHealth()
	h.Mount(e)
	return s
}

// Engine exposes the gin engine so api packages can attach route groups.
func (s *Server) Engine() *gin.Engine { return s.engine }

func (s *Server) registerHealth() {
	s.engine.GET("/healthz", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})
	s.engine.GET("/readyz", func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
		defer cancel()
		if err := s.store.Health(ctx); err != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"status": "db_unavailable"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"status": "ready"})
	})
}

// Run starts the HTTP server and blocks until the context is cancelled, then
// shuts down gracefully.
func (s *Server) Run(ctx context.Context) error {
	srv := &http.Server{Addr: s.cfg.HTTPAddr, Handler: s.engine}
	errCh := make(chan error, 1)
	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
	}()
	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return srv.Shutdown(shutdownCtx)
	}
}
