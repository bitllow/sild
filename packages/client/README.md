# @sild/client

The Sild REST and realtime plumbing shared by every JavaScript client: the web
widget (`web/`), the inbox (`inbox/`) and the triage bridge (`bridge/`).

- `wire.ts`: the JSON shapes the backend renders (`backend/internal/views`).
- `http.ts`: `createHttp` with bearer, cookie or anonymous credentials, and `ApiError`.
- `list.ts`: the list contract (`pages`, `collectAll`) and `?since=` catch-up (spec §4.0, §5.4).
- `realtime.ts`: `connect` over centrifuge (WebSocket or SSE), envelope parsing.
- `resources.ts`: typed endpoint functions over an `Http`, no app state.

It ships TypeScript source, not a build. Consumers depend on it with
`"@sild/client": "file:../packages/client"`: the widget bundles it with esbuild,
the inbox lists it in Next's `transpilePackages`, and the bridge runs the `.ts`
directly on Node 24. Only erasable TypeScript syntax is allowed.

`src` imports nothing by bare specifier, because a linked package resolves
imports from its own path, not the consumer's. Pass centrifuge in:
`connect({ Centrifuge, ... })`.

Tests: `node --test 'src/**/*.test.ts'` here, and in the bridge's `npm test`,
whose `tsc` also typechecks them.
