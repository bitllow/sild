package domain

import (
	"context"
	"encoding/json"
	"regexp"
	"strings"
	"time"

	"github.com/bitllow/sild/backend/internal/id"
	"github.com/bitllow/sild/backend/internal/store/models"
)

// brandAssetTTL is how long a resolved logo/icon signed URL stays valid. The
// widget/SDK fetch the brand at load and cache it, so a generous TTL avoids
// mid-session breakage.
const brandAssetTTL = 24 * time.Hour

// BrandConfig is one messenger look — the per-brand config the Appearance UI
// edits and the widget/SDK render (Settings → Appearance). It is stored as an
// opaque JSON blob on the brand row; this struct is the canonical shape, and
// normalize() clamps every enum to a known value so surfaces never see garbage.
//
// logo / iconImg hold data URLs in the current design (uploaded client-side).
// Production would swap these for asset URLs without changing the shape.
type BrandConfig struct {
	// Logo / IconImg hold bucket object keys (persisted). Read responses resolve
	// them into LogoURL / IconImgURL (signed GET URLs) for the client to render;
	// those URL fields are never persisted (cleared by normalize). A legacy
	// data:/http value in Logo/IconImg is passed through and rendered directly.
	Logo         string `json:"logo"`
	LogoURL      string `json:"logoUrl,omitempty"`
	Brand        string `json:"brand"`
	Theme        string `json:"theme"`        // light|dark|auto
	Font         string `json:"font"`         // system|sild|inter|figtree|dmsans
	Radius       string `json:"radius"`       // sharp|default|rounded|pillowy
	LauncherIcon string `json:"launcherIcon"` // chat|message|help|sparkle|custom
	IconImg      string `json:"iconImg"`
	IconImgURL   string `json:"iconImgUrl,omitempty"`
	LauncherPos  string `json:"launcherPos"`  // left|right
	LauncherSize string `json:"launcherSize"` // sm|md|lg
	Heading      string `json:"heading"`
	Sub          string `json:"sub"`
	Topics       string `json:"topics"` // newline-separated labels (max 4 shown)
	ShowTeam     bool   `json:"showTeam"`
	PoweredBy    bool   `json:"poweredBy"`
}

// Brand is one named look plus its config and active flag.
type Brand struct {
	ID     string
	Name   string
	Active bool
	Config BrandConfig
}

// DefaultBrandConfig mirrors the design's seed base — a sensible Sild-blue,
// light, default-cornered messenger with welcome copy and topics filled in.
func DefaultBrandConfig() BrandConfig {
	return BrandConfig{
		Brand:        "#2563FD",
		Theme:        "light",
		Font:         "sild",
		Radius:       "default",
		LauncherIcon: "chat",
		LauncherPos:  "right",
		LauncherSize: "md",
		Heading:      "Hi there.",
		Sub:          "How can we help? We typically reply in a few minutes.",
		Topics:       "Track my order\nChange pickup address\nBilling question",
		ShowTeam:     true,
		PoweredBy:    true,
	}
}

var hexColor = regexp.MustCompile(`^#(?:[0-9a-fA-F]{3}|[0-9a-fA-F]{6})$`)

func oneOf(v, def string, allowed ...string) string {
	for _, a := range allowed {
		if v == a {
			return v
		}
	}
	return def
}

// normalize clamps every enum/color to a known value, falling back to the
// default config so a malformed client payload can never reach a surface.
func (c BrandConfig) normalize() BrandConfig {
	d := DefaultBrandConfig()
	c.Theme = oneOf(c.Theme, d.Theme, "light", "dark", "auto")
	c.Font = oneOf(c.Font, d.Font, "system", "sild", "inter", "figtree", "dmsans")
	c.Radius = oneOf(c.Radius, d.Radius, "sharp", "default", "rounded", "pillowy")
	c.LauncherIcon = oneOf(c.LauncherIcon, d.LauncherIcon, "chat", "message", "help", "sparkle", "custom")
	c.LauncherPos = oneOf(c.LauncherPos, d.LauncherPos, "left", "right")
	c.LauncherSize = oneOf(c.LauncherSize, d.LauncherSize, "sm", "md", "lg")
	if !hexColor.MatchString(c.Brand) {
		c.Brand = d.Brand
	}
	// Resolved URLs are read-only projections — never persist them.
	c.LogoURL = ""
	c.IconImgURL = ""
	return c
}

// isObjectKey reports whether an asset value is a bucket object key (vs an
// already-usable data:/http URL — legacy or externally hosted).
func isObjectKey(v string) bool {
	return v != "" && !strings.HasPrefix(v, "data:") && !strings.HasPrefix(v, "http://") && !strings.HasPrefix(v, "https://")
}

// resolveAssets fills LogoURL/IconImgURL with signed GET URLs for object-key
// assets so a client can render them. Best-effort: a signing failure leaves the
// URL empty (the client falls back to the raw value / hides the image).
func (s *Service) resolveAssets(ctx context.Context, c BrandConfig) BrandConfig {
	if isObjectKey(c.Logo) {
		if u, err := s.bucket.SignGet(ctx, c.Logo, brandAssetTTL); err == nil {
			c.LogoURL = u
		}
	}
	if isObjectKey(c.IconImg) {
		if u, err := s.bucket.SignGet(ctx, c.IconImg, brandAssetTTL); err == nil {
			c.IconImgURL = u
		}
	}
	return c
}

