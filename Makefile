# Sild monorepo orchestrator. Delegates to each component's own Makefile.

.PHONY: build test dev dev-infra web-build backend-build backend-test backend-migrate up down logs

# Build the web drop-in FIRST so the backend embeds the current bundle (§9),
# then build the Go binaries.
build: web-build backend-build

test: backend-test

# Web drop-in (Phase 3). esbuild needs the Node pinned in web/.nvmrc (the system
# default node is often too old), so source nvm and `nvm install` it first. Copies
# the bundle into the backend's embed dir (see web/build.mjs) so `go build` picks
# it up. NVM_REQUIRED=0 skips nvm if your shell already has a new enough node.
NVM_DIR ?= $(HOME)/.nvm

web-build:
	@if [ "$(NVM_REQUIRED)" != "0" ]; then \
	  [ -s "$(NVM_DIR)/nvm.sh" ] || { echo "nvm not found at $(NVM_DIR) — install nvm, or run 'make web-build NVM_REQUIRED=0' to use the system node"; exit 1; }; \
	fi
	cd web && \
	if [ "$(NVM_REQUIRED)" != "0" ]; then . "$(NVM_DIR)/nvm.sh" && nvm install; fi && \
	npm install && npm run build

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

# ── Ship to microk8s (namespace `sild`) ──────────────────────────────────────
# Two images, one Docker Hub repo, tagged per package:
#     dmitri896/sild:<package>        moving tag the manifests reference
#     dmitri896/sild:<package>-<sha>  immutable tag `make update` pins
#
# PACKAGE = backend | inbox   (backend = api/ws/worker/mail/migrate/demo).
#
#     make pack   PACKAGE=inbox     # buildx the image for the amd64 node
#     make upload PACKAGE=inbox     # push both tags to Docker Hub
#     make deploy                   # kubectl apply the whole stack (+ migrate)
#     make update PACKAGE=inbox     # set image → rolling update, no full apply
#
# First install:
#     make pack upload PACKAGE=backend
#     make pack upload PACKAGE=inbox
#     make deploy
.PHONY: pack upload deploy update buildx-init

HUB      ?= dmitri896/sild
TAG      ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo latest)
PLATFORM ?= linux/amd64          # the node (bitllow-prod) is amd64
NS       ?= sild
K8S_DIR  ?= deploy/k8s
BUILDER  ?= sild-builder
PACKAGE  ?= backend

# Deployments that run each package's image — targeted by `make update`.
DEPLOYS_backend = sild-api sild-ws sild-worker sild-mail sild-demo
DEPLOYS_inbox   = sild-inbox

REF = $(HUB):$(PACKAGE)-$(TAG)

# A docker-container builder (+ amd64 emulation) so cross-building from an Apple
# Silicon Mac to the amd64 node works with --load. No-op once it exists.
buildx-init:
	@docker buildx inspect $(BUILDER) >/dev/null 2>&1 || \
	  docker buildx create --name $(BUILDER) --driver docker-container >/dev/null
	@docker run --rm --privileged tonistiigi/binfmt --install amd64 >/dev/null 2>&1 || true

# `pack` — build the image (moving + sha tags), both from the repo root.
pack: buildx-init
	docker buildx build --builder $(BUILDER) --platform $(PLATFORM) --load \
	  -f deploy/$(PACKAGE).Dockerfile \
	  -t $(HUB):$(PACKAGE) -t $(REF) \
	  .

# `upload` — push both tags to Docker Hub (run `docker login` once first).
upload:
	docker push $(HUB):$(PACKAGE)
	docker push $(REF)

# `deploy` — apply the whole stack; re-run migrations against the current image.
deploy:
	kubectl apply -f $(K8S_DIR)/
	kubectl -n $(NS) delete job sild-migrate --ignore-not-found
	kubectl apply -f $(K8S_DIR)/20-migrate.yaml

# `update` — only swap the image on the package's Deployments (fast path). For
# backend, also re-run the migration Job on the new image.
update:
	@for d in $(DEPLOYS_$(PACKAGE)); do \
	  echo "→ set image $$d = $(REF)"; \
	  kubectl -n $(NS) set image deployment/$$d $$d=$(REF); \
	done
	@if [ "$(PACKAGE)" = "backend" ]; then \
	  kubectl -n $(NS) delete job sild-migrate --ignore-not-found; \
	  sed 's#dmitri896/sild:backend#$(REF)#' $(K8S_DIR)/20-migrate.yaml | kubectl apply -f -; \
	fi
	@for d in $(DEPLOYS_$(PACKAGE)); do kubectl -n $(NS) rollout status deployment/$$d --timeout=180s; done
