// Package testutil builds an in-process Sild backend over SQLite for tests:
// real store, services, and the full gin engine, plus seed + request helpers.
// A capturing realtime publisher lets tests assert on egress events (§5).
package testutil

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
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
	"github.com/bitllow/sild/backend/internal/jobs"
	"github.com/bitllow/sild/backend/internal/mail"
	"github.com/bitllow/sild/backend/internal/middleware"
	"github.com/bitllow/sild/backend/internal/push"
	"github.com/bitllow/sild/backend/internal/realtime"
	"github.com/bitllow/sild/backend/internal/search"
	"github.com/bitllow/sild/backend/internal/secrets"
	"github.com/bitllow/sild/backend/internal/storage"
	"github.com/bitllow/sild/backend/internal/store"
	"github.com/bitllow/sild/backend/internal/store/gormstore"
	"github.com/bitllow/sild/backend/internal/store/models"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// CapturePublisher records every published envelope for assertions. It also
// implements realtime.Subscriber so tests can assert live subscription changes
// (e.g. peer_access revoke → unsubscribe from peer channels).
type CapturePublisher struct {
	mu           sync.Mutex
	Events       []Captured
	Subscribed   []string // "userID→channel"
	Unsubscribed []string // "userID→channel"
}

func (p *CapturePublisher) Subscribe(userID, channel string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.Subscribed = append(p.Subscribed, userID+"→"+channel)
	return nil
}

func (p *CapturePublisher) Unsubscribe(userID, channel string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.Unsubscribed = append(p.Unsubscribed, userID+"→"+channel)
	return nil
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

// SentNudge is one nudge as it reached a device.
type SentNudge struct {
	ProjectID string
	Target    push.Target
	Nudge     push.Nudge
}

// CaptureNotifier records nudges for assertions (§5.5), the push counterpart of
// CaptureMailer. Fail makes the next Notify return that error, so a test can
// drive the dead-token and retry paths without a transport.
type CaptureNotifier struct {
	mu   sync.Mutex
	Sent []SentNudge
	// Fail, when set, is returned by Notify instead of delivering.
	Fail error
	// CheckErr, when set, is returned by Check — a credential the provider rejects.
	CheckErr error
}

func (n *CaptureNotifier) Notify(_ context.Context, cred push.Credential, tgt push.Target, nu push.Nudge) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.Fail != nil {
		return n.Fail
	}
	n.Sent = append(n.Sent, SentNudge{ProjectID: cred.ProjectID, Target: tgt, Nudge: nu})
	return nil
}

func (n *CaptureNotifier) Check(context.Context, push.Credential) error { return n.CheckErr }

// Nudges returns a copy of what was delivered.
func (n *CaptureNotifier) Nudges() []SentNudge {
	n.mu.Lock()
	defer n.mu.Unlock()
	return append([]SentNudge(nil), n.Sent...)
}

// Reset clears captured nudges and any injected failure.
func (n *CaptureNotifier) Reset() {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.Sent, n.Fail = nil, nil
}

// Harness is a fully wired backend for a single test.
type Harness struct {
	T        *testing.T
	Cfg      *config.Config
	DB       *gorm.DB
	Store    store.Store
	Svc      *domain.Service
	Search   *domain.SearchService
	KM       *auth.KeyManager
	Pub      *CapturePublisher
	Mailer   *CaptureMailer
	Notifier *CaptureNotifier
	Secrets  *secrets.Box
	Push     *push.FanOut
	Engine   *gin.Engine
	Bucket   storage.Bucket
	Sink     archive.Sink
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
		// A fixed key so tests exercise real sealing rather than the no-key path.
		Secrets: config.Secrets{Key: "ZGV2LW9ubHktdGVzdC1rZXktMzJieXRlcy1sb25nISE="},
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
	notifier := &CaptureNotifier{}
	box, err := secrets.New(cfg.Secrets.Key)
	if err != nil {
		t.Fatalf("secrets: %v", err)
	}
	sink, err := archive.New(cfg, bucket)
	if err != nil {
		t.Fatalf("archive sink: %v", err)
	}
	svc := domain.New(st, pub, km, bucket, mailer, sink, notifier, box, cfg)
	fanout := push.NewFanOut(st, notifier, box, svc.ResolveText)
	searchSvc := domain.NewSearch(st, search.New(db))
	svc.UseSearch(searchSvc)                 // GET /v1/conversations?q= runs search through the service
	authn := auth.NewAdminAuthenticator(cfg) // dev stub (no Google configured)
	mw := middleware.NewAuth(st, km)
	h := api.New(svc, searchSvc, mw, km, authn, bucket, cfg)

	e := gin.New()
	e.Use(gin.Recovery())
	h.Mount(e)

	return &Harness{T: t, Cfg: cfg, DB: db, Store: st, Svc: svc, Search: searchSvc, KM: km, Pub: pub, Mailer: mailer, Notifier: notifier, Secrets: box, Push: fanout, Engine: e, Bucket: bucket, Sink: sink}
}

