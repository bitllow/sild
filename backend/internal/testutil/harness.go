// Package testutil builds an in-process Sild backend over SQLite for tests:
// real store, services, and the full gin engine, plus seed + request helpers.
// A capturing realtime publisher lets tests assert on egress events (§5).
package testutil

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync"
	"testing"

	"github.com/bitllow/sild/backend/internal/api"
	"github.com/bitllow/sild/backend/internal/archive"
	"github.com/bitllow/sild/backend/internal/auth"
	"github.com/bitllow/sild/backend/internal/config"
	"github.com/bitllow/sild/backend/internal/domain"
	"github.com/bitllow/sild/backend/internal/mail"
	"github.com/bitllow/sild/backend/internal/middleware"
	"github.com/bitllow/sild/backend/internal/realtime"
	"github.com/bitllow/sild/backend/internal/search"
	"github.com/bitllow/sild/backend/internal/storage"
	"github.com/bitllow/sild/backend/internal/store"
	"github.com/bitllow/sild/backend/internal/store/gormstore"
	"github.com/bitllow/sild/backend/internal/store/models"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// CapturePublisher records every published envelope for assertions.
type CapturePublisher struct {
	mu     sync.Mutex
	Events []Captured
}

// Captured is one published envelope plus its target.
type Captured struct {
	Target realtime.Target
	Env    realtime.Envelope
}

func (p *CapturePublisher) Publish(_ context.Context, t realtime.Target, env realtime.Envelope) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.Events = append(p.Events, Captured{Target: t, Env: env})
	return nil
}

// OfType returns captured events of a given type.
func (p *CapturePublisher) OfType(t string) []Captured {
	p.mu.Lock()
	defer p.mu.Unlock()
	var out []Captured
	for _, e := range p.Events {
		if e.Env.Type == t {
			out = append(out, e)
		}
	}
	return out
}

// Reset clears captured events.
func (p *CapturePublisher) Reset() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.Events = nil
}

// CaptureMailer records outbound email for assertions (§6.2).
type CaptureMailer struct {
	mu   sync.Mutex
	Sent []mail.OutboundEmail
}

func (m *CaptureMailer) Send(_ context.Context, msg mail.OutboundEmail) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Sent = append(m.Sent, msg)
	return nil
}

// Messages returns a copy of captured outbound mail.
func (m *CaptureMailer) Messages() []mail.OutboundEmail {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]mail.OutboundEmail(nil), m.Sent...)
}

// Harness is a fully wired backend for a single test.
type Harness struct {
	T      *testing.T
	Cfg    *config.Config
	DB     *gorm.DB
	Store  store.Store
	Svc    *domain.Service
	Search *domain.SearchService
	KM     *auth.KeyManager
	Pub    *CapturePublisher
	Mailer *CaptureMailer
	Engine *gin.Engine
}

// New builds a harness backed by a fresh SQLite file (default). Pass a DSN/driver
// via env-independent overrides for cross-dialect runs (see NewWithConfig).
func New(t *testing.T) *Harness {
	t.Helper()
	dir := t.TempDir()
	cfg := &config.Config{
		Env:      "test",
		HTTPAddr: ":0",
		DB:       config.DB{Driver: config.SQLite, DSN: filepath.Join(dir, "test.db")},
		Auth:     config.Auth{Issuer: "https://test.sild.local", DefaultTokenTTLSecs: 1800, MaxTokenTTLSecs: 3600, AdminSessionTTLHours: 168},
		Storage:  config.Storage{Backend: "local", PublicURL: "http://test.local", LocalDir: dir},
		Realtime: config.Realtime{Broker: "memory"},
		Archive:  config.Archive{Sink: "gcs_json", IdleDays: 30},
		Email:    config.Email{InboundDomain: "inbound.test", SMTPListenAddr: ":0", From: "support@inbound.test"},
	}
	return NewWithConfig(t, cfg)
}

// NewWithConfig builds a harness for an arbitrary config (used for Postgres/MySQL).
func NewWithConfig(t *testing.T, cfg *config.Config) *Harness {
	t.Helper()
	gin.SetMode(gin.TestMode)

	db, err := gormstore.Open(cfg)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := gormstore.Migrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	st := gormstore.New(db)
	pub := &CapturePublisher{}
	bucket, err := storage.New(cfg)
	if err != nil {
		t.Fatalf("storage: %v", err)
	}
	km := auth.NewKeyManager(st, cfg)
	if err := km.EnsureActiveKey(context.Background()); err != nil {
		t.Fatalf("ensure key: %v", err)
	}
	mailer := &CaptureMailer{}
	sink, err := archive.New(cfg)
	if err != nil {
		t.Fatalf("archive sink: %v", err)
	}
	svc := domain.New(st, pub, km, bucket, mailer, sink, cfg)
	searchSvc := domain.NewSearch(st, search.New(db))
	authn := auth.NewAdminAuthenticator(cfg) // dev stub (no Google configured)
	mw := middleware.NewAuth(st, km)
	h := api.New(svc, searchSvc, mw, km, authn, bucket, cfg)

	e := gin.New()
	e.Use(gin.Recovery())
	h.Mount(e)

	return &Harness{T: t, Cfg: cfg, DB: db, Store: st, Svc: svc, Search: searchSvc, KM: km, Pub: pub, Mailer: mailer, Engine: e}
}

