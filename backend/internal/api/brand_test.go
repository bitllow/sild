package api_test

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/bitllow/sild/backend/internal/store/models"
	"github.com/bitllow/sild/backend/internal/testutil"
)

type brandConfigJSON struct {
	Logo         string `json:"logo"`
	LogoURL      string `json:"logoUrl"`
	IconImg      string `json:"iconImg"`
	IconImgURL   string `json:"iconImgUrl"`
	Brand        string `json:"brand"`
	Theme        string `json:"theme"`
	Font         string `json:"font"`
	Radius       string `json:"radius"`
	LauncherIcon string `json:"launcherIcon"`
	LauncherPos  string `json:"launcherPos"`
	LauncherSize string `json:"launcherSize"`
	Heading      string `json:"heading"`
	Sub          string `json:"sub"`
	Topics       string `json:"topics"`
	ShowTeam     bool   `json:"showTeam"`
	PoweredBy    bool   `json:"poweredBy"`
}

type brandJSON struct {
	ID     string          `json:"id"`
	Name   string          `json:"name"`
	Config brandConfigJSON `json:"config"`
}

type brandsJSON struct {
	Brands        []brandJSON `json:"brands"`
	ActiveBrandID string      `json:"active_brand_id"`
}

// First GET seeds a single active default brand so the Appearance UI always has
// one to edit (§8).
func TestBrandsSeedsDefault(t *testing.T) {
	h := testutil.New(t)
	tenant := h.SeedTenant()
	h.SeedAdmin(tenant.ID, "owner@test", models.PlatformOwner)
	owner := loginAs(t, h, "owner@test")

	var got brandsJSON
	w := h.Request("GET", "/v1/brands").Cookie("sild_admin", owner).Do()
	if w.Code != http.StatusOK {
		t.Fatalf("get: %d %s", w.Code, w.Body)
	}
	testutil.DecodeJSON(t, w, &got)
	if len(got.Brands) != 1 {
		t.Fatalf("expected 1 seeded brand, got %d", len(got.Brands))
	}
	b := got.Brands[0]
	if got.ActiveBrandID != b.ID {
		t.Fatalf("active id %q != seeded brand id %q", got.ActiveBrandID, b.ID)
	}
	if b.Config.Brand != "#2563FD" || b.Config.Theme != "light" || !b.Config.PoweredBy {
		t.Fatalf("unexpected default config: %+v", b.Config)
	}
}

// PUT replaces the whole set, normalizes garbage enums/colors, honors the active
// selection (incl. a brand created client-side with a temp id), and persists.
func TestBrandsSaveNormalizeAndActivate(t *testing.T) {
	h := testutil.New(t)
	tenant := h.SeedTenant()
	h.SeedAdmin(tenant.ID, "owner@test", models.PlatformOwner)
	owner := loginAs(t, h, "owner@test")

	// A valid brand plus a client-created one (temp id "new_x") marked active,
	// carrying garbage in several fields that must be clamped to defaults.
	body := map[string]any{
		"active_brand_id": "new_x",
		"brands": []map[string]any{
			{"id": "", "name": "Acme Rides", "config": map[string]any{"brand": "#1F8A5B", "theme": "dark", "radius": "pillowy"}},
			{"id": "new_x", "name": "  ", "config": map[string]any{
				"brand": "not-a-hex", "theme": "neon", "font": "comic", "launcherSize": "huge", "heading": "Need a hand?",
			}},
		},
	}
	var got brandsJSON
	w := h.Request("PUT", "/v1/brands").Cookie("sild_admin", owner).Header("If-Match", "*").JSON(body).Do()
	if w.Code != http.StatusOK {
		t.Fatalf("put: %d %s", w.Code, w.Body)
	}
	testutil.DecodeJSON(t, w, &got)
	if len(got.Brands) != 2 {
		t.Fatalf("expected 2 brands, got %d", len(got.Brands))
	}
	// The active brand is the one that came in with the temp id — now assigned a
	// real br_ id, blank name defaulted, and garbage clamped.
	active := brandByID(t, got, got.ActiveBrandID)
	if active.Config.Heading != "Need a hand?" {
		t.Fatalf("active is not the temp-id brand: %+v", active)
	}
	if active.ID[:3] != "br_" {
		t.Fatalf("temp id not replaced with a real id: %q", active.ID)
	}
	if active.Name != "Brand" {
		t.Fatalf("blank name not defaulted: %q", active.Name)
	}
	if active.Config.Brand != "#2563FD" || active.Config.Theme != "light" || active.Config.Font != "sild" || active.Config.LauncherSize != "md" {
		t.Fatalf("garbage not normalized: %+v", active.Config)
	}

	// Re-GET: the set persisted and the active selection stuck.
	var reload brandsJSON
	w = h.Request("GET", "/v1/brands").Cookie("sild_admin", owner).Do()
	testutil.DecodeJSON(t, w, &reload)
	if len(reload.Brands) != 2 || reload.ActiveBrandID != got.ActiveBrandID {
		t.Fatalf("did not persist: brands=%d active=%q want=%q", len(reload.Brands), reload.ActiveBrandID, got.ActiveBrandID)
	}
}

