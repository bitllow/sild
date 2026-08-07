# Deploying Sild

One image, one schema, several topologies. You move between them by changing env
vars and a replica count — never by rebuilding, and never by changing the shape of
your configuration.

Two rules hold everywhere:

- **`sild-migrate` is the only process that changes the schema** (ARCHITECTURE
  §4). No serving process migrates — not on startup, not lazily. Run `sild-migrate`
  before the serving containers, on every release. Nothing detects a schema you
  forgot to migrate: the process starts, `/readyz` passes, and the queries that
  need the missing column fail one by one. Sequence the migration, don't rely on a
  check.
- **`sild-standalone` is a production binary.** It requires `SILD_ENV=production`
  and holds to the fleet-safety checks in `config.Validate`. `sild-dev` is the
  zero-config path; the two never overlap.

## The image

One image holds every binary; the topology picks one with `command`. It is built
from the **repo root** (not `./backend`), because the drop-in widget bundle is
compiled and embedded in the same build (§9) — building from `./backend` would
embed a stale bundle.

Compose builds it for you (`up --build`). Everywhere else you push it to a registry
first, and it must be **linux/amd64**: Cloud Run, GKE and most managed nodes are
amd64, so an image built natively on an Apple Silicon Mac will not run.

```sh
# Docker Hub (what deploy/k8s/ references: dmitri896/sild:backend)
make pack upload PACKAGE=backend
```

`pack` cross-builds via a buildx builder with amd64 emulation and tags both
`:backend` and `:backend-<sha>`; `upload` pushes both. `docker login` once first.

For Cloud Run, use **Artifact Registry** in the same project — Cloud Run cannot
pull from a private Docker Hub repo without extra credentials, and a same-region
registry pulls faster:

```sh
gcloud artifacts repositories create sild --repository-format=docker --location=REGION
gcloud auth configure-docker REGION-docker.pkg.dev

IMAGE=REGION-docker.pkg.dev/PROJECT/sild/backend
docker buildx build --platform linux/amd64 \
  -f deploy/backend.Dockerfile -t $IMAGE:$(git rev-parse --short HEAD) -t $IMAGE .
docker push --all-tags $IMAGE
```

Deploy the immutable `<sha>` tag rather than the moving one, so a revision is
reproducible and a rollback is a revision switch. The inbox is a second image
(`deploy/inbox.Dockerfile`, `make pack upload PACKAGE=inbox`) — remember it bakes
`SILD_API_URL` and `NEXT_PUBLIC_*` at build time, so it is rebuilt per environment,
not reconfigured.

## Tiers

| Tier | What runs | Store | Broker | Scales to |
|---|---|---|---|---|
| **Dev** | `sild-dev` | SQLite file | memory | 1, dev only |
| **One container** | `sild-standalone` + Postgres + Redis (+ inbox) | Postgres | Redis | N — add replicas |
| **Cloud Run / PaaS** | same image, N revisions + a jobs runner | Cloud SQL | Memorystore | N |
| **Split** | `api` / `ws` / `worker` / `mail` / `migrate` | Postgres | Redis | each on its own axis |

`sild-standalone` runs every serving role in one process: REST, the Centrifuge
WS/SSE transport, and the background jobs. That is a packaging choice, not a
different architecture. N standalone replicas behind a load balancer scale exactly
like N `api` + M `ws` pods, because each replica holds its own connections and
fans out through the same Redis broker.

> **Attachment storage caps this today.** `STORAGE_BACKEND=gcs` and `s3` are not
> implemented — `storage.New` returns `"gcs storage backend not yet wired"` and the
> process will not start. `local` is the only working backend, so every replica
> needs one genuinely shared filesystem at `STORAGE_LOCAL_DIR` (a compose volume,
> an RWX PVC). Where you cannot provide one — Cloud Run, multi-host — you are held
> to a single replica until the object-storage backend lands. Everything below that
> says `gcs` is what the topology wants, not what runs.

It requires Postgres or MySQL and Redis, for the same reason the split deployment
does: SQLite is single-node, and the memory broker never leaves the process, so
events published by one replica would never reach clients connected to another.
Refusing them at boot is what makes "add a replica" a safe instruction instead of
a change that half-works in production.

## One container

```sh
docker compose -f deploy/standalone/compose.yaml up --build
open http://localhost:3000
```

That brings up `sild-migrate` (once), `sild-standalone`, Postgres, Redis and the
inbox. Change `STORAGE_SIGNING_KEY` and the bootstrap password before you put it
anywhere real.

