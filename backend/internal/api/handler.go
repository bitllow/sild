// Package api wires the REST surface (§4). Routes are grouped by audience
// (integration/user/admin/public) at the file level; shared paths that accept
// multiple credential types use the Any() middleware and authorize by principal.
package api

import (
	"fmt"
	"net/http"
	"slices"

	"github.com/bitllow/sild/backend/internal/apiutil"
	"github.com/bitllow/sild/backend/internal/auth"
	"github.com/bitllow/sild/backend/internal/config"
	"github.com/bitllow/sild/backend/internal/domain"
	"github.com/bitllow/sild/backend/internal/middleware"
	"github.com/bitllow/sild/backend/internal/principal"
	"github.com/bitllow/sild/backend/internal/storage"
	"github.com/bitllow/sild/backend/internal/store/models"
	"github.com/bitllow/sild/backend/internal/webasset"
	"github.com/gin-gonic/gin"
)

// Handler holds the dependencies for all REST handlers.
type Handler struct {
	svc    *domain.Service
	search *domain.SearchService
	mw     *middleware.Auth
	km     *auth.KeyManager
	authn  auth.AdminAuthenticator
	bucket storage.Bucket
	cfg    *config.Config
}

// New constructs the api Handler. dig provides it.
func New(svc *domain.Service, search *domain.SearchService, mw *middleware.Auth, km *auth.KeyManager, authn auth.AdminAuthenticator, bucket storage.Bucket, cfg *config.Config) *Handler {
	return &Handler{svc: svc, search: search, mw: mw, km: km, authn: authn, bucket: bucket, cfg: cfg}
}

// Mount builds the router from the route manifest. Each route's middleware chain
// is DERIVED from its descriptor, so a route cannot be registered without a
// declared classification and principal set, and cannot be moved into a weaker
// guard — there is no second place to move it to.
//
// Every route carries its own body cap and is registered on the bare engine
// rather than under a capped group: a nested MaxBytesReader can only lower a
// limit, so a group cap would silently bound the raw upload and email paths.
func (h *Handler) Mount(e *gin.Engine) {
	// One limiter per rate class, shared by every route declaring it. A bucket per
	// route would give credential acquisition two budgets to spend.
	limiters := map[rateClass]gin.HandlerFunc{
		rateAuth:    h.mw.RateLimitAuth(),
		rateIngress: h.mw.RateLimitIngress(),
	}

	for _, r := range routeManifest() {
		if r.Enabled != nil && !r.Enabled(h) {
			continue
		}
		chain := make([]gin.HandlerFunc, 0, 5)
		chain = append(chain, middleware.BodyLimit(r.bodyLimit()))
		// Publish the declared set so a handler picking its action at runtime —
		// create_support vs create_peer, claim vs close — cannot pick one this route
		// does not declare.
		chain = append(chain, apiutil.DeclareActions(r.Actions))
		if lim, ok := limiters[r.Rate]; ok {
			chain = append(chain, lim)
		}
		chain = append(chain, h.authChain(r)...)
		handler := r.Handler
		chain = append(chain, func(c *gin.Context) { handler(h, c) })
		e.Handle(r.Method, r.Path, chain...)
	}
}

// authChain is the credential guard a descriptor implies. Derived, never
// declared separately: the manifest says who may call the route, and this is the
// only translation of that into middleware.
func (h *Handler) authChain(r routeSpec) []gin.HandlerFunc {
	switch r.Class {
	case classInfrastructure, classSignedIngress:
		// No principal: infrastructure serves no tenant data, and signed ingress
		// is gated by a signature the handler verifies.
		return nil

	case classPublic:
		// Credential acquisition takes none. A public route that still ACTS on a
		// principal when one is present (the app-id-keyed brand read) resolves it
		// optionally — and 401s an invalid one rather than downgrading.
		if len(r.Actions) == 0 {
			return nil
		}
		return []gin.HandlerFunc{h.mw.OptionalAuth()}
	}

	if slices.Equal(r.Principals, signedOnly) {
		return nil // the signature is the credential; the handler verifies it
	}
	guard := []gin.HandlerFunc{h.credentialGuard(r.Principals)}
	if r.privilegedOnly() {
		guard = append(guard, middleware.RequireRole(models.PlatformOwner, models.PlatformAdmin))
	}
	return guard
}

// credentialGuard maps the declared principal kinds onto the middleware that
// admits exactly those.
func (h *Handler) credentialGuard(kinds []principal.Kind) gin.HandlerFunc {
	switch {
	case slices.Equal(kinds, anyPrincipal):
		return h.mw.Any()
	case slices.Equal(kinds, keyOnly):
		return h.mw.APIKey()
	case slices.Equal(kinds, userOnly):
		return h.mw.UserJWT()
	case slices.Equal(kinds, adminOnly):
		return h.mw.Admin()
	}
	// An undeclarable combination must not fall through to "no guard".
	panic(fmt.Sprintf("no credential guard for principal set %v", kinds))
}

func (r routeSpec) bodyLimit() int64 {
	if r.BodyLimit != 0 {
		return r.BodyLimit
	}
	return middleware.BodyLimitJSON
}

func (r routeSpec) successStatus() int {
	if r.Success != 0 {
		return r.Success
	}
	return http.StatusOK
}

// devLoginAvailable gates the stub Google login on a non-production deployment.
func (h *Handler) devLoginAvailable() bool {
	return h.authn.IsStub() && h.cfg.Env != "production"
}

// localStorageServed reports whether attachment bytes are served by this process.
// GCS/S3 issue direct-to-bucket signed URLs and don't mount the local routes.
func (h *Handler) localStorageServed() bool {
	return h.cfg.Storage.Backend == "local" || h.cfg.Storage.Backend == ""
}

// widgetBundle serves the web drop-in (§9), embedded at build time so the binary
// is self-contained. CORS is engine-wide, so customer sites can load it
// cross-origin; front it with a CDN in production.
func (h *Handler) widgetBundle(c *gin.Context) {
	c.Header("Cache-Control", "public, max-age=300")
	c.Data(http.StatusOK, "application/javascript; charset=utf-8", webasset.Widget)
}
