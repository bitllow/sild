# Sild — Web drop-in (Phase 3)

The embeddable end-user chat widget (spec §9). A **self-contained script-tag
bundle** that drops onto any site (WordPress, arbitrary HTML) and renders inside
a **custom element + shadow DOM**, so it can't clash with the host page's styles.

- **Renderer:** Preact (~4KB) inside `<sild-widget>`'s shadow root — *not* React,
  so the bundle stays light (~30KB gzipped vs ~90KB+ for a React build).
- **Core:** a framework-agnostic `SildClient` (`src/core/`) owns auth, the §4.2
  REST calls, and the §5 realtime client over **SSE** (proxy-friendly, no WS
  upgrade). It owns no DOM, so a future `@sild/react` package can render the same
  core with React as a *peer* dependency (the host SPA already ships React).

## Usage

```html
<script src="https://chat.sild.io/widget.js"></script>
<script>
  Sild.init({
    // mints a user JWT via the host backend (which holds the API key);
    // the browser never sees the API key. A guest is the same call with a
    // host-generated id.
    tokenProvider: () => fetch('/sild/token').then(r => r.json()).then(d => d.token),
    conversationId: 'c_abc' // optional; omit for the list / new-request view.
                            // REQUIRED for guest tokens (§9 opens to one thread).
  });
</script>
```

`baseUrl` defaults to the script's own origin; pass it explicitly for local dev.

## Build

```bash
nvm use 24 && npm install
npm run build      # → dist/widget.js  (esbuild, IIFE, minified)
npm run typecheck
```

## How it's served + embedded

`widget.js` is **embedded into the Go binary** (`go:embed`) so any sild binary
serves it self-contained — no static host, no filesystem-path assumptions:

```
npm run build  →  dist/widget.js
               →  copied to backend/internal/webasset/widget.js   (build.mjs)
go build       →  go:embed bakes it into the binary                (webasset/asset.go)
GET /widget.js →  served from the embedded bytes                   (api/handler.go)
```

From the repo root, **`make build`** (and `make dev`) runs the web build *first*,
so the binary always embeds the current bundle. A committed **placeholder**
`backend/internal/webasset/widget.js` keeps a plain `go build` working on a fresh
clone (it serves a stub with a "run the build" console warning until you build
the real bundle); `web/dist` is gitignored. Front `/widget.js` with a CDN in prod.

The widget runs on the *customer's* origin, so the backend enables **CORS** on the
public/user-JWT routes + SSE (`internal/middleware/cors.go`). The admin inbox is
unrelated — it reaches the API same-origin via its own proxy.

## Try it locally

```bash
cd backend && make dev                       # serves /widget.js, /sild-demo, SSE
cd web && npm run build                       # builds dist/widget.js
open http://localhost:8080/sild-demo          # faux "Acme Rides" host page + widget
```

`make dev` also exposes a dev-only `GET /v1/dev/widget-token` that stands in for
the host backend's token endpoint (mints a guest JWT in the dev tenant). The
full loop — open request → it lands in the inbox queue (live) → agent replies →
the widget receives it over SSE — is verified end-to-end.

## Layout

```
src/
├── core/        # SildClient + types — framework-agnostic (REST §4.2, SSE §5)
├── widget/      # Preact UI: App (launcher/home/thread/composer) + scoped CSS
└── index.tsx    # <sild-widget> custom element + window.Sild.init
public/demo.html # faux host page for local validation
build.mjs        # esbuild → dist/widget.js
```

## Follow-ups

- **`@sild/react`** — thin React wrapper over `src/core` (React as peerDep).
- **Attachments** — `POST /v1/uploads` signed PUT then send `object_key`
  (core has the REST shape; UI affordance not wired yet).
- **Guest reload-persistence** is the host's job (remember the id, re-mint) — the
  demo persists a guest id in `localStorage`.
