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
	brands, err := h.svc.ListBrands(c.Request.Context(), apiutil.Tenant(c))
	if err != nil {
		apiutil.Fail(c, err)
		return
	}
	c.JSON(http.StatusOK, brandsView(brands))
}

// saveBrands replaces the tenant's whole brand set with the staged edits
// (owner/admin, §7). The request is the full list plus the active id.
func (h *Handler) saveBrands(c *gin.Context) {
	var req struct {
		Brands        []brandBody `json:"brands"`
		ActiveBrandID string      `json:"active_brand_id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.BadRequest(c, "invalid body")
		return
	}
	brands := make([]domain.Brand, 0, len(req.Brands))
	for _, b := range req.Brands {
		brands = append(brands, domain.Brand{ID: b.ID, Name: b.Name, Config: b.Config})
	}
	saved, err := h.svc.SaveBrands(c.Request.Context(), apiutil.Tenant(c), brands, req.ActiveBrandID)
	if err != nil {
		apiutil.Fail(c, err)
		return
	}
	c.JSON(http.StatusOK, brandsView(saved))
}

// getPublicBrand returns a tenant's active brand for the web drop-in's load path
// — unauthenticated, keyed by the host-embedded app id (= tenant id). Branding is
// public, so this mints no token and records no user/session; the launcher can be
// styled on first paint without side effects. Cacheable for a short window.
func (h *Handler) getPublicBrand(c *gin.Context) {
	b, err := h.svc.PublicBrand(c.Request.Context(), c.Query("app_id"))
	if err != nil {
		apiutil.Fail(c, err)
		return
	}
	c.Header("Cache-Control", "public, max-age=60")
	c.JSON(http.StatusOK, gin.H{"name": b.Name, "config": b.Config})
}

// getMyBrand returns the active brand (name + config) to a messenger surface (web
// widget / native SDK) at load. Available to any user JWT — guest or real — since
// branding is not sensitive and both surfaces authenticate the same way. The name
// backs the header fallback when no logo is set.
func (h *Handler) getMyBrand(c *gin.Context) {
	b, err := h.svc.ActiveBrand(c.Request.Context(), apiutil.Tenant(c))
	if err != nil {
		apiutil.Fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"name": b.Name, "config": b.Config})
}