Two containers serve the product — the Go backend and the inbox. The inbox is a
Next app whose only server-side job is proxying `/v1` so the admin session cookie
stays same-origin; folding it into the Go binary is possible but unproven, and it
is not what stands between you and a deployment.

### Add a replica

```sh
docker compose -f deploy/standalone/compose.yaml up -d --scale sild=3
```

Nothing else changes: the outbox claim and the archive lease already make the
in-process jobs safe on every replica (ARCHITECTURE §4), and Redis already carries
realtime between them. Each replica takes its own host port from the published
range (8080, 8081, …) — put your own load balancer in front of them. Stay on one
Docker host: the compose volume is shared only within it, and
`STORAGE_LOCAL_SHARED=true` is an assertion the backend takes at its word rather
than something it can verify. Spreading replicas across hosts needs the
object-storage backend, which is not wired yet (see the note under Tiers).

## Kubernetes

`deploy/k8s/standalone/` is one Deployment and its Service. `deploy/k8s/` is the
split topology. Both read the same ConfigMap and Secret; see
[`deploy/k8s/standalone/README.md`](../deploy/k8s/standalone/README.md).

## Cloud Run

Cloud Run fits standalone well — one listener, one container, `$PORT` injected.
Not yet, though: it needs object storage, and that backend is a stub (see Tiers),
so this section is the shape to deploy once it lands rather than a working recipe.
The caveats below come from Cloud Run, not from Sild.

```sh
gcloud run deploy sild \
  --image REGION-docker.pkg.dev/PROJECT/sild/backend \
  --command /usr/local/bin/sild-standalone \
  --add-cloudsql-instances PROJECT:REGION:INSTANCE \
  --vpc-connector sild-connector \
  --timeout 3600 \
  --session-affinity \
  --cpu-boost \
  --set-env-vars "SILD_ENV=production,DB_DRIVER=postgres,SILD_BROKER=redis,STORAGE_BACKEND=gcs,STORAGE_BUCKET=sild-uploads,SILD_JOBS=" \
  --set-secrets "DB_DSN=sild-db-dsn:latest,SILD_REDIS_URL=sild-redis-url:latest"
```

- **`$PORT`.** Cloud Run injects it and it wins over `SILD_HTTP_ADDR`. REST and
  `/v1/ws` share that one listener; `SILD_WS_ADDR` is ignored by this binary.
- **`--timeout 3600` and `--session-affinity`** for WebSockets. Without the
  timeout, Cloud Run cuts long-lived connections at the request deadline. Clients
  reconnect and catch up (§5.4), but affinity keeps a reconnect on the instance
  that already holds the subscription state.
- **`STORAGE_BACKEND=gcs` is mandatory at any instance count** — the local backend
  writes to the container filesystem, which does not survive a revision — and it is
  the piece that is not implemented yet.
- **Cloud SQL** through the connector socket DSN
  (`host=/cloudsql/PROJECT:REGION:INSTANCE user=… dbname=…`), and **Memorystore**
  for Redis via a VPC connector.

### Jobs on Cloud Run

Serving revisions above run with `SILD_JOBS=""` — a pure serving replica. Run the
jobs as a separate Cloud Run **Job** on a Cloud Scheduler trigger, using
`sild-worker --once`, which runs each selected job a single time and exits:

```sh
gcloud run jobs create sild-jobs \
  --image REGION-docker.pkg.dev/PROJECT/sild/backend \
  --command /usr/local/bin/sild-worker \
  --args=--once \
  --set-cloudsql-instances PROJECT:REGION:INSTANCE \
  --set-env-vars "SILD_ENV=production,DB_DRIVER=postgres,SILD_BROKER=redis,STORAGE_BACKEND=gcs" \
  --set-secrets "DB_DSN=sild-db-dsn:latest,SILD_REDIS_URL=sild-redis-url:latest"
```

Note what is *not* there: no `--jobs webhook,archive`. `gcloud` splits both `--args`
and `--set-env-vars` on commas, so a comma-containing value silently becomes extra
arguments — `--args --once,--jobs,webhook,archive` would set `--jobs=webhook` and
drop `archive` on the floor. The default job set is already `webhook,archive`, so
passing nothing is both shorter and correct. If you do need a subset, use gcloud's
alternate delimiter: `--set-env-vars "^##^SILD_JOBS=webhook,archive"`.

