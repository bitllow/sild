package di_test

import (
	"path/filepath"
	"testing"

	"github.com/bitllow/sild/backend/internal/di"
	"github.com/bitllow/sild/backend/internal/server"
)

// The container must build the full sild-api graph without duplicate-provider or
// missing-dependency errors (regression guard for the cmd wiring).
func TestContainerBuildsServer(t *testing.T) {
	t.Setenv("DB_DRIVER", "sqlite")
	t.Setenv("DB_DSN", filepath.Join(t.TempDir(), "di.db"))
	t.Setenv("STORAGE_LOCAL_DIR", t.TempDir())
	t.Setenv("SILD_BROKER", "memory")

	c, err := di.New()
	if err != nil {
		t.Fatalf("di.New: %v", err)
	}
	if err := c.Invoke(func(s *server.Server) error {
		if s == nil {
			t.Fatal("nil server")
		}
		return nil
	}); err != nil {
		t.Fatalf("invoke *server.Server: %v", err)
	}
}
