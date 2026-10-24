#!/usr/bin/env bash
# Runs the tech-support triage demo locally: sild-dev (seeded), the fake data
# adapter, the bridge driving Claude Code in a container, the inbox, and the
# fake integrator (Fleetline) that embeds it and validates its sign-ins.
#
#   scripts/triage-local.sh            start everything; Ctrl-C stops it all
#   RUNNER_MODE=local ...              run the CLI on the host instead of docker
#   ROUTER=anthropic ...               route requester text with a model (default: rules)
#   FRESH=1 ...                        start from an empty database
#
# Model auth: ANTHROPIC_API_KEY, or a token file at bridge/.secrets/oauth-token
# (`claude setup-token`, local testing only).
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
STATE="$ROOT/.triage-local"
[ "${FRESH:-0}" = 1 ] && rm -rf "$STATE"
mkdir -p "$STATE/work" "$STATE/logs"

export NVM_DIR="${NVM_DIR:-$HOME/.nvm}"
# shellcheck disable=SC1091
. "$NVM_DIR/nvm.sh" >/dev/null
nvm use 24 >/dev/null

SILD_PORT="${SILD_PORT:-8080}"
INBOX_PORT="${INBOX_PORT:-3000}"
ADAPTER_PORT="${ADAPTER_PORT:-8790}"
RUNNER_MODE="${RUNNER_MODE:-docker}"
RUNNER_IMAGE="${RUNNER_IMAGE:-sild-runner:2.1.296}"

pids=()
cleanup() {
  trap - INT TERM EXIT
  echo "stopping…"
  for p in ${pids[@]+"${pids[@]}"}; do pkill -P "$p" 2>/dev/null || true; kill "$p" 2>/dev/null || true; done
  wait 2>/dev/null || true
}
trap cleanup INT TERM EXIT

wait_for() { # url, name, log
  for _ in $(seq 1 120); do
    curl -sf "$1" >/dev/null 2>&1 && return 0
    sleep 0.5
  done
  echo "$2 did not come up; see $3" >&2
  exit 1
}

# Model credential for the bridge's proxy. The runner container never gets it.
if [ -n "${ANTHROPIC_API_KEY:-}" ]; then
  model_auth=(ANTHROPIC_API_KEY="$ANTHROPIC_API_KEY")
elif [ -s "$ROOT/bridge/.secrets/oauth-token" ]; then
  model_auth=(CLAUDE_CODE_OAUTH_TOKEN_FILE="$ROOT/bridge/.secrets/oauth-token")
else
  echo "set ANTHROPIC_API_KEY or write bridge/.secrets/oauth-token" >&2
  exit 1
fi

# The code the assistant reads: tracked files only, so nothing untracked (keys,
# tokens) is ever inside its read-only mount.
if [ ! -d "$STATE/src" ]; then
  mkdir -p "$STATE/src"
  git -C "$ROOT" archive HEAD | tar -x -C "$STATE/src"
fi

if [ "$RUNNER_MODE" = docker ] && ! docker image inspect "$RUNNER_IMAGE" >/dev/null 2>&1; then
  echo "building ${RUNNER_IMAGE}…"
  docker build -t "$RUNNER_IMAGE" - < "$ROOT/bridge/runner.Dockerfile" >"$STATE/logs/image.log"
fi

echo "starting sild-dev on :${SILD_PORT}…"
(cd "$ROOT/backend" && go build -o "$STATE/sild-dev" ./cmd/sild-dev)
# exec, so the pid the cleanup kills is the server itself.
(cd "$STATE" && exec env SILD_HTTP_ADDR=":$SILD_PORT" DB_DSN="$STATE/sild.db" \
  SILD_DEV_TRIAGE=1 SILD_DEV_TRIAGE_KEY_FILE="$STATE/assistant.key" \
  ./sild-dev >"$STATE/logs/sild.log" 2>&1) &
