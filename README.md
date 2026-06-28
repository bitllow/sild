# Sild

*Sild ("bridge" in Estonian) — a multi-tenant chat platform that bridges client,
driver, dispatcher, support, and email into one conversation primitive.*

One **untyped** conversation primitive serves every case (dispatcher↔client,
client↔driver, client↔support). Support is not a type — it's any conversation
carrying an **assignment**. Multi-tenant from row zero; Postgres canonical, with
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

## Services (four binaries)

| Binary | Role |
|---|---|
| `sild-api` | REST API (§4), stateless |
| `sild-ws` | Centrifuge WS/SSE egress (§5), holds connections |
| `sild-worker` | webhook relay, archival, (push/email) — `--jobs` selects |
| `sild-migrate` | AutoMigrate + dialect index hook, then exits |

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
