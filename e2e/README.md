# @sild/e2e — end-to-end behavior tests

Playwright tests that drive the **inbox** and the **drop-in widget** from a real
user's perspective against a live, zero-infra backend. The suite's backbone is
the **cross-surface** flow: a message sent in the widget appears live in the
inbox, and the agent's reply appears live back in the widget — proving the whole
product works together after any change.

## What it does automatically

`playwright.config.ts` boots everything for you (`webServer`):

- **Backend** — `go run ./cmd/sild-dev` with a **fresh temp SQLite DB per run**
  (`DB_DSN`), so the first-run seed always fires: tenant, admin
  `admin@sild.local` / `password123`, and 5 sample conversations. In-memory
  broker, in-process worker/SMTP — no Postgres/Redis/Docker.
- **Inbox** — `next dev` on :3000, proxying `/v1/*` and `/widget.js` to :8080.
- **Widget bundle** — rebuilt in `global-setup` so the embedded `/widget.js` is
  current.

Test isolation is by **unique ids** (each test creates its own conversation),
not by resetting the shared DB.

## Test data is created by driving the app

Conversations are created by **driving the real widget** (`support/appdata.ts`):
open the launcher, start a conversation, type, send — exactly as a visitor
would. No backend requests are forged. The only non-UI actions are two states
the inbox has no control for (`support/backdoor.ts`):

- **closing an assignment** — the "Close conversation" button closes the
  conversation, not its assignment, and nothing in the inbox closes an
  assignment; the "Closed" queue filters on assignment status. (Likely a product
  gap worth a look.)
- **creating a webhook** — the Webhooks tab only lists / toggles / deletes.

The inbox/widget do not emit typing or read-receipt events, so those aren't
app-reachable and are intentionally not tested.

## Database: SQLite and Postgres

The suite runs against the zero-infra default (SQLite) and the primary
production target (Postgres, with full trigram search). Point it at Postgres:

```bash
SILD_E2E_DB_DRIVER=postgres \
SILD_E2E_DB_DSN="host=localhost port=5432 user=sild password=sild dbname=sild sslmode=disable" \
npm test
```

CI runs both via a matrix. `sild-dev` auto-migrates on start, so no migrate step
is needed.

## Coverage

Opt-in browser-code coverage (inbox client JS, source-mapped back to
`inbox/src`) via monocart:

```bash
npm run test:coverage      # E2E_COVERAGE=1, writes coverage/index.html
npm run coverage:report
```

This covers code executed in the inbox page. The Go backend has its own
coverage (`cd backend && go test -cover ./...`).

## Requirements

- **Node 24** (`nvm use` — see `../inbox/.nvmrc`).
- **Go 1.25** (the version in `../backend/go.mod`). With `GOTOOLCHAIN=auto`
  (the default) a newer toolchain is fetched automatically.
- Deps installed in `../web` and `../inbox` (`npm ci` in each) — needed to build
  the widget and run the inbox.

## Run

```bash
npm install            # first time
npx playwright install chromium
npm test               # all projects, headless
npm run test:ui        # watch interactively
npm run test:cross     # just the widget↔inbox realtime flows
npm run test:inbox     # just the inbox
npm run test:widget    # just the widget
npm run report         # open the last HTML report
```

Do **not** run against `make dev` (Postgres/Redis + persistent DB). The config
always launches its own `sild-dev` with a throwaway DB. If `go run` is
unavailable locally, point the suite at a prebuilt binary:
`SILD_E2E_BACKEND_CMD='./bin/sild-dev' npm test`.

## Layout

- `playwright.config.ts` — projects (`setup`, `inbox`, `widget`, `cross`) + servers.
- `setup/admin.setup.ts` — logs the admin in once → `.auth/agent.json`.
- `fixtures/index.ts` — merged `test` with the `app` factory (widget-driven data) + coverage.
- `support/` — `appdata.ts` (widget-driven creation), `backdoor.ts` (the 2 no-UI
  actions), `inbox.ts`, `widget.ts` (shadow-DOM-piercing locators), `realtime.ts`.
- `specs/inbox` (auth, list, search, view, settings-*), `specs/widget`,
  `specs/cross` (realtime-flow / -guest / -edge / -scenarios, multi-conversation,
  appearance-parity).

The Go backend suite runs in CI too (`.github/workflows/e2e.yml`, `backend-tests`
job); run it locally with `cd backend && go test ./...`.