pids+=($!)
wait_for "http://localhost:$SILD_PORT/healthz" sild-dev "$STATE/logs/sild.log"
for _ in $(seq 1 40); do [ -s "$STATE/assistant.key" ] && break; sleep 0.25; done
[ -s "$STATE/assistant.key" ] || { echo "sild-dev wrote no assistant key; see $STATE/logs/sild.log" >&2; exit 1; }

echo "starting the fake data adapter on :${ADAPTER_PORT}…"
[ -s "$STATE/adapter.secret" ] || (umask 077 && openssl rand -hex 24 >"$STATE/adapter.secret")
adapter_secret="$(cat "$STATE/adapter.secret")"
(cd "$ROOT/bridge" && exec env ADAPTER_SECRET="$adapter_secret" PORT="$ADAPTER_PORT" \
  node fake-adapter/server.ts >"$STATE/logs/adapter.log" 2>&1) &
pids+=($!)
for _ in $(seq 1 60); do grep -q '^ADAPTER_PUBLIC_KEY=' "$STATE/logs/adapter.log" 2>/dev/null && break; sleep 0.25; done
adapter_key="$(sed -n 's/^ADAPTER_PUBLIC_KEY=//p' "$STATE/logs/adapter.log")"
[ -n "$adapter_key" ] || { echo "fake adapter did not start; see $STATE/logs/adapter.log" >&2; exit 1; }

echo "starting the bridge (${RUNNER_MODE} runner)…"
(cd "$ROOT/bridge" && exec env \
  SILD_URL="http://localhost:$SILD_PORT" SILD_SERVICE_KEY="$(cat "$STATE/assistant.key")" \
  WORK_ROOT="$STATE/work" SRC_DIR="$STATE/src" RUNNER_MODE="$RUNNER_MODE" RUNNER_IMAGE="$RUNNER_IMAGE" \
  IDLE_RELEASE_SECONDS="${IDLE_RELEASE_SECONDS:-120}" MODEL="${MODEL:-claude-sonnet-5-5}" \
  ADAPTER_URL="http://127.0.0.1:$ADAPTER_PORT" ADAPTER_SECRET="$adapter_secret" ADAPTER_PUBLIC_KEY="$adapter_key" \
  ADAPTER_ALLOW_INSECURE_LOCAL=1 ROUTER="${ROUTER:-fake}" \
  "${model_auth[@]}" node src/main.ts >"$STATE/logs/bridge.log" 2>&1) &
pids+=($!)

echo "starting the inbox on :${INBOX_PORT}…"
(cd "$ROOT/inbox" && exec env SILD_API_URL="http://localhost:$SILD_PORT" \
  NEXT_PUBLIC_SILD_WS_URL="ws://localhost:$SILD_PORT/v1/ws" \
  npx next dev -p "$INBOX_PORT" >"$STATE/logs/inbox.log" 2>&1) &
pids+=($!)
wait_for "http://localhost:$INBOX_PORT" inbox "$STATE/logs/inbox.log"

# The dev seed registers this host's validator and origin at :4400.
echo "starting the fake integrator on :4400…"
(cd "$ROOT/tools/fake-integrator" && exec env INBOX_URL="http://localhost:$INBOX_PORT" \
  node server.ts >"$STATE/logs/integrator.log" 2>&1) &
pids+=($!)
wait_for "http://localhost:4400" "fake integrator" "$STATE/logs/integrator.log"

cat <<EOF

  inbox      http://localhost:$INBOX_PORT
  requester  marko@sild.local / password123   → Tech support → New request
  operator   admin@sild.local / password123   → Tech support (every chat, data requests)
  embedded   http://localhost:4400              Fleetline host app; switch users in its top bar
  logs       $STATE/logs/{sild,adapter,bridge,inbox,integrator}.log

  Try: "Andres Kuusk hasn't received any new trips since this morning."
       "Our ERP webhook stopped working, it gets 401."
       "Adding a van fails with VIN invalid: WV1ZZZSYZP9012345"
       "The September invoice export has 212 orders but dispatch shows 219."

  Ctrl-C stops everything.
EOF
wait
