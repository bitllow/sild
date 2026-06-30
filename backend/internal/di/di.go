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
	"github.com/bitllow/sild/backend/internal/server"
	"github.com/bitllow/sild/backend/internal/storage"
	"github.com/bitllow/sild/backend/internal/store/gormstore"
	"go.uber.org/dig"
)

// New builds a container with every shared provider. dig only constructs what an
// Invoke actually needs, so unused providers cost nothing per binary.
func New() (*dig.Container, error) {
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
		provideRealtimePublisher,   // realtime.Publisher

		domain.New,         // *domain.Service
		domain.NewSearch,   // *domain.SearchService
		middleware.NewAuth, // *middleware.Auth
		api.New,            // *api.Handler
		server.New,         // *server.Server

		// Worker dependencies (constructed only when sild-worker invokes them).
		webhook.NewRelay,    // *webhook.Relay
		archive.New,         // archive.Sink
		archive.NewJob,      // *archive.Job
		providePushNotifier, // push.Notifier
		providePresence,     // push.PresenceChecker
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

// providePushNotifier supplies the push transport. NoopNotifier until FCM/APNs
// are configured.
func providePushNotifier() push.Notifier { return push.NoopNotifier{} }

// providePresence supplies the presence checker. AlwaysOffline until wired to
// Centrifuge presence (Redis) in sild-worker.
func providePresence() push.PresenceChecker { return push.AlwaysOffline{} }

// Provide registers additional providers (role-specific wiring).
func Provide(c *dig.Container, providers ...any) error {
	for _, p := range providers {
		if err := c.Provide(p); err != nil {
			return err
		}
	}
	return nil
}
