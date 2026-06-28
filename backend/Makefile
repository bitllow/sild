BIN := bin
BINARIES := sild-api sild-ws sild-worker sild-migrate

.PHONY: build $(BINARIES) run-api run-ws run-worker migrate test test-pg tidy fmt vet docker clean

build: $(BINARIES)

$(BINARIES):
	go build -o $(BIN)/$@ ./cmd/$@

run-api: ; go run ./cmd/sild-api
run-ws: ; go run ./cmd/sild-ws
run-worker: ; go run ./cmd/sild-worker -jobs webhook,push,email
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
