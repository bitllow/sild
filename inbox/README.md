# Sild — Support inbox (Phase 2)

The agent-facing admin UI: the **assignment queue**, not a conversation type
(spec §8). Next.js (App Router) · TypeScript (strict) · MobX.

A pixel-faithful build of the *Sild Surfaces* design — the design system tokens
and components are ported verbatim from `docs/Inbox and drop-in UI design/` —
**wired to the live Go backend** (`/v1/admin/*`, §4.3) over a cookie session.

## Connect to the backend

The inbox calls `/v1/*`, which Next proxies to the Go backend (default
`http://localhost:8080`, override with `SILD_API_URL`). Proxying keeps the
browser same-origin so the HttpOnly admin session cookie is first-party — no
CORS setup needed.

```bash
# 1. backend (seeds a dev tenant + admin + 5 sample support requests on first run)
cd backend && make dev          # REST + WS on :8080  (admin@sild.local / password123)

# 2. inbox
cd inbox && nvm use 24 && npm install && npm run dev   # http://localhost:3000
```

Sign in with **email/password** (`admin@sild.local` / `password123`) or Google.
Everything is real: the queue, transcripts (incl. internal notes), claim, close,
send, API keys, webhooks, and team roles hit the backend; the inbox updates
**live over the Centrifuge WebSocket** (§5) and the search bar runs the
mixed-token admin search (§4.3).

### Realtime (§5)

The browser opens a Centrifuge connection straight to the node
(`NEXT_PUBLIC_SILD_WS_URL`, default `ws://localhost:8080/v1/ws`) — a cross-origin
WebSocket the cookie can't ride, so it authenticates with a short-lived **agent
token** minted at `GET /v1/admin/realtime/token` (cookie-authed). Channels are
attached server-side: an agent connection subscribes to `conv:<id>` and
`conv:<id>:internal` for every conversation in the queue, plus `agents:<tenant>`
for new requests. Messages, assignment changes, conversation close, and typing
arrive live; on (re)connect the client runs a REST catch-up (§5.4). New support
requests trigger a resubscribe so their conv channel is covered.

### Search (§4.3)

The list's search bar calls `GET /v1/admin/search` (debounced) — mixed tokens:
`status:`/`assignee:me`/`role:`/`channel:`/`meta.*:` filters plus free keywords
(partial trigram match on message bodies + member metadata). Results replace the
queue list with the matched-message snippet as the preview.

## Screens

- **Login** — Google OIDC sign-in (mocked; flips session state).
- **Inbox** — conversation list with `You / Unassigned / Closed / All` filters,
  search bar, presence, unread badges, queued pills.
- **Conversation view** — transcript (in/out/internal-note/system bubbles, read
  receipts, email channel), composer with a **Reply / Internal note** toggle,
  Claim + Close conversation, member detail panel.
- **Settings** — API keys (issue-once dialog, revoke), Webhooks (events, toggle,
  delete), Team (platform-role select).

## Scripts

Requires **Node 18+** (the repo's default `nvm` alias is ancient — `nvm use 24`).

```bash
npm run dev         # http://localhost:3000
npm run build       # production build
npm run typecheck   # tsc --noEmit
npm run lint
```

## Layout

```
src/
├── app/                  # Next.js App Router (layout, page, globals.css)
├── api/                  # typed admin API client (client.ts + admin.ts)
├── components/
│   ├── ds/               # design-system primitives (Avatar, Button, Dialog, …)
│   └── inbox/            # inbox screens (Shell, ConversationList/View, Settings, …)
├── store/                # MobX RootStore + API↔UI mappers + provider
└── styles/               # design tokens (verbatim) + ported component CSS
```

## State & data

`RootStore` (MobX, `makeAutoObservable`) bootstraps a session, loads the
assignment queue, fetches each conversation + its messages, then opens the
realtime connection. Actions (`claim`, `closeConv`, `sendMessage`, settings CRUD)
call the API; realtime events patch state in place. `store/map.ts` converts API
shapes (`internal/views`) to the UI types. A 30s reconcile + on-connect REST
catch-up backstop the socket (§5.4).

## Known simplifications

- **Presence dots** and **per-conversation unread on first load** aren't exposed
  over the admin REST surface, so presence is omitted and unread is derived from
  live events only (0 on load).
- **Google sign-in** redirects to the real OIDC endpoint; its callback returns
  JSON rather than redirecting back to the SPA, so password login is the
  fully-wired path in dev.

## Backend additions made for this UI

To make the design fully functional, a few endpoints were added to the Go
backend: `GET /v1/admin/team`, `PATCH /v1/admin/webhooks/:id` (active toggle),
`PATCH /v1/admin/team/:id` (role), and `GET /v1/admin/realtime/token` (agent WS
token). The Centrifuge node gained an **agent connection path** (`typ:"agent"`
tokens → queue-derived server-side subscriptions incl. the internal channel),
and `cmd/sild-dev` seeds 5 sample support requests on first run.