// ── Seed helpers ────────────────────────────────────────────────────────────

// SeedTenant creates a tenant (with optional searchable metadata keys).
func (h *Harness) SeedTenant(searchableKeys ...string) *models.Tenant {
	h.T.Helper()
	t, err := h.Svc.CreateTenant(context.Background(), "Test Tenant")
	if err != nil {
		h.T.Fatalf("seed tenant: %v", err)
	}
	if len(searchableKeys) > 0 {
		if err := h.Store.Tenants().SetSearchableKeys(context.Background(), t.ID, searchableKeys); err != nil {
			h.T.Fatalf("seed keys: %v", err)
		}
	}
	return t
}

// SeedContact stores a person's profile, the way the SDK or a host backend does.
func (h *Harness) SeedContact(tenantID, externalUserID, metadata string) {
	h.T.Helper()
	if err := h.Svc.UpsertContact(context.Background(), tenantID, externalUserID, json.RawMessage(metadata)); err != nil {
		h.T.Fatalf("seed contact: %v", err)
	}
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

// SeedAdmin creates an admin_user holding one role, tenant-wide.
func (h *Harness) SeedAdmin(tenantID, email string, role models.PlatformRole) *models.AdminUser {
	h.T.Helper()
	return h.SeedAdminScoped(tenantID, email, role, models.RoleScope{})
}

// SeedAdminScoped creates an admin_user holding one role with a scope.
func (h *Harness) SeedAdminScoped(tenantID, email string, role models.PlatformRole, scope models.RoleScope) *models.AdminUser {
	h.T.Helper()
	a, err := h.Svc.InviteAgent(context.Background(), tenantID, email, "", "", role, scope)
	if err != nil {
		h.T.Fatalf("seed admin: %v", err)
	}
	return a
}

// GrantPeer gives a member the agent role that reaches peer conversations.
func (h *Harness) GrantPeer(tenantID, adminID string) {
	h.T.Helper()
	h.SetPeerScope(tenantID, adminID, true)
}

// SetPeerScope moves the member's agent assignment on or off peer conversations,
// adding the role if they do not hold it yet.
func (h *Harness) SetPeerScope(tenantID, adminID string, peer bool) {
	h.T.Helper()
	ctx := context.Background()
	scope := models.RoleScope{Peer: peer}
	err := h.Svc.RescopeRole(ctx, tenantID, adminID, models.PlatformAgent, scope)
	if errors.Is(err, domain.ErrNotFound) {
		err = h.Svc.AssignRole(ctx, tenantID, adminID, models.PlatformAgent, scope)
	}
	if err != nil {
		h.T.Fatalf("set peer scope: %v", err)
	}
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

// RunPushJob drains the nudge queue the way the background job does, so a test
// asserts on what a deployment would actually deliver rather than on the queue.
func (h *Harness) RunPushJob() {
	h.T.Helper()
	if err := jobs.RunOnce(context.Background(), jobs.Set{jobs.Push: true}, jobs.Deps{Push: h.Push}); err != nil {
		h.T.Fatalf("push job: %v", err)
	}
}

// SeedPushCredential configures a tenant's push credential directly, skipping
// the provider check a real save performs.
func (h *Harness) SeedPushCredential(tenantID, projectID string) {
	h.T.Helper()
	sealed, err := h.Secrets.Seal([]byte(`{"type":"service_account","project_id":"` + projectID + `"}`))
	if err != nil {
		h.T.Fatalf("seal credential: %v", err)
	}
	err = h.Store.PushConfigs().Upsert(context.Background(), &models.TenantPushConfig{
		TenantID: tenantID, ProjectID: projectID, ClientEmail: "push@" + projectID + ".iam.gserviceaccount.com",
		CredentialSealed: sealed, CredentialKeyID: h.Secrets.KeyID(),
	})
	if err != nil {
		h.T.Fatalf("seed push credential: %v", err)
	}
}

// SeedPushToken registers a device for a user.
func (h *Harness) SeedPushToken(tenantID, userID, token string, platform models.PushPlatform) {
	h.T.Helper()
	err := h.Store.PushTokens().Upsert(context.Background(), &models.PushToken{
		TenantID: tenantID, MemberKind: models.MemberUser, ExternalUserID: &userID,
		Platform: platform, Token: token,
	})
	if err != nil {
		h.T.Fatalf("seed push token: %v", err)
	}
}