// An empty brand set is rejected — a tenant always keeps at least one brand.
func TestBrandsRejectEmpty(t *testing.T) {
	h := testutil.New(t)
	tenant := h.SeedTenant()
	h.SeedAdmin(tenant.ID, "owner@test", models.PlatformOwner)
	owner := loginAs(t, h, "owner@test")

	w := h.Request("PUT", "/v1/brands").Cookie("sild_admin", owner).Header("If-Match", "*").
		JSON(map[string]any{"active_brand_id": "", "brands": []any{}}).Do()
	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422 for empty set, got %d %s", w.Code, w.Body)
	}
}

// The widget/SDK endpoint returns the active brand (name + config) to any user
// JWT and reflects saved edits (§9).
func TestMyBrandReturnsActive(t *testing.T) {
	h := testutil.New(t)
	tenant := h.SeedTenant()
	h.SeedAdmin(tenant.ID, "owner@test", models.PlatformOwner)
	owner := loginAs(t, h, "owner@test")
	tok := h.MintToken(tenant.ID, "u_widget")

	// Save a single custom active brand.
	w := h.Request("PUT", "/v1/brands").Cookie("sild_admin", owner).Header("If-Match", "*").JSON(map[string]any{
		"active_brand_id": "keep",
		"brands": []map[string]any{
			{"id": "keep", "name": "Acme Business", "config": map[string]any{"brand": "#7C3AED", "heading": "Hello!"}},
		},
	}).Do()
	if w.Code != http.StatusOK {
		t.Fatalf("save: %d %s", w.Code, w.Body)
	}

	var got struct {
		Name   string          `json:"name"`
		Config brandConfigJSON `json:"config"`
	}
	w = h.Request("GET", "/v1/brands/active").Bearer(tok).Do()
	if w.Code != http.StatusOK {
		t.Fatalf("me/brand: %d %s", w.Code, w.Body)
	}
	testutil.DecodeJSON(t, w, &got)
	if got.Name != "Acme Business" || got.Config.Brand != "#7C3AED" || got.Config.Heading != "Hello!" {
		t.Fatalf("me/brand did not reflect active brand: %+v / %+v", got.Name, got.Config)
	}
}

// Brands are owner/admin only (§7): an agent cannot read or write them.
func TestBrandsRequireOwnerAdmin(t *testing.T) {
	h := testutil.New(t)
	tenant := h.SeedTenant()
	h.SeedAdmin(tenant.ID, "agent@test", models.PlatformAgent)
	agent := loginAs(t, h, "agent@test")

	if w := h.Request("GET", "/v1/brands").Cookie("sild_admin", agent).Do(); w.Code != http.StatusForbidden {
		t.Fatalf("agent GET: expected 403, got %d", w.Code)
	}
	if w := h.Request("PUT", "/v1/brands").Cookie("sild_admin", agent).Header("If-Match", "*").
		JSON(map[string]any{"brands": []any{}}).Do(); w.Code != http.StatusForbidden {
		t.Fatalf("agent PUT: expected 403, got %d", w.Code)
	}
}

// The unauthenticated widget-load path returns the active brand keyed by app id
// (= tenant id), and in single-tenant dev the app id may be omitted — with no
// token and no user record created (§9, P2 review fix).
func TestPublicBrandUnauthenticated(t *testing.T) {
	h := testutil.New(t)
	tenant := h.SeedTenant()

	var got struct {
		Name   string          `json:"name"`
		Config brandConfigJSON `json:"config"`
	}
	// Keyed by app id, no auth headers/cookies at all.
	w := h.Request("GET", "/v1/brands/active?app_id="+tenant.ID).Do()
	if w.Code != http.StatusOK {
		t.Fatalf("public brand (app_id): %d %s", w.Code, w.Body)
	}
	testutil.DecodeJSON(t, w, &got)
	if got.Config.Brand != "#2563FD" {
		t.Fatalf("default brand not returned: %+v", got.Config)
	}
	// app_id is required, never inferred from a sole tenant: otherwise the response
	// would depend on how many tenants the deployment holds, and an anonymous
	// caller could read tenant data without naming the application.
	w = h.Request("GET", "/v1/brands/active").Do()
	if w.Code != http.StatusBadRequest {
		t.Fatalf("public brand without app_id: expected 400, got %d %s", w.Code, w.Body)
	}
	if !strings.Contains(w.Body.String(), `"app_id"`) {
		t.Errorf("the 400 must name the missing field, got %s", w.Body)
	}
	// Unknown tenant → 404.
	if w = h.Request("GET", "/v1/brands/active?app_id=t_does_not_exist").Do(); w.Code != http.StatusNotFound {
		t.Fatalf("unknown app_id: expected 404, got %d", w.Code)
	}
}

