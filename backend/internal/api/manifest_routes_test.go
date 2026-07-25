package api

import (
	"testing"

	"github.com/bitllow/sild/backend/internal/auth"
	"github.com/bitllow/sild/backend/internal/config"
	"github.com/bitllow/sild/backend/internal/middleware"
	"github.com/gin-gonic/gin"
)

// mountedRouteKeys mounts the real route table and returns "METHOD PATH" for
// each route. Mount only touches the engine, so no dependencies are needed.
func mountedRouteKeys(t *testing.T) []string {
	t.Helper()
	gin.SetMode(gin.TestMode)
	e := gin.New()
	// Mount only registers handlers; nothing it calls dereferences a dependency,
	// so the zero wiring is enough to enumerate the table.
	cfg := &config.Config{Env: "development"}
	cfg.Storage.Backend = "local"
	h := New(nil, nil, middleware.NewAuth(nil, nil), nil, auth.NewAdminAuthenticator(cfg), nil, cfg)
	h.Mount(e)

	out := make([]string, 0, 64)
	for _, r := range e.Routes() {
		out = append(out, r.Method+" "+r.Path)
	}
	return out
}
