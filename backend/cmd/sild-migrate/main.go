// Command sild-migrate runs AutoMigrate + the dialect index hook, then exits.
package main

import (
	"log"

	"github.com/bitllow/sild/backend/internal/config"
	"github.com/bitllow/sild/backend/internal/store/gormstore"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}
	db, err := gormstore.Open(cfg)
	if err != nil {
		log.Fatalf("open db: %v", err)
	}
	if err := gormstore.Migrate(db); err != nil {
		log.Fatalf("migrate: %v", err)
	}
	log.Printf("sild-migrate: schema up to date (driver=%s)", cfg.DB.Driver)
}
