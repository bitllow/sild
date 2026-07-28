// Package config loads runtime configuration from the environment.
//
// Twelve-factor: every setting comes from an env var with a sane default so a
// clean checkout runs against SQLite with zero config (the open-source
// easy-install path). Production overrides DB_DRIVER/DB_DSN etc.
package config

import (
	"fmt"
	"os"
	"strings"

	"github.com/caarlos0/env/v11"
)

// Driver identifies the SQL dialect. The store and search layers branch on it.
type Driver string

const (
	Postgres Driver = "postgres"
	MySQL    Driver = "mysql"
	SQLite   Driver = "sqlite"
)

type Config struct {
	Env      string `env:"SILD_ENV" envDefault:"development"`
	HTTPAddr string `env:"SILD_HTTP_ADDR" envDefault:":8080"`
	// Port is injected by PaaS platforms and wins over HTTPAddr — the platform,
	// not the operator, owns it.
	Port      string `env:"PORT"`
	DB        DB
	Auth      Auth
	Realtime  Realtime
	Storage   Storage
	Archive   Archive
	Email     Email
	Jobs      Jobs
	Bootstrap Bootstrap
}

// ListenAddr is the address the REST listener binds.
func (c *Config) ListenAddr() string {
	if c.Port == "" {
		return c.HTTPAddr
	}
	return ":" + c.Port
}

// Bootstrap creates the first tenant on an empty database, for platforms where
// running a one-off command is awkward. sild-admin is the general mechanism.
type Bootstrap struct {
	TenantName    string `env:"SILD_BOOTSTRAP_TENANT"`
	AdminEmail    string `env:"SILD_BOOTSTRAP_ADMIN_EMAIL"`
	AdminName     string `env:"SILD_BOOTSTRAP_ADMIN_NAME"`
	AdminPassword string `env:"SILD_BOOTSTRAP_ADMIN_PASSWORD"`
}

// Jobs selects the background work a process runs in-process. Every job is
// lease-guarded, so any number of processes may enable the same one.
type Jobs struct {
	// Enabled has no envDefault because env would fill it in for an empty value
	// too, and SILD_JOBS="" means something: a replica that runs no jobs.
	Enabled string `env:"SILD_JOBS"`
	// SMTPIngest runs the forwarded-mail receiver in-process. Off by default: the
	// PaaS targets have no raw TCP ingress and use POST /v1/email/inbound.
	SMTPIngest bool `env:"SILD_SMTP_INGEST" envDefault:"false"`
}

// DefaultJobs is what a process runs when SILD_JOBS is not set at all.
const DefaultJobs = "webhook,archive"

// Email configures the forwarding ingestion daemon (inbound) and the outbound
// SMTP relay (§6.2). Each tenant gets a forwarding address
// <inbound_token>@<InboundDomain>; orgs forward their support mailbox to it and
// the sild-mail daemon catches and ingests every message.
type Email struct {
	// InboundDomain is the host part of every tenant's forwarding address.
	InboundDomain string `env:"SILD_EMAIL_INBOUND_DOMAIN" envDefault:"inbound.sild.local"`
	// SMTPListenAddr is where the sild-mail receiver daemon listens. Behind the
	// MX in production (:25); a high port in dev so it needs no privileges.
	SMTPListenAddr string `env:"SILD_SMTP_ADDR" envDefault:":2525"`

	// Outbound relay (agent replies leave through it). When RelayAddr is empty
	// the mailer is a no-op (zero-config dev keeps working).
	RelayAddr string `env:"SILD_SMTP_RELAY_ADDR"`
	RelayUser string `env:"SILD_SMTP_RELAY_USER"`
	RelayPass string `env:"SILD_SMTP_RELAY_PASS"`
	// From is the fallback envelope/From address when a tenant configures none.
	From string `env:"SILD_SMTP_FROM" envDefault:"support@inbound.sild.local"`
}

// Auth holds token + session + admin-OIDC settings.
type Auth struct {
	Issuer               string `env:"SILD_JWT_ISSUER" envDefault:"https://chat.sild.io"`
	DefaultTokenTTLSecs  int    `env:"SILD_TOKEN_TTL_SECONDS" envDefault:"1800"`
	MaxTokenTTLSecs      int    `env:"SILD_TOKEN_MAX_TTL_SECONDS" envDefault:"3600"`
	AdminSessionTTLHours int    `env:"SILD_ADMIN_SESSION_TTL_HOURS" envDefault:"168"`
	// Google OIDC (admin auth). When unset, a dev stub login is available in
	// non-production so the inbox is usable without configuring Google.
	GoogleClientID     string `env:"SILD_GOOGLE_CLIENT_ID"`
	GoogleClientSecret string `env:"SILD_GOOGLE_CLIENT_SECRET"`
	GoogleRedirectURL  string `env:"SILD_GOOGLE_REDIRECT_URL"`
}

