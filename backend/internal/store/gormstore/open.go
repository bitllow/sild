// Package gormstore is the GORM-backed implementation of the store interfaces.
// It is the only package that knows which SQL dialect is in use; the rest of the
// app depends on store.Store (ARCHITECTURE §3).
package gormstore

import (
	"fmt"

	"github.com/bitllow/sild/backend/internal/config"
	"log"
	"os"
	"time"

	"github.com/glebarez/sqlite" // pure-Go, cgo-free
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// Open connects to the configured database. dig provides *gorm.DB from this.
func Open(cfg *config.Config) (*gorm.DB, error) {
	var dialector gorm.Dialector
	switch cfg.DB.Driver {
	case config.Postgres:
		dialector = postgres.Open(cfg.DB.DSN)
	case config.MySQL:
		dialector = mysql.Open(cfg.DB.DSN)
	case config.SQLite:
		dialector = sqlite.Open(cfg.DB.DSN)
	default:
		return nil, fmt.Errorf("unsupported DB_DRIVER %q (want postgres|mysql|sqlite)", cfg.DB.Driver)
	}

	// IgnoreRecordNotFoundError: "not found" is an expected control-flow signal
	// here (idempotency lookups, first-time bootstrap), not a warning.
	level := logger.Warn
	if cfg.DB.LogSQL {
		level = logger.Info
	}
	gcfg := &gorm.Config{Logger: logger.New(
		log.New(os.Stdout, "\r\n", log.LstdFlags),
		logger.Config{SlowThreshold: 200 * time.Millisecond, LogLevel: level, IgnoreRecordNotFoundError: true},
	)}

	db, err := gorm.Open(dialector, gcfg)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", cfg.DB.Driver, err)
	}
	return db, nil
}

// Dialect reports the active dialect for capability branching (search, upserts).
func Dialect(db *gorm.DB) config.Driver {
	switch db.Dialector.Name() {
	case "postgres":
		return config.Postgres
	case "mysql":
		return config.MySQL
	default:
		return config.SQLite
	}
}
