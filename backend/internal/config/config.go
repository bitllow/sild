// Package config loads runtime configuration from the environment.
//
// Twelve-factor: every setting comes from an env var with a sane default so a
// clean checkout runs against SQLite with zero config (the open-source
// easy-install path). Production overrides DB_DRIVER/DB_DSN etc.
package config

import "github.com/caarlos0/env/v11"

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
	DB       DB
	Auth     Auth
	Realtime Realtime
	Storage  Storage
	Archive  Archive
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
	Backend   string `env:"STORAGE_BACKEND" envDefault:"local"` // local | gcs | s3
	Bucket    string `env:"STORAGE_BUCKET" envDefault:"sild-local"`
	Region    string `env:"STORAGE_REGION"`
	LocalDir  string `env:"STORAGE_LOCAL_DIR" envDefault:"./.uploads"`
	PublicURL string `env:"STORAGE_PUBLIC_URL" envDefault:"http://localhost:8080"`
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
	return &cfg, nil
}
