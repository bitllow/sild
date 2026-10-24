#!/usr/bin/env bash
# Runs the tech-support triage demo locally: sild-dev (seeded), the fake data
# adapter, the bridge driving Claude Code in a container, the inbox, and the
# fake integrator (Fleetline) that embeds it and validates its sign-ins.
#
#   scripts/triage-local.sh            start everything; Ctrl-C stops it all
#   RUNNER_MODE=local ...              run the CLI on the host instead of docker
#   ROUTER=fake|anthropic ...          keyword rules or Claude Haiku (default: Haiku with an API key)
#   FRESH=1 ...                        start from an empty database
#   SRC_REPOS="/path/a /path/b" ...    the code the assistant reads (default: this repo)
#   MODEL=... EFFORT=high ...          the session model and effort (default: opus, CLI default effort)
#   STEP_UP=1 ...                      data needs the requester's approval (a simulated passkey) in
#                                      the fake integrator at http://localhost:4400
#
# Model auth: ANTHROPIC_API_KEY, or bridge/.secrets/oauth-token (`claude setup-token`,
# local testing only) for Claude Code plus bridge/.secrets/api-key for the router.
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

# Claude Code runs on ANTHROPIC_API_KEY when set, else the subscription token, else
# the key file. A subscription token serves only Claude Code, so the router gets the
# API key separately and runs on keyword rules without one. The runner never sees either.
key_file="$ROOT/bridge/.secrets/api-key"
oauth_file="$ROOT/bridge/.secrets/oauth-token"
if [ -n "${ANTHROPIC_API_KEY:-}" ]; then
  model_auth=(ANTHROPIC_API_KEY="$ANTHROPIC_API_KEY")
elif [ -s "$oauth_file" ]; then
  model_auth=(CLAUDE_CODE_OAUTH_TOKEN_FILE="$oauth_file")
elif [ -s "$key_file" ]; then
  model_auth=(ANTHROPIC_API_KEY="$(cat "$key_file")")
else
  echo "set ANTHROPIC_API_KEY, or write bridge/.secrets/oauth-token or bridge/.secrets/api-key" >&2
  exit 1
fi
router_auth=()
router_default=fake
if [ -n "${ANTHROPIC_API_KEY:-}" ]; then
  router_auth=(ROUTER_API_KEY="$ANTHROPIC_API_KEY")
  router_default=anthropic
elif [ -s "$key_file" ]; then
  router_auth=(ROUTER_API_KEY_FILE="$key_file")
  router_default=anthropic
fi

