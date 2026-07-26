package api

import (
	"testing"

	"github.com/bitllow/sild/backend/internal/auth"
	"github.com/bitllow/sild/backend/internal/config"
	"github.com/bitllow/sild/backend/internal/middleware"
	"github.com/gin-gonic/gin"
)

// newManifestProbeHandler builds a Handler wired only enough to register routes
// and answer descriptor questions. Mount and the Enabled predicates dereference
// nothing else, so the zero wiring is sufficient.
func newManifestProbeHandler(t *testing.T) *Handler {
	t.Helper()
	cfg := &config.Config{Env: "development"}
	cfg.Storage.Backend = "local"
	return New(nil, nil, middleware.NewAuth(nil, nil), nil, auth.NewAdminAuthenticator(cfg), nil, cfg)
}

// mountedRouteKeys mounts the real route table and returns "METHOD PATH" for
// each route.
func mountedRouteKeys(t *testing.T) []string {
	t.Helper()
	gin.SetMode(gin.TestMode)
	e := gin.New()
	newManifestProbeHandler(t).Mount(e)

	out := make([]string, 0, 64)
	for _, r := range e.Routes() {
		out = append(out, r.Method+" "+r.Path)
	}
	return out
}