// A logo stored as a bucket object key is persisted as the key and resolved to a
// signed GET URL (logoUrl) on read — never a base64 blob in the row.
func TestBrandLogoStoredAsObjectKeyResolvedToURL(t *testing.T) {
	h := testutil.New(t)
	tenant := h.SeedTenant()
	h.SeedAdmin(tenant.ID, "owner@test", models.PlatformOwner)
	owner := loginAs(t, h, "owner@test")
	tok := h.MintToken(tenant.ID, "u_widget")

	// A real upload grant → an object key OWNED by this tenant. resolveAssets only
	// signs owned keys, so a fabricated key would (correctly) not resolve.
	var grant struct {
		ObjectKey string `json:"object_key"`
	}
	gw := h.Request("POST", "/v1/uploads").Cookie("sild_admin", owner).
		JSON(map[string]any{"mime_type": "image/png", "size_bytes": 1024, "filename": "logo.png"}).Do()
	if gw.Code != http.StatusOK && gw.Code != http.StatusCreated {
		t.Fatalf("upload grant: %d %s", gw.Code, gw.Body)
	}
	testutil.DecodeJSON(t, gw, &grant)
	key := grant.ObjectKey

	w := h.Request("PUT", "/v1/brands").Cookie("sild_admin", owner).Header("If-Match", "*").JSON(map[string]any{
		"active_brand_id": "a",
		"brands": []map[string]any{
			{"id": "a", "name": "Acme", "config": map[string]any{"brand": "#2563FD", "logo": key}},
		},
	}).Do()
	if w.Code != http.StatusOK {
		t.Fatalf("save: %d %s", w.Code, w.Body)
	}

	var got brandsJSON
	w = h.Request("GET", "/v1/brands").Cookie("sild_admin", owner).Do()
	testutil.DecodeJSON(t, w, &got)
	b := got.Brands[0]
	if b.Config.Logo != key {
		t.Fatalf("logo should persist as the object key, got %q", b.Config.Logo)
	}
	if !strings.Contains(b.Config.LogoURL, key) || !strings.Contains(b.Config.LogoURL, "/uploads/") {
		t.Fatalf("logo not resolved to a signed URL: %q", b.Config.LogoURL)
	}

	// The widget/SDK read path resolves it too.
	var mine struct {
		Config brandConfigJSON `json:"config"`
	}
	w = h.Request("GET", "/v1/brands/active").Bearer(tok).Do()
	testutil.DecodeJSON(t, w, &mine)
	if mine.Config.Logo != key || !strings.Contains(mine.Config.LogoURL, key) {
		t.Fatalf("me/brand did not resolve logo: %+v", mine.Config)
	}
}

// The unauthenticated public brand path is READ-ONLY: a page view for a tenant
// with no brand yet returns an in-memory default and writes no DB rows (no
// "page view causes backend side effects").
func TestPublicBrandDoesNotSeed(t *testing.T) {
	h := testutil.New(t)
	tenant := h.SeedTenant()

	w := h.Request("GET", "/v1/brands/active?app_id="+tenant.ID).Do()
	if w.Code != http.StatusOK {
		t.Fatalf("public brand: %d %s", w.Code, w.Body)
	}
	rows, err := h.Store.Brands().List(context.Background(), tenant.ID)
	if err != nil {
		t.Fatalf("list brands: %v", err)
	}
	if len(rows) != 0 {
		t.Fatalf("public read must not seed rows, found %d", len(rows))
	}
}

// A brand config may persist any string in logo/iconImg, but only keys backed by
// an upload owned by the tenant get signed — otherwise the public endpoint would
// be a cross-tenant object-signing oracle.
func TestBrandCrossTenantAssetNotSigned(t *testing.T) {
	h := testutil.New(t)
	tenant := h.SeedTenant()
	h.SeedAdmin(tenant.ID, "owner@test", models.PlatformOwner)
	owner := loginAs(t, h, "owner@test")

	foreignKey := "t_other/obj_stolen/secret.png" // not this tenant's upload
	w := h.Request("PUT", "/v1/brands").Cookie("sild_admin", owner).Header("If-Match", "*").JSON(map[string]any{
		"active_brand_id": "a",
		"brands": []map[string]any{
			{"id": "a", "name": "X", "config": map[string]any{"brand": "#2563FD", "logo": foreignKey}},
		},
	}).Do()
	if w.Code != http.StatusOK {
		t.Fatalf("save: %d %s", w.Code, w.Body)
	}

	var got brandsJSON
	w = h.Request("GET", "/v1/brands").Cookie("sild_admin", owner).Do()
	testutil.DecodeJSON(t, w, &got)
	b := got.Brands[0]
	if b.Config.LogoURL != "" {
		t.Fatalf("unowned key must not be signed, got logoUrl=%q", b.Config.LogoURL)
	}
}

func brandByID(t *testing.T, bs brandsJSON, id string) brandJSON {
	t.Helper()
	for _, b := range bs.Brands {
		if b.ID == id {
			return b
		}
	}
	t.Fatalf("brand %q not found in %+v", id, bs.Brands)
	return brandJSON{}
}