# The code the assistant reads: committed files only, so nothing untracked (.env,
# local keys) is ever inside its read-only mount; key-shaped files are dropped
# even when committed.
repos="${SRC_REPOS:-$ROOT}"
if [ "$(cat "$STATE/src/.repos" 2>/dev/null)" != "$repos" ]; then
  rm -rf "$STATE/src" && mkdir -p "$STATE/src"
  for r in $repos; do
    dest="$STATE/src"
    [ -n "${SRC_REPOS:-}" ] && dest="$STATE/src/$(basename "$r")" && mkdir -p "$dest"
    git -C "$r" archive HEAD | tar -x -C "$dest"
  done
  find "$STATE/src" -type f \( -name '.env*' -o -name '*.pem' -o -name '*.key' -o -name '*.p12' \
    -o -name '*.jks' -o -name '*.keystore' -o -name '*.tfvars' -o -name '*credentials*.json' \) -delete
  # Committed key material goes entirely; credential values elsewhere are redacted in place.
  grep -rlE --null -- '-----BEGIN [A-Z ]*PRIVATE KEY-----|"private_key"[[:space:]]*:' "$STATE/src" | xargs -0 rm -f
  cred='AKIA[0-9A-Z]{16}|AIza[0-9A-Za-z_-]{35}|sk_live_[0-9A-Za-z]{20,}|rk_live_[0-9A-Za-z]{20,}|xox[abp]-[0-9A-Za-z-]{10,}|ghp_[A-Za-z0-9]{30,}|glpat-[A-Za-z0-9_-]{20}|sk-ant-[a-z0-9]+-[A-Za-z0-9_-]{20,}'
  names='(passw(or)?d|pwd|secret|api_?key|token|credential)[A-Za-z0-9_]*'
  grep -rliE --null -- "$cred|$names[\"']?[[:space:]]*[:=][[:space:]]*([\"'][^\"']{4,}[\"']|[A-Za-z0-9/+_-]{16,})" "$STATE/src" |
    CRED="$cred" xargs -0 perl -pi -e '
      s/($ENV{CRED})/REDACTED/g;
      s/((?:passw(?:or)?d|pwd|secret|api_?key|token|credential)\w*["\x27]?\s*[:=]\s*(["\x27]))[^"\x27\n]{4,}\2/${1}REDACTED${2}/gi;
      s/((?:passw(?:or)?d|pwd|secret|api_?key|token|credential)\w*["\x27]?\s*[:=]\s*)[A-Za-z0-9\/+_-]{16,}/${1}REDACTED/gi'
  echo "$repos" >"$STATE/src/.repos"
fi

if [ "$RUNNER_MODE" = docker ] && ! docker image inspect "$RUNNER_IMAGE" >/dev/null 2>&1; then
  echo "building ${RUNNER_IMAGE}…"
  docker build -t "$RUNNER_IMAGE" - < "$ROOT/bridge/runner.Dockerfile" >"$STATE/logs/image.log"
fi

echo "starting sild-dev on :${SILD_PORT}…"
(cd "$ROOT/backend" && go build -o "$STATE/sild-dev" ./cmd/sild-dev)
# exec, so the pid the cleanup kills is the server itself.
# The fake integrator may also be opened from this machine's LAN address.
lan_ip="$(ipconfig getifaddr en0 2>/dev/null || ipconfig getifaddr en1 2>/dev/null || true)"
(cd "$STATE" && exec env SILD_HTTP_ADDR=":$SILD_PORT" DB_DSN="$STATE/sild.db" \
  SILD_DEV_TRIAGE=1 SILD_DEV_TRIAGE_KEY_FILE="$STATE/assistant.key" \
  SILD_DEV_EMBED_ORIGIN="http://localhost:4400${lan_ip:+,http://$lan_ip:4400}" \
  ./sild-dev >"$STATE/logs/sild.log" 2>&1) &
pids+=($!)
wait_for "http://localhost:$SILD_PORT/healthz" sild-dev "$STATE/logs/sild.log"
for _ in $(seq 1 40); do [ -s "$STATE/assistant.key" ] && break; sleep 0.25; done
[ -s "$STATE/assistant.key" ] || { echo "sild-dev wrote no assistant key; see $STATE/logs/sild.log" >&2; exit 1; }

echo "starting the fake data adapter on :${ADAPTER_PORT}…"
[ -s "$STATE/adapter.secret" ] || (umask 077 && openssl rand -hex 24 >"$STATE/adapter.secret")
adapter_secret="$(cat "$STATE/adapter.secret")"
# The adapter checks Sild's requester assertions against sild-dev's JWKS.
tenant_id="$(curl -sf -H "Authorization: Bearer $(cat "$STATE/assistant.key")" "http://localhost:$SILD_PORT/v1/principal" |
  sed -n 's/.*"tenant_id":"\([^"]*\)".*/\1/p')"
(cd "$ROOT/bridge" && exec env ADAPTER_SECRET="$adapter_secret" PORT="$ADAPTER_PORT" \
  SILD_TENANT="$tenant_id" SILD_JWKS_URL="http://localhost:$SILD_PORT/.well-known/jwks.json" \
  ELEVATION_URL="${STEP_UP:+http://localhost:4400/elevation}" \
  node fake-adapter/server.ts >"$STATE/logs/adapter.log" 2>&1) &
pids+=($!)
for _ in $(seq 1 60); do grep -q '^ADAPTER_PUBLIC_KEY=' "$STATE/logs/adapter.log" 2>/dev/null && break; sleep 0.25; done
adapter_key="$(sed -n 's/^ADAPTER_PUBLIC_KEY=//p' "$STATE/logs/adapter.log")"
[ -n "$adapter_key" ] || { echo "fake adapter did not start; see $STATE/logs/adapter.log" >&2; exit 1; }

echo "starting the bridge (${RUNNER_MODE} runner)…"
(cd "$ROOT/bridge" && exec env \
  SILD_URL="http://localhost:$SILD_PORT" SILD_SERVICE_KEY="$(cat "$STATE/assistant.key")" \
  WORK_ROOT="$STATE/work" SRC_DIR="$STATE/src" RUNNER_MODE="$RUNNER_MODE" RUNNER_IMAGE="$RUNNER_IMAGE" \
  IDLE_RELEASE_SECONDS="${IDLE_RELEASE_SECONDS:-120}" MODEL="${MODEL:-claude-opus-5-5}" EFFORT="${EFFORT:-}" \
  ADAPTER_URL="http://127.0.0.1:$ADAPTER_PORT" ADAPTER_SECRET="$adapter_secret" ADAPTER_PUBLIC_KEY="$adapter_key" \
  ADAPTER_ALLOW_INSECURE_LOCAL=1 ROUTER="${ROUTER:-$router_default}" \
  "${model_auth[@]}" ${router_auth[@]+"${router_auth[@]}"} node src/main.ts >"$STATE/logs/bridge.log" 2>&1) &
pids+=($!)

echo "starting the inbox on :${INBOX_PORT}…"
# The inbox finds realtime on port 8080 of whatever host it was opened on, so a phone on the LAN works.
ws_url=""
[ "$SILD_PORT" = 8080 ] || ws_url="ws://localhost:$SILD_PORT/v1/ws"
(cd "$ROOT/inbox" && exec env SILD_API_URL="http://localhost:$SILD_PORT" \
  NEXT_PUBLIC_SILD_WS_URL="$ws_url" \
  npx next dev -p "$INBOX_PORT" >"$STATE/logs/inbox.log" 2>&1) &
pids+=($!)
wait_for "http://localhost:$INBOX_PORT" inbox "$STATE/logs/inbox.log"

# The dev seed registers this host's validator and origin at :4400.
echo "starting the fake integrator on :4400…"
# Unset, the integrator frames the inbox on the host it was opened on, so the LAN address works too.
inbox_url=""
[ "$INBOX_PORT" = 3000 ] || inbox_url="http://localhost:$INBOX_PORT"
(cd "$ROOT/tools/fake-integrator" && exec env INBOX_URL="$inbox_url" \
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
