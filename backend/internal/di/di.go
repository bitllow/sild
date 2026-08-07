// Package di is the composition root. It is the only place that knows which
// concrete implementations back the interfaces (ARCHITECTURE §3). Each binary
// builds the container, registers any role-specific providers, then Invokes.
package di

import (
	"github.com/bitllow/sild/backend/internal/api"
	"github.com/bitllow/sild/backend/internal/archive"
	"github.com/bitllow/sild/backend/internal/auth"
	"github.com/bitllow/sild/backend/internal/config"
	"github.com/bitllow/sild/backend/internal/connector/webhook"
	"github.com/bitllow/sild/backend/internal/domain"
	"github.com/bitllow/sild/backend/internal/mail"
	"github.com/bitllow/sild/backend/internal/middleware"
	"github.com/bitllow/sild/backend/internal/push"
	"github.com/bitllow/sild/backend/internal/realtime"
	"github.com/bitllow/sild/backend/internal/search"
	"github.com/bitllow/sild/backend/internal/secrets"
	"github.com/bitllow/sild/backend/internal/server"
	"github.com/bitllow/sild/backend/internal/storage"
	"github.com/bitllow/sild/backend/internal/store"
	"github.com/bitllow/sild/backend/internal/store/gormstore"
	"go.uber.org/dig"
)

// Option adjusts which implementations New registers.
type Option func(*settings)

type settings struct{ noRealtime bool }

// WithoutRealtime drops events instead of publishing them. For one-shot operator
// commands: they reach domain.Service, which needs a Publisher, but nothing is
// connected to receive — and dialing the broker would make the CLI need Redis.
func WithoutRealtime() Option { return func(s *settings) { s.noRealtime = true } }

// New builds a container with every shared provider. dig only constructs what an
// Invoke actually needs, so unused providers cost nothing per binary.
func New(opts ...Option) (*dig.Container, error) {
	var s settings
	for _, o := range opts {
		o(&s)
	}
	publisher := any(provideRealtimePublisher)
	if s.noRealtime {
		publisher = func() realtime.Publisher { return realtime.NoopPublisher{} }
	}
	c := dig.New()
	providers := []any{
		config.Load,
		gormstore.Open, // *gorm.DB
		gormstore.New,  // store.Store

		auth.NewKeyManager,         // *auth.KeyManager
		auth.NewAdminAuthenticator, // auth.AdminAuthenticator
		storage.New,                // storage.Bucket
		search.New,                 // search.Backend
		provideMailer,              // mail.Mailer
		realtime.NewNode,           // *realtime.Node (ws serves it; api publishes through it)
		publisher,                  // realtime.Publisher

		newService,         // *domain.Service (search attached)
		domain.NewSearch,   // *domain.SearchService
		middleware.NewAuth, // *middleware.Auth
		api.New,            // *api.Handler
		server.New,         // *server.Server

		// Worker dependencies (constructed only when sild-worker invokes them).
		webhook.NewRelay,    // *webhook.Relay
		archive.New,         // archive.Sink
		archive.NewJob,      // *archive.Job
		providePushNotifier, // push.Notifier
		provideSecrets,      // *secrets.Box
		push.NewFanOut,      // *push.FanOut
	}
	for _, p := range providers {
		if err := c.Provide(p); err != nil {
			return nil, err
		}
	}
	return c, nil
}

// provideRealtimePublisher runs the node's broker connection and returns the
// egress publisher. In sild-api this publishes through the broker to the sild-ws
// nodes (Redis); in single-process/dev it is an in-memory node.
func provideRealtimePublisher(node *realtime.Node) (realtime.Publisher, error) {
	if err := node.Run(); err != nil {
		return nil, err
	}
	return realtime.NewCentrifugePublisher(node.Node), nil
}

// provideMailer supplies the outbound email transport (§6.2). A real SMTP relay
// when SILD_SMTP_RELAY_ADDR is set; NoopMailer otherwise so zero-config dev
// keeps working.
func provideMailer(cfg *config.Config) mail.Mailer {
	if cfg.Email.RelayAddr == "" {
		return mail.NoopMailer{}
	}
	return mail.NewSMTPMailer(cfg.Email.RelayAddr, cfg.Email.RelayUser, cfg.Email.RelayPass, cfg.Email.From)
}

// providePushNotifier supplies the push transport. Credentials are per-tenant,
// so one transport serves every tenant — it is handed the credential per send.
func providePushNotifier() push.Notifier { return push.NewFCM() }

// provideSecrets supplies the box that seals tenant credentials. An unset key
// yields a box that fails every call: production refuses to start without one
// (config.Validate), and development gets a clear error at the write rather than
// a column that silently holds plaintext.
func provideSecrets(cfg *config.Config) (*secrets.Box, error) {
	return secrets.New(cfg.Secrets.Key)
}

// Provide registers additional providers (role-specific wiring).
func Provide(c *dig.Container, providers ...any) error {
	for _, p := range providers {
		if err := c.Provide(p); err != nil {
			return err
		}
	}
	return nil
}

// newService builds the domain service and attaches search. Search is wired
// here rather than in domain.New because both are dig providers over the same
// store, and a constructor dependency between them would be cyclic.
func newService(
	st store.Store, pub realtime.Publisher, km *auth.KeyManager, bucket storage.Bucket,
	mailer mail.Mailer, sink archive.Sink, notifier push.Notifier, box *secrets.Box,
	cfg *config.Config, ss *domain.SearchService,
) *domain.Service {
	svc := domain.New(st, pub, km, bucket, mailer, sink, notifier, box, cfg)
	svc.UseSearch(ss)
	return svc
}