// Realtime selects the Centrifuge broker.
type Realtime struct {
	Broker   string `env:"SILD_BROKER" envDefault:"memory"` // memory | redis
	RedisURL string `env:"SILD_REDIS_URL" envDefault:"redis://localhost:6379"`
	WSAddr   string `env:"SILD_WS_ADDR" envDefault:":8081"`
}

// Storage selects the attachment bucket backend (§11).
type Storage struct {
	Backend string `env:"STORAGE_BACKEND" envDefault:"local"` // local | gcs | s3
	// Bucket carries no default: only the cloud backends read it, and a default
	// would let STORAGE_BACKEND=gcs start against a bucket nobody named.
	Bucket    string `env:"STORAGE_BUCKET"`
	Region    string `env:"STORAGE_REGION"`
	LocalDir  string `env:"STORAGE_LOCAL_DIR" envDefault:"./.uploads"`
	PublicURL string `env:"STORAGE_PUBLIC_URL" envDefault:"http://localhost:8080"`
	// SigningKey signs local upload URLs (§11). Without it a signed URL is a
	// permanent bearer token for any object key. Generated per-process when
	// unset, which is fine for a single dev node and wrong for a fleet — set it
	// in any deployment running more than one replica.
	SigningKey string `env:"STORAGE_SIGNING_KEY"`
	// LocalShared asserts that LocalDir is the SAME storage on every replica (a
	// shared volume, or a single node). The signing key only makes replicas agree
	// on URLs — the bytes still live wherever they were written, so without this
	// a second replica answers 404 for the first's uploads. Production will not
	// run on the local backend unless an operator states this explicitly.
	LocalShared bool `env:"STORAGE_LOCAL_SHARED" envDefault:"false"`
}

// Archive selects the cold-storage sink (§12).
type Archive struct {
	Sink     string `env:"ARCHIVE_SINK" envDefault:"gcs_json"` // bigquery | gcs_json | s3_json
	IdleDays int    `env:"ARCHIVE_IDLE_DAYS" envDefault:"30"`
}

type DB struct {
	// Driver selects the dialect: postgres | mysql | sqlite.
	Driver Driver `env:"DB_DRIVER" envDefault:"sqlite"`
	// DSN is the driver-native connection string. For sqlite this is a file
	// path (default keeps a local dev db). Examples:
	//   postgres: "host=localhost user=sild password=... dbname=sild sslmode=disable"
	//   mysql:    "sild:pass@tcp(localhost:3306)/sild?parseTime=true"
	//   sqlite:   "sild.db"
	DSN    string `env:"DB_DSN" envDefault:"sild.db"`
	LogSQL bool   `env:"DB_LOG_SQL" envDefault:"false"`
}

// Load reads the environment into a Config. dig calls this to provide *Config.
func Load() (*Config, error) {
	cfg, err := env.ParseAs[Config]()
	if err != nil {
		return nil, err
	}
	if _, set := os.LookupEnv("SILD_JOBS"); !set {
		cfg.Jobs.Enabled = DefaultJobs
	}
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return &cfg, nil
}

// RequireProduction rejects a non-production environment, for a binary that must
// not inherit the development defaults — chiefly the stub admin login.
func (c *Config) RequireProduction() error {
	if c.Env == "production" {
		return nil
	}
	return fmt.Errorf("SILD_ENV must be production (got %q): this binary serves real traffic, and development mode enables the stub admin login — use sild-dev for the zero-config path", c.Env)
}

// Validate rejects configurations that are silently wrong above one replica.
// Each of these fails invisibly at runtime — realtime that reaches half the
// users, uploads that 404 on the wrong node — so production refuses to start
// instead. Development keeps the zero-config defaults.
func (c *Config) Validate() error {
	if c.Env != "production" {
		return nil
	}
	var bad []string
	if c.Realtime.Broker != "redis" {
		bad = append(bad, "SILD_BROKER must be redis: the memory broker is per-process, so events published by one replica never reach clients connected to another")
	}
	if c.DB.Driver == SQLite {
		bad = append(bad, "DB_DRIVER must be postgres or mysql: sqlite is single-node")
	}
	if c.Storage.Backend == "local" {
		if c.Storage.SigningKey == "" {
			bad = append(bad, "STORAGE_SIGNING_KEY must be set with STORAGE_BACKEND=local: a per-process key makes every other replica reject the URLs this one signs")
		}
		if !c.Storage.LocalShared {
			bad = append(bad, "STORAGE_BACKEND=local stores attachment bytes on the node that received them, so another replica answers 404 for them: use gcs/s3, or set STORAGE_LOCAL_SHARED=true to assert STORAGE_LOCAL_DIR is one shared volume across every replica")
		}
	}
	if len(bad) == 0 {
		return nil
	}
	return fmt.Errorf("invalid production config:\n  - %s", strings.Join(bad, "\n  - "))
}
