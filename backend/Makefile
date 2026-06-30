BIN := bin
BINARIES := sild-api sild-ws sild-worker sild-migrate sild-mail

.PHONY: build $(BINARIES) dev run-api run-ws run-worker run-mail migrate test test-pg tidy fmt vet docker clean

build: $(BINARIES)

# All-in-one: REST + WS + worker in one process (SQLite, in-memory broker).
# Zero infra, no external tools. Seeds a dev tenant + admin + API key on first run.
dev: ; go run ./cmd/sild-dev

$(BINARIES):
	go build -o $(BIN)/$@ ./cmd/$@

run-api: ; go run ./cmd/sild-api
run-ws: ; go run ./cmd/sild-ws
run-worker: ; go run ./cmd/sild-worker -jobs webhook,push,email
run-mail: ; go run ./cmd/sild-mail
migrate: ; go run ./cmd/sild-migrate

# Default test run uses SQLite (no infra). Cross-dialect tests gate on env vars.
test: ; go test ./...

# Run the full suite against Postgres + MySQL (needs docker-compose up).
test-pg:
	SILD_TEST_POSTGRES_DSN="$(SILD_TEST_POSTGRES_DSN)" \
	SILD_TEST_MYSQL_DSN="$(SILD_TEST_MYSQL_DSN)" \
	go test ./...

tidy: ; go mod tidy
fmt: ; go fmt ./...
vet: ; go vet ./...

docker:
	docker build -t sild-backend .

clean: ; rm -rf $(BIN)