// ── Seed helpers ────────────────────────────────────────────────────────────

// SeedTenant creates a tenant (with optional searchable metadata keys).
func (h *Harness) SeedTenant(searchableKeys ...string) *models.Tenant {
	h.T.Helper()
	t := &models.Tenant{Name: "Test Tenant", MaxAttachmentBytes: 10 << 20}
	if err := h.Store.Tenants().Create(context.Background(), t); err != nil {
		h.T.Fatalf("seed tenant: %v", err)
	}
	if len(searchableKeys) > 0 {
		if err := h.Store.Tenants().SetSearchableKeys(context.Background(), t.ID, searchableKeys); err != nil {
			h.T.Fatalf("seed keys: %v", err)
		}
	}
	return t
}

// SeedAPIKey mints an API key for a tenant and returns the full secret string.
func (h *Harness) SeedAPIKey(tenantID string) string {
	h.T.Helper()
	full, _, err := h.Svc.CreateAPIKey(context.Background(), tenantID, "test")
	if err != nil {
		h.T.Fatalf("seed api key: %v", err)
	}
	return full
}

// SeedAdmin creates an admin_user and returns it.
func (h *Harness) SeedAdmin(tenantID, email string, role models.PlatformRole) *models.AdminUser {
	h.T.Helper()
	a, err := h.Svc.InviteAgent(context.Background(), tenantID, email, role)
	if err != nil {
		h.T.Fatalf("seed admin: %v", err)
	}
	return a
}

// MintToken issues a user JWT for a host user id in a tenant.
func (h *Harness) MintToken(tenantID, userID string) string {
	h.T.Helper()
	tok, _, err := h.Svc.MintToken(context.Background(), tenantID, userID, 1800)
	if err != nil {
		h.T.Fatalf("mint token: %v", err)
	}
	return tok
}

// ── Request helpers ─────────────────────────────────────────────────────────

// Req is a request builder.
type Req struct {
	h       *Harness
	method  string
	path    string
	body    io.Reader
	headers map[string]string
	cookies map[string]string
}

// Request starts building an HTTP request against the engine.
func (h *Harness) Request(method, path string) *Req {
	return &Req{h: h, method: method, path: path, headers: map[string]string{}, cookies: map[string]string{}}
}

// JSON sets a JSON body.
func (r *Req) JSON(v any) *Req {
	b, _ := json.Marshal(v)
	r.body = bytes.NewReader(b)
	r.headers["Content-Type"] = "application/json"
	return r
}

// Raw sets a raw request body (exact bytes — for signature tests).
func (r *Req) Raw(b []byte, contentType string) *Req {
	r.body = bytes.NewReader(b)
	r.headers["Content-Type"] = contentType
	return r
}

// Header sets an arbitrary request header.
func (r *Req) Header(k, v string) *Req { r.headers[k] = v; return r }

// Bearer sets the Authorization header.
func (r *Req) Bearer(token string) *Req { r.headers["Authorization"] = "Bearer " + token; return r }

// Cookie sets a request cookie.
func (r *Req) Cookie(name, val string) *Req { r.cookies[name] = val; return r }

// Do executes the request and returns the recorder.
func (r *Req) Do() *httptest.ResponseRecorder {
	r.h.T.Helper()
	req := httptest.NewRequest(r.method, r.path, r.body)
	for k, v := range r.headers {
		req.Header.Set(k, v)
	}
	for k, v := range r.cookies {
		req.AddCookie(&http.Cookie{Name: k, Value: v})
	}
	w := httptest.NewRecorder()
	r.h.Engine.ServeHTTP(w, req)
	return w
}

// DecodeJSON unmarshals a recorder body into v.
func DecodeJSON(t *testing.T, w *httptest.ResponseRecorder, v any) {
	t.Helper()
	if err := json.Unmarshal(w.Body.Bytes(), v); err != nil {
		t.Fatalf("decode json (%d): %v\nbody: %s", w.Code, err, w.Body.String())
	}
}
