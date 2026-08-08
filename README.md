# Sild

*Sild ("bridge" in Estonian) — a multi-tenant chat platform 
that bridges various clients and channels into one conversation primitive.*

Multi-tenant from row zero; Postgres canonical, with
MySQL and SQLite supported via the ORM for easy self-hosting.

See [`docs/chat-platform-spec.md`](docs/chat-platform-spec.md) for the product
spec and [`ARCHITECTURE.md`](ARCHITECTURE.md) for how the monorepo is built.

## Layout

```
backend/   Phase 1 — Go service (gin + dig + GORM)   ← implemented
inbox/     Phase 2 — React admin/support inbox
web/       Phase 3 — drop-in widget
sdks/      Phase 4 — Swift / Kotlin
docs/      spec
```

## Quick start (zero infra)

A fresh clone runs against SQLite — no Postgres or Redis needed:

```bash
cd backend
make migrate        # build schema (SQLite file)
make run-api        # REST on :8080
curl localhost:8080/readyz
```

## Full stack (Postgres + Redis)

Realtime needs Redis because `sild-api` (publish) and `sild-ws` (connections)
are separate processes (see ARCHITECTURE §3a):

```bash
docker compose up -d        # postgres + redis + api + ws + worker + migrate
# REST → http://localhost:8080   WS → ws://localhost:8081/v1/ws
```

## Services

| Binary | Role |
|---|---|
| `sild-api` | REST API (§4), stateless |
| `sild-ws` | Centrifuge WS/SSE egress (§5), holds connections |
| `sild-worker` | webhook relay, archival, push nudges — `--jobs` / `SILD_JOBS` selects a subset; `--once` for cron |
| `sild-mail` | forwarded-mail SMTP ingest (§6.2) |
| `sild-migrate` | AutoMigrate + dialect index hook, then exits — the **only** thing that changes the schema (ARCHITECTURE §4) |
| `sild-standalone` | all serving roles in one process on one port — a packaging choice, still scales to N replicas |
| `sild-admin` | operator CLI: create tenants, operators, API keys |

## Deploy

One image, one schema, several topologies — you move between them with env vars
and a replica count, not a rebuild. Full guide:
[`docs/deployment.md`](docs/deployment.md).

| Tier | What runs | Store | Broker | Scales to |
|---|---|---|---|---|
| **Dev** | `sild-dev` | SQLite | memory | 1, dev only |
| **One container** | `sild-standalone` + Postgres + Redis (+ inbox) | Postgres | Redis | N |
| **Cloud Run / PaaS** | same image, N revisions + a jobs runner | Cloud SQL | Memorystore | N |
| **Split** | `api` / `ws` / `worker` / `mail` | Postgres | Redis | each on its own axis |

```bash
docker compose -f deploy/standalone/compose.yaml up --build   # → localhost:3000
# add a replica — nothing else changes:
docker compose -f deploy/standalone/compose.yaml up -d --scale sild=3
```

Then create the first tenant (there is no HTTP provisioning API by design):

```bash
sild-admin tenant create --name "Acme" --admin-email you@acme.com
```

## Tests

```bash
cd backend && make test          # full suite on SQLite
```

Spec-to-test mapping: [`backend/TESTING.md`](backend/TESTING.md).

## Configuration

Everything is env-driven with SQLite defaults — see
[`backend/.env.example`](backend/.env.example). Key knobs: `DB_DRIVER`
(postgres|mysql|sqlite), `DB_DSN`, `SILD_BROKER` (memory|redis),
`STORAGE_BACKEND` (local|gcs|s3), `ARCHIVE_SINK` (gcs_json|s3_json|bigquery).

With `SILD_ENV=production` the defaults that are single-node are refused at
boot: `SILD_BROKER` must be `redis` (the memory broker never leaves the
process), `DB_DRIVER` must not be `sqlite`, and `STORAGE_BACKEND=local` needs an
explicit `STORAGE_SIGNING_KEY` every replica shares.

One knob stays per-replica by design: the rate limiter
(`middleware/ratelimit.go`) counts in-process, so N replicas allow N× the
configured rate. It blunts brute force; it is not a fleet-wide quota.
