# Sild monorepo orchestrator. Delegates to each component's own Makefile.

.PHONY: build test dev dev-infra web-build backend-build backend-test backend-migrate up down logs

# Build the web drop-in FIRST so the backend embeds the current bundle (§9),
# then build the Go binaries.
build: web-build backend-build

test: backend-test

# Web drop-in (Phase 3). Needs Node 18+. Copies the bundle into the backend's
# embed dir (see web/build.mjs) so `go build` picks it up.
web-build: ; cd web && npm install && npm run build

backend-build: ; $(MAKE) -C backend build
backend-test:  ; $(MAKE) -C backend test
backend-migrate: ; $(MAKE) -C backend migrate

# All-in-one dev: build the widget (embedded), bring up Postgres + Redis from
# docker compose, then run the single-process backend dev server against them
# (REST + WS/SSE + serves /widget.js and /sild-demo). sild-dev auto-migrates on
# start, so no separate migrate step is needed.
DEV_DB_DSN    ?= host=localhost port=5433 user=sild password=sild dbname=sild sslmode=disable
DEV_REDIS_URL ?= redis://localhost:6380

dev: web-build dev-infra
	DB_DRIVER=postgres \
	DB_DSN="$(DEV_DB_DSN)" \
	SILD_BROKER=redis \
	SILD_REDIS_URL="$(DEV_REDIS_URL)" \
	$(MAKE) -C backend dev

# Bring up ONLY Postgres + Redis (the backend itself runs locally via `go run`)
# and block until both report healthy so the dev server connects cleanly.
dev-infra:
	docker compose up -d postgres redis
	@echo "waiting for postgres + redis to be healthy..."
	@until [ "$$(docker inspect -f '{{.State.Health.Status}}' $$(docker compose ps -q postgres))" = healthy ] && \
	       [ "$$(docker inspect -f '{{.State.Health.Status}}' $$(docker compose ps -q redis))"    = healthy ]; do \
	  sleep 1; \
	done
	@echo "postgres + redis healthy."

# Full local stack: Postgres + Redis (+ the backend binaries).
up:   ; docker compose up -d
down: ; docker compose down
logs: ; docker compose logs -f
