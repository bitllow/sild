# Sild — Architecture

Monorepo for **Sild**, a multi-tenant chat platform (see [`docs/chat-platform-spec.md`](docs/chat-platform-spec.md)).
This document describes how the repo is laid out and *why*, so the six delivery
phases land without re-litigating structure.

The product spec is canonical for **behavior**; this document is canonical for
**structure**. Where the spec is Postgres-literal, this document records the
portable form we actually build (see [Cross-engine](#cross-engine-postgres--mysql--sqlite)).

---

## 1. Goals that shape the architecture

1. **Open source, easy install.** A fresh clone runs with zero infra
   (`make run` against SQLite). Production swaps in Postgres or MySQL via env.
2. **Any database the ORM supports.** GORM dialects: Postgres, MySQL, SQLite.
   Achieved by **dependency inversion** — the app depends on repository
   interfaces, never on GORM directly. The DB is a wiring choice.
3. **Multi-tenant from row zero.** `tenant_id` on every row; tenant resolved
   once per request and passed explicitly into every repository call.
4. **Phased delivery.** Backend (P1) → inbox (P2) → web widget (P3) → native
   SDKs (P4) → email (P5) → archival (P6). Each is a separate component that
   consumes the same §4/§5 contract.

---

## 2. Monorepo layout

```
sild/
├── Makefile              # orchestrator — delegates to each component
├── README.md
├── ARCHITECTURE.md       # this file
├── docs/
│   └── chat-platform-spec.md
├── backend/              # Phase 1 — Go service  ← current focus
├── inbox/                # Phase 2 — React admin/support inbox
├── web/                  # Phase 3 — drop-in widget (script bundle + @sild/react)
├── sdks/
│   ├── swift/            # Phase 4
│   └── kotlin/           # Phase 4
└── deploy/               # docker-compose (pg/mysql/redis), Dockerfiles, k8s
```

Each component owns its toolchain and Makefile. The root Makefile fans out
(`make backend/test`, `make web/build`, …) and never assumes a polyglot build —
the Go module never sees the JS/Swift/Kotlin trees and vice-versa.

---

## 3. Backend layout (Go)

The organizing principle is **dependency inversion around interfaces**. Concrete
implementations point inward at interfaces; `dig` binds them at one composition
root (`internal/di`). This is what delivers both "swap the DB" and testability.

```
backend/
├── cmd/                      # one binary per service (see §3a)
│   ├── sild-api/             #   REST (gin)        — stateless
│   ├── sild-ws/              #   realtime (Centrifuge) — holds connections
│   ├── sild-worker/          #   background jobs    — --jobs selects subset
│   └── sild-migrate/         #   AutoMigrate, then exit
├── internal/
│   ├── config/               # env-driven config (driver + DSN + addr)
│   ├── id/                   # ULID prefixed, sortable ids
│   ├── di/                   # dig container = the composition root
│   │
│   ├── server/               # gin engine + router + middleware ONLY
│   │   └── middleware/       # auth, tenant-scope, request-id, recovery
│   │
│   ├── api/                  # gin handlers, grouped by audience (§4)
│   │   ├── integration/      #   §4.1 API-key   (server↔server)
│   │   ├── user/             #   §4.2 user-JWT   (SDK + web)
│   │   ├── admin/            #   §4.3 admin-session (inbox)
│   │   └── public/           #   §4.4 jwks
│   │
│   ├── domain/               # USE CASES — pure Go, no gin, no GORM
│   │   ├── conversation/  message/  membership/
│   │   ├── assignment/    search/   token/
│   │   └── …                #   business rules, transactions, invariants
│   │
│   ├── store/                # ── PERSISTENCE BOUNDARY ──
│   │   ├── store.go          #   repository INTERFACES (domain-facing)
│   │   ├── models/           #   GORM models = the §3 schema (portable)
│   │   └── gormstore/        #   GORM impl + dialect Open() + AutoMigrate
│   │
│   ├── search/               # search.Backend iface; postgres(trgm)+portable(LIKE)
│   ├── auth/                 # JWT mint/verify, JWKS, API-key hash/verify
│   ├── realtime/             # Centrifuge node, channels, publishers (egress)
│   │
│   ├── connector/
│   │   ├── webhook/          # §6.1 outbox relay + delivery log + retries
│   │   └── email/            # §6.2 inbound parse, outbound reply
│   ├── storage/              # §11 Bucket iface: gcs | s3
│   ├── archive/              # §12 Sink iface: bigquery | gcs_json | s3_json
│   └── push/                 # §5.5 Notifier iface: FCM + APNs
```

### Layering rule (one direction only)

```
api (gin handlers)
   └─→ domain (use-cases)
          └─→ interfaces:  store.* · search.Backend · realtime.Publisher
              · storage.Bucket · archive.Sink · push.Notifier · auth.*
                 └─→ concrete impls (gormstore, centrifuge, gcs/s3, fcm…)
                        ▲ bound to interfaces by di — the ONLY place that
                          knows which DB / dialect / bucket is in use
```

- **Handlers never import GORM.** They parse/validate HTTP, call a domain
  service, render the result.
- **Domain depends only on interfaces.** Logic is unit-testable with fakes; no
  database needed.
- **`di` is the single composition root.** Swapping MySQL↔Postgres, or a real
  bucket for a fake in tests, is a one-line binding change.

> **Decision:** full domain/service layer from day one (not handlers-call-repos).
> The project is long-lived and spans six phases; keeping business rules out of
> handlers keeps them testable and the layering stable.

---

## 3a. Runtime topology (services & entrypoints)

> **Decision:** **separate binaries per service** (not one binary with role
> subcommands), and **one worker with selectable jobs**.

The spec's realtime layer is **egress-only**: `api` never calls into `ws`, it
*publishes* events to a broker after the Postgres write, and the `ws` nodes —
which hold the client connections — fan them out. The two services share state
only through the broker. That decoupling is what lets them run, deploy, and
scale independently.

| Binary | Type | State | Scales by | Needs |
|---|---|---|---|---|
| **`sild-api`** | HTTP (gin), REST §4 | stateless | request rate | store, search, auth, storage, broker (*publish only*) |
| **`sild-ws`** | WS + SSE (Centrifuge), §5 | **stateful — holds connections** | # connections | broker (subscribe), store (membership on connect), auth (JWKS) |
| **`sild-worker`** | background loops, no public HTTP | stateless | outbox/queue depth | store, broker (presence), webhook/push/email/archive deps |
| **`sild-migrate`** | one-shot, runs & exits | — | — | db |

- Email **inbound** (`POST /v1/email/inbound`) is an HTTP route with
  provider-signature auth → it lives **inside `sild-api`**.
- Email **outbound**, **webhook outbox relay**, **push fan-out**, and the
  **archival job** are background → **`sild-worker`**, selected via
  `--jobs webhook,push,email,archive`. Archival may also be triggered
  on-demand/cron (`sild-worker --jobs archive`).
- Each main.go stays thin: build the shared `di` container, then run its role.
  Shared providers (config, db, store, search, auth, broker) are registered
  once in `internal/di`; each binary differs only in which long-running
  components it starts.

### The broker decouples — and gates Redis

`api` handlers call `node.Publish(channel, event)` after committing to Postgres;
Centrifuge routes it through the **broker** to whichever process holds the
connection. Presence (used by `sild-worker` to push only to members with no live
connection) also lives in the broker.

- **Memory broker** — in-process only. Usable only when one binary holds both
  the publisher and the connections (not our deployment).
- **Redis broker** — **required**, because `api` and `ws` are separate
  processes: it is the bus that carries API-published events to the `ws` nodes
  and holds cluster-wide presence. Selected in `di` by config.

> **Consequence of separate binaries:** realtime needs **Redis**. `sild-api`
> alone (REST-only, no live push) runs against just the DB; end-to-end realtime
> requires `sild-ws` + `sild-worker` + Redis. The standard dev/OSS path is
> `make -C deploy up` (compose: DB + Redis + the four binaries). This is the
> accepted tradeoff for the per-service split.

```
       publish                          ┌─────────┐ subscribe   ┌──────┐
┌──────┐  (after PG     ┌─────────┐     │  Redis  │────────────▶│  ws  │ (×M)
│ api  │───  commit) ──▶│  Redis  │     │ broker  │  presence   │ holds│
│ (×N) │                │ broker  │     └────┬────┘             │ conns│
└──────┘                └────┬────┘          │                  └──────┘
                             │ outbox+presence│
                             ▼                ▼
                        ┌────────┐      (worker queries presence to decide
                        │ worker │       push fan-out; drains the outbox)
                        └────────┘
```

Each binary exposes its own `/healthz` + `/metrics`.

---

## 4. Cross-engine (Postgres / MySQL / SQLite)

The spec is written Postgres-literal. Three couplings can't survive verbatim
across dialects; here's the portable form:

| Spec (Postgres) | Portable form | Rationale |
|---|---|---|
| `text[]` columns (`searchable_metadata_keys`, webhook `events`) | **child tables** (`tenant_searchable_keys`, `webhook_endpoint_events`) | arrays don't exist outside PG; child tables AutoMigrate everywhere |
| `jsonb` metadata | **`datatypes.JSON`** | maps to `jsonb` / `JSON` / `TEXT` per dialect |
| `pg_trgm` GIN trigram search | **`search.Backend` interface** | trigram is PG-only; see below |
| `GREATEST()` monotonic upsert | upsert on PG+MySQL; **read-modify-write** fallback on SQLite | SQLite lacks `GREATEST` |

### Search is a capability tier, not a portable feature

`search.Backend` has two implementations, chosen by dialect at startup:

- **`postgres`** — full mixed-token search: `pg_trgm` similarity ranking on
  `messages.body` and the materialized `member_search_text`, plus jsonb
  `meta.<key>` fallback. This is the spec's intended experience.
- **`portable`** — `LIKE '%term%'` across the same columns, no similarity
  ranking. Correct, slower, unranked. MySQL can later gain a FULLTEXT/ngram
  implementation as a middle tier.

The rest of the app is dialect-blind: it calls `search.Backend`. Search quality
degrading on SQLite/MySQL is a **documented, accepted tradeoff** for easy
install — not a hidden behavior.

### Migrations

> **Decision:** **AutoMigrate + a dialect index hook.**

`gormstore.Migrate(db)`:
1. `db.AutoMigrate(models.All()...)` — builds every table on any dialect.
2. `applyDialectIndexes(db)` — branches on `db.Dialector.Name()`:
   - **postgres** → `CREATE EXTENSION pg_trgm`; `GIN (… gin_trgm_ops)` on
     `messages.body` and `member_search_text`.
   - **mysql** → `FULLTEXT` (ngram) on the same columns.
   - **sqlite** → none (LIKE path).

No SQL files to maintain by hand. Versioned migrations (golang-migrate) can be
introduced before the first production deploy if schema-change auditing is
needed; AutoMigrate covers development and the OSS install path until then.

---

## 5. Cross-cutting concerns

### Tenancy

The spec invariant — *tenant is never client-supplied; `tenant_id` on every
row* — is enforced structurally:

- **Resolved once** in `server/middleware`, from the verified `tid` claim (user
  JWT), the API-key binding (server routes), or the admin session. Never from a
  path, header, or body.
- Carried on `context.Context` as a `TenantContext`.
- **Every repository method takes `tenantID` explicitly.** No repo reads tenant
  from anywhere else, so a missing scope is a compile-time-visible omission, not
  a silent cross-tenant leak.
- `tenant_id` is a column on **every** model — including `conversation_members`,
  `message_attachments`, and `read_receipts` (which the spec's §3 listing
  omitted).

### Transactions & event durability

- `store.Tx(ctx, fn)` wraps the atomic creates the spec requires (§1):
  conversation + members + assignment commit together or not at all.
- **Webhook events** are written to an **`outbox` table inside the same
  transaction**. A relay worker in `connector/webhook` drains the outbox with
  exponential backoff and a per-attempt delivery log → at-least-once delivery
  with dedupe via `X-Sild-Event-Id`.
- The **socket stays best-effort** (Centrifuge publish *after* commit). Missed
  events are recovered by reconnect catch-up (§5.4) — no delivery guarantee, by
  design.

### Identifiers

ULIDs with a type prefix (`c_…`, `m_…`), generated in `internal/id`.
Lexicographic order == chronological order, which is load-bearing for:
- cursor pagination (`?before=` / `?after=`), and
- the monotonic read-receipt guard (compare ids directly).

---

## 6. Review findings → where they're handled

These came out of spec review; this records their resolution in the structure.

| Finding | Resolution |
|---|---|
| `tenant_id` missing on `conversation_members`, `message_attachments`, `read_receipts` | column added to every model; repo signatures take `tenantID` |
| Archive read auth vs. deleted `conversation_members` | `archive` persists a **membership snapshot** in the tombstone; archived reads authorize against the snapshot, not hot membership |
| Reconnect catch-up misses convos added while offline | SDK contract: re-fetch `/v1/me/conversations` **before** per-conv `after=` catch-up (doc-only; no backend change) |
| Unsortable message ids break pagination & `GREATEST` | ULID ids (`internal/id`) — sortable strings |
| Admin tenant resolution underspecified | admin session carries tenant + platform role; multi-tenant admins use an explicit tenant selector (open spec decision, surfaced in P2) |
| Guests not distinguishable from users | explicit `guest:true` member metadata + token scope hint; widget gates the list-view affordance on it (open spec decision, surfaced in P3) |
| Uploads have no ownership/validation record | new `uploads` model (tenant, uploader, mime/size, completion state); attach verifies the key is a completed upload in the caller's tenant |
| Assignment-close vs. conversation-close ambiguous | closing the last assignment does **not** auto-close the conversation; conversation close is its own action; archival keys on `conversation.status` (open spec decision, surfaced in P2/P6) |

Items marked "open spec decision" are behavioral choices, not blockers for the
backend skeleton; each is surfaced again at the phase that needs it.

---

## 7. Technology choices

| Concern | Choice | Note |
|---|---|---|
| HTTP | **gin** | handlers grouped by audience under `internal/api` |
| DI | **uber-go/dig** | reflection-based; single composition root in `internal/di` |
| ORM | **GORM** | CRUD via GORM; **raw SQL** for trigram search & `ON CONFLICT … GREATEST` upserts |
| SQLite driver | **glebarez/sqlite** | pure-Go (no cgo) → `make build` stays cgo-free |
| IDs | **oklog/ulid** | sortable, prefixed |
| Config | **caarlos0/env** | env-only, sane defaults |
| Realtime | **Centrifuge** (embedded) | egress-only; runs in its own `sild-ws` binary (§3a) |
| Broker | **Redis** | bus between `sild-api` (publish) and `sild-ws` (fan-out); holds presence |
| JSON columns | **gorm.io/datatypes** | portable jsonb/JSON/TEXT |

---

## 8. Build & run

Four binaries (§3a). Realtime requires Redis (separate-binary consequence).

```
make build            # builds all four (cgo-free)
make test

# run individually (REST-only works against just the DB):
make run-api          # sild-api   :8080
make run-ws           # sild-ws    :8081   (needs Redis broker)
make run-worker       # sild-worker --jobs webhook,push,email
make migrate          # sild-migrate, then exit

# full stack the easy way — DB + Redis + all four binaries:
make -C deploy up     # docker-compose
```

`backend/Makefile`: `build · run-api · run-ws · run-worker · migrate · test ·
tidy · fmt · vet · docker`. Each `run-*` target maps to one `cmd/sild-*` binary.

---

## 9. Phase map (delivery order)

| Phase | Deliverable | Component |
|---|---|---|
| 1 | Backend: auth, REST, realtime, push, webhooks, RBAC, JWKS, uploads, search, internal notes | `backend/` |
| 2 | Admin/support inbox (assignment queue + search + internal notes) | `inbox/` |
| 3 | Web drop-in (script + React + WP) — first validation target | `web/` |
| 4 | Native SDKs (Swift, Kotlin) | `sdks/` |
| 5 | Email connector (inbound parse → thread, outbound reply) | `backend/internal/connector/email` |
| 6 | Archival job → pluggable sink (BigQuery first) | `backend/internal/archive` |

All clients (P2–P4) consume the identical §4 REST + §5 realtime contract the
backend proves out in P1.
```
