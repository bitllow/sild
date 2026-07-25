package api

import (
	"net/http"

	"github.com/bitllow/sild/backend/internal/apiutil"
	"github.com/bitllow/sild/backend/internal/domain"
	"github.com/bitllow/sild/backend/internal/httpx"
	"github.com/gin-gonic/gin"
)

// ── Appearance: web/SDK messenger branding (§8 Settings → Appearance) ────────
//
// A brand is one complete, named messenger look. A tenant keeps several and
// marks one active; the active brand's config drives both the web drop-in widget
// and the native SDK messengers. The admin edits the whole set client-side
// (staged Save/Reset) and PUTs it back as one payload.

type brandBody struct {
	ID     string             `json:"id"`
	Name   string             `json:"name"`
	Config domain.BrandConfig `json:"config"`
}

func brandView(b domain.Brand) gin.H {
	return gin.H{"id": b.ID, "name": b.Name, "config": b.Config}
}

func brandsView(brands []domain.Brand) gin.H {
	out := make([]gin.H, 0, len(brands))
	activeID := ""
	for _, b := range brands {
		out = append(out, brandView(b))
		if b.Active {
			activeID = b.ID
		}
	}
	if activeID == "" && len(brands) > 0 {
		activeID = brands[0].ID
	}
	return gin.H{"brands": out, "active_brand_id": activeID}
}

// listBrands returns the tenant's brands + which one is active (owner/admin, §7).
// Seeds a default brand on first access.
func (h *Handler) listBrands(c *gin.Context) {
	brands, version, err := h.svc.ListBrandsVersioned(c.Request.Context(), apiutil.Tenant(c))
	if err != nil {
		apiutil.Fail(c, err)
		return
	}
	c.Header("ETag", version)
	c.JSON(http.StatusOK, brandsView(brands))
}

// saveBrands replaces the tenant's whole brand set with the staged edits
// (owner/admin, §7). The request is the full list plus the active id.
func (h *Handler) saveBrands(c *gin.Context) {
	var req struct {
		Brands        []brandBody `json:"brands"`
		ActiveBrandID string      `json:"active_brand_id"`
	}
	if !httpx.DecodeJSON(c, &req) {
		return
	}
	// PUT replaces the whole set, so a concurrent edit would be discarded silently
	// without a precondition. The comparison happens inside SaveBrands' write
	// transaction; this only insists the client named a version.
	expect, ok := apiutil.RequireIfMatch(c)
	if !ok {
		return
	}
	brands := make([]domain.Brand, 0, len(req.Brands))
	for _, b := range req.Brands {
		brands = append(brands, domain.Brand{ID: b.ID, Name: b.Name, Config: b.Config})
	}
	saved, err := h.svc.SaveBrands(c.Request.Context(), apiutil.Tenant(c), brands, req.ActiveBrandID, expect)
	if err != nil {
		apiutil.Fail(c, err)
		return
	}
	c.Header("ETag", h.svc.BrandsVersionOf(saved))
	c.JSON(http.StatusOK, brandsView(saved))
}

// getActiveBrand: GET /v1/brands/active — the active brand for any messenger
// surface.
//
// Public when keyed by app_id (the drop-in's first paint, before a token exists),
// tenant-scoped with a credential. An invalid credential never reaches here:
// OptionalAuth 401s rather than downgrading, so an expired session cannot become
// a silent tenant switch.
func (h *Handler) getActiveBrand(c *gin.Context) {
	// Credential first: an authenticated caller's app_id is ignored, so the public
	// path cannot read another tenant's brand.
	if tenant := apiutil.Tenant(c); tenant != "" {
		b, err := h.svc.ActiveBrand(c.Request.Context(), tenant)
		if err != nil {
			apiutil.Fail(c, err)
			return
		}
		// Private: an authenticated response must never populate a shared cache
		// that an anonymous request could then be served from.
		c.Header("Cache-Control", "private, no-store")
		c.Header("Vary", "Authorization, Cookie")
		c.JSON(http.StatusOK, gin.H{"name": b.Name, "config": b.Config})
		return
	}

	// An empty app_id resolves against a single-tenant deployment and 404s when
	// ambiguous, so the dev widget works without one.
	b, err := h.svc.PublicBrand(c.Request.Context(), c.Query("app_id"))
	if err != nil {
		apiutil.Fail(c, err)
		return
	}
	c.Header("Cache-Control", "public, max-age=60")
	c.Header("Vary", "Authorization, Cookie")
	c.JSON(http.StatusOK, gin.H{"name": b.Name, "config": b.Config})
}
