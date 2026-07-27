package config_test

import (
	"strings"
	"testing"

	"github.com/bitllow/sild/backend/internal/config"
)

// A production config that would corrupt silently above one replica must fail at
// boot instead. Each of these is invisible at runtime: realtime that reaches
// half the users, uploads that 404 on the wrong node.
func TestProductionRejectsSingleNodeConfig(t *testing.T) {
	valid := func() *config.Config {
		return &config.Config{
			Env:      "production",
			DB:       config.DB{Driver: config.Postgres, DSN: "host=db"},
			Realtime: config.Realtime{Broker: "redis"},
			Storage:  config.Storage{Backend: "gcs"},
		}
	}
	if err := valid().Validate(); err != nil {
		t.Fatalf("a correct production config was rejected: %v", err)
	}

	cases := []struct {
		name   string
		broken func(*config.Config)
		want   string
	}{
		{"memory broker", func(c *config.Config) { c.Realtime.Broker = "memory" }, "SILD_BROKER"},
		{"sqlite", func(c *config.Config) { c.DB.Driver = config.SQLite }, "DB_DRIVER"},
		{"local storage without a shared signing key", func(c *config.Config) {
			c.Storage.Backend = "local"
			c.Storage.LocalShared = true
		}, "STORAGE_SIGNING_KEY"},
		{"local storage no operator says is shared", func(c *config.Config) {
			c.Storage.Backend = "local"
			c.Storage.SigningKey = "shared-secret"
		}, "STORAGE_LOCAL_SHARED"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := valid()
			tc.broken(cfg)
			err := cfg.Validate()
			if err == nil {
				t.Fatalf("production started with %s", tc.name)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error does not name %s: %v", tc.want, err)
			}
		})
	}
}

// Local storage is allowed in production only when the operator states both that
// URLs verify everywhere (a shared key) and that the bytes are reachable
// everywhere (one shared volume). A shared key alone is not enough: it makes
// replicas agree on signatures while each still holds its own files.
func TestLocalStorageNeedsSharedKeyAndSharedVolume(t *testing.T) {
	cfg := &config.Config{
		Env:      "production",
		DB:       config.DB{Driver: config.Postgres},
		Realtime: config.Realtime{Broker: "redis"},
		Storage:  config.Storage{Backend: "local", SigningKey: "shared-secret", LocalShared: true},
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("rejected: %v", err)
	}
}

// The zero-config developer path must keep working: SQLite, memory broker, local
// storage, no keys.
func TestDevelopmentKeepsZeroConfigDefaults(t *testing.T) {
	cfg := &config.Config{
		Env:      "development",
		DB:       config.DB{Driver: config.SQLite, DSN: "sild.db"},
		Realtime: config.Realtime{Broker: "memory"},
		Storage:  config.Storage{Backend: "local"},
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("development config rejected: %v", err)
	}
}
