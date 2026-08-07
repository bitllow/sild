package config_test

import (
	"strings"
	"testing"

	"github.com/bitllow/sild/backend/internal/config"
)

// sild-standalone runs the whole product in one process the way sild-dev does, so
// the thing that keeps them apart is this check. Without it a standalone
// deployment left at the default SILD_ENV serves real traffic with the stub admin
// login enabled and none of the fleet-safety checks applied.
func TestRequireProductionRejectsDevelopment(t *testing.T) {
	for _, env := range []string{"", "development", "staging"} {
		cfg := &config.Config{Env: env}
		err := cfg.RequireProduction()
		if err == nil {
			t.Fatalf("SILD_ENV=%q was accepted", env)
		}
		if !strings.Contains(err.Error(), "sild-dev") {
			t.Fatalf("error does not point at the zero-config path: %v", err)
		}
	}
	if err := (&config.Config{Env: "production"}).RequireProduction(); err != nil {
		t.Fatalf("production was rejected: %v", err)
	}
}

// PaaS platforms inject the port and expect the process to use it; ignoring it
// binds a port nothing routes to, and the deployment looks healthy while timing
// out. The operator's own SILD_HTTP_ADDR applies everywhere else.
func TestListenAddrPrefersInjectedPort(t *testing.T) {
	cases := []struct {
		name, httpAddr, port, want string
	}{
		{"no injected port", ":8080", "", ":8080"},
		{"injected port wins", ":8080", "9090", ":9090"},
		{"injected port with no configured addr", "", "8080", ":8080"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := &config.Config{HTTPAddr: tc.httpAddr, Port: tc.port}
			if got := cfg.ListenAddr(); got != tc.want {
				t.Fatalf("ListenAddr() = %q, want %q", got, tc.want)
			}
		})
	}
}