// decodeConfig unmarshals a stored brand blob onto the defaults, so any field
// absent from an older/partial row inherits a sane value.
func decodeConfig(raw []byte) BrandConfig {
	cfg := DefaultBrandConfig()
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &cfg)
	}
	return cfg.normalize()
}

func toBrand(m models.Brand) Brand {
	return Brand{ID: m.ID, Name: m.Name, Active: m.Active, Config: decodeConfig(m.Config)}
}

// ListBrands returns the tenant's brands, seeding a single default brand the
// first time so the Appearance UI always has one to edit.
func (s *Service) ListBrands(ctx context.Context, tenantID string) ([]Brand, error) {
	rows, err := s.store.Brands().List(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		seeded, err := s.seedDefaultBrand(ctx, tenantID)
		if err != nil {
			return nil, err
		}
		return []Brand{seeded}, nil
	}
	out := make([]Brand, 0, len(rows))
	for _, r := range rows {
		b := toBrand(r)
		b.Config = s.resolveAssets(ctx, b.Config)
		out = append(out, b)
	}
	return out, nil
}

// ActiveBrand returns the active brand for a surface (widget / SDK), seeding a
// default brand if the tenant has none yet.
func (s *Service) ActiveBrand(ctx context.Context, tenantID string) (Brand, error) {
	row, err := s.store.Brands().Active(ctx, tenantID)
	if err == nil {
		b := toBrand(*row)
		b.Config = s.resolveAssets(ctx, b.Config)
		return b, nil
	}
	// No active brand (or none at all) — seed and return the default. ListBrands
	// already resolves assets.
	brands, lerr := s.ListBrands(ctx, tenantID)
	if lerr != nil {
		return Brand{}, lerr
	}
	for _, b := range brands {
		if b.Active {
			return b, nil
		}
	}
	return brands[0], nil
}

// PublicBrand returns a tenant's active brand for the unauthenticated widget
// load path (§9), keyed by the host-embedded app id (= tenant id). No token, no
// user/session record — branding is public. In single-tenant dev the app id may
// be omitted.
func (s *Service) PublicBrand(ctx context.Context, appID string) (Brand, error) {
	tenantID := appID
	if tenantID == "" {
		ids, err := s.store.Tenants().AllIDs(ctx)
		if err != nil {
			return Brand{}, err
		}
		if len(ids) != 1 {
			return Brand{}, ErrNotFound // app_id is required when multiple tenants exist
		}
		tenantID = ids[0]
	} else if _, err := s.store.Tenants().Get(ctx, tenantID); err != nil {
		return Brand{}, mapStoreErr(err)
	}
	return s.ActiveBrand(ctx, tenantID)
}

// SaveBrands replaces the tenant's whole brand set with the staged edits from
// the Appearance UI. activeID selects the active brand (falls back to the first).
// Every config is normalized; empty input is rejected so a tenant always has at
// least one brand.
func (s *Service) SaveBrands(ctx context.Context, tenantID string, brands []Brand, activeID string) ([]Brand, error) {
	if len(brands) == 0 {
		return nil, invalid("at least one brand is required")
	}
	rows := make([]models.Brand, 0, len(brands))
	activeSet := false
	for i, b := range brands {
		// Match active by the ORIGINAL client id — a brand created client-side
		// carries a temporary id that we regenerate below, so comparing after
		// regeneration would drop the active flag on a just-created brand.
		active := b.ID == activeID
		if active {
			activeSet = true
		}
		bid := b.ID
		if id.Prefix(bid) != id.Brand {
			bid = id.New(id.Brand)
		}
		name := strings.TrimSpace(b.Name)
		if name == "" {
			name = "Brand"
		}
		blob, err := json.Marshal(b.Config.normalize())
		if err != nil {
			return nil, err
		}
		rows = append(rows, models.Brand{
			ID:       bid,
			TenantID: tenantID,
			Name:     name,
			Position: i,
			Active:   active,
			Config:   blob,
		})
	}
	if !activeSet {
		rows[0].Active = true // activeID matched nothing — default to the first
	}
	if err := s.store.Brands().Replace(ctx, tenantID, rows); err != nil {
		return nil, err
	}
	// Promote any newly-referenced asset uploads from pending → completed so they
	// aren't reaped as abandoned uploads. Idempotent + best-effort.
	for _, r := range rows {
		cfg := decodeConfig(r.Config)
		for _, key := range []string{cfg.Logo, cfg.IconImg} {
			if isObjectKey(key) {
				_ = s.store.Uploads().MarkCompleted(ctx, tenantID, key)
			}
		}
	}
	out := make([]Brand, 0, len(rows))
	for _, r := range rows {
		b := toBrand(r)
		b.Config = s.resolveAssets(ctx, b.Config)
		out = append(out, b)
	}
	return out, nil
}

// seedDefaultBrand persists a single active default brand for a tenant.
func (s *Service) seedDefaultBrand(ctx context.Context, tenantID string) (Brand, error) {
	blob, err := json.Marshal(DefaultBrandConfig())
	if err != nil {
		return Brand{}, err
	}
	row := models.Brand{
		ID:       id.New(id.Brand),
		TenantID: tenantID,
		Name:     "Default",
		Position: 0,
		Active:   true,
		Config:   blob,
	}
	if err := s.store.Brands().Replace(ctx, tenantID, []models.Brand{row}); err != nil {
		return Brand{}, err
	}
	return toBrand(row), nil
}
