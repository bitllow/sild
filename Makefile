# Sild monorepo orchestrator. Delegates to each component's own Makefile.

.PHONY: build test backend-build backend-test backend-migrate up down logs

build: backend-build

test: backend-test

backend-build: ; $(MAKE) -C backend build
backend-test:  ; $(MAKE) -C backend test
backend-migrate: ; $(MAKE) -C backend migrate

# Full local stack: Postgres + Redis (+ the backend binaries).
up:   ; docker compose up -d
down: ; docker compose down
logs: ; docker compose logs -f