`--once` is what makes it a Job rather than a service: `sild-standalone` and a bare
`sild-worker` both loop forever, so as a Cloud Run Job they would run until the
task timeout and never report success. Overlapping executions are still safe — the
outbox claim and the archive lease are what make that true — but a Job has to
finish.

This is the recommended shape: it keeps CPU-always-allocated cost off the serving
revisions. The alternative — `--min-instances=1` with CPU always allocated and
`SILD_JOBS=webhook,archive` on the serving revision — also works, and is correct
at any instance count for the same reason. What does *not* work is in-process jobs
on a scale-to-zero revision: with no CPU allocated between requests, the tickers do
not fire.

### Migrations on Cloud Run

A Cloud Run Job running `sild-migrate`, executed before you route traffic to the
new revision. Never in the serving container's startup path.

### Inbound email on Cloud Run

Cloud Run has no raw TCP ingress, so the SMTP daemon cannot receive there. Point
your email provider's inbound webhook at `POST /v1/email/inbound` instead. This is
why `SILD_SMTP_INGEST` is off by default — set it to `true` only where you control
port 25 and your MX records point at the process.

## Creating the first tenant

There is no HTTP tenant-provisioning API: it would need a cross-tenant root
principal that does not exist, and every deployment target can run a command.
`sild-admin` ships in the same image.

```sh
sild-admin tenant create --name "Acme" --admin-email you@acme.com --admin-name "Your Name"
```

It prints the three things a new customer needs — tenant id, the API key (once),
and the forwarding address — and prompts for the owner's password on a terminal.
Piping it in works too: `--password -` reads one line from stdin. A password is
never accepted as an argument, where it would land in shell history and in the
process list.

```sh
sild-admin tenant list
sild-admin agent invite       --tenant ID --email a@acme.com --role agent
sild-admin agent set-password --tenant ID --email a@acme.com
sild-admin agent peer-access  --tenant ID --email a@acme.com --on
sild-admin apikey create --tenant ID --label ci
sild-admin apikey revoke --tenant ID --id KEY_ID
```

On Kubernetes: `kubectl exec deploy/sild-standalone -- sild-admin tenant create …`.
On Cloud Run: a Cloud Run Job against the same image. This is the cleaner path
everywhere.

Where running a command is genuinely awkward, `sild-standalone` will bootstrap the
first tenant from env — but **only while the tenant table is empty**, so restarts
and extra replicas are no-ops and it can never surprise an existing deployment:

```
SILD_BOOTSTRAP_TENANT=Acme
SILD_BOOTSTRAP_ADMIN_EMAIL=you@acme.com
SILD_BOOTSTRAP_ADMIN_NAME=Your Name
SILD_BOOTSTRAP_ADMIN_PASSWORD=…
```

It logs what it created, including the API key once. Clear the password variable
once you have signed in. Bootstrap is the convenience; `sild-admin` is the
mechanism.

## Env matrix

| Var | Dev | One container | Cloud Run | Split |
|---|---|---|---|---|
| `SILD_ENV` | `development` | `production` | `production` | `production` |
| `DB_DRIVER` / `DB_DSN` | `sqlite` | `postgres` | `postgres` (socket DSN) | `postgres` |
| `SILD_BROKER` | `memory` | `redis` | `redis` | `redis` |
| `SILD_REDIS_URL` | — | `redis://redis:6379` | Memorystore | in-cluster |
| `SILD_HTTP_ADDR` | `:8080` | `:8080` | ignored (`$PORT`) | `:8080` |
| `SILD_WS_ADDR` | — | ignored | ignored | `:8081` (`sild-ws`) |
| `SILD_JOBS` | — | `webhook,archive` | `""` + a Job | — (`sild-worker`) |
| `SILD_SMTP_INGEST` | — | `false` | `false` | — (`sild-mail`) |
| `STORAGE_BACKEND` | `local` | `local` (shared volume) | `gcs` (not wired) | `local` (RWX) |
| `STORAGE_SIGNING_KEY` | — | required with `local` | — | required with `local` |
| `STORAGE_LOCAL_SHARED` | — | `true` (one volume) | — | `true` |

Full list with defaults: [`backend/.env.example`](../backend/.env.example).

## What stays per-replica

The rate limiter (`middleware/ratelimit.go`) counts in-process, so N replicas
allow N× the configured rate. It blunts brute force; it was never a fleet-wide
quota, and standalone does not change that.
