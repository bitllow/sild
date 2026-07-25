# Uniform conversations API — plan

Base: `428dcf7` (includes peer conversations `#1` and the Android SDK `#2`).

Decisions taken: **whole surface**, **hard cutover** (no aliases), peer access
enforced as a **scope filter in the authorizer** rather than a per-route guard.

> **Pre-release compatibility assumption.** No production or external consumers
> exist. `/v1` may be replaced in place — no aliases, no deprecation period, no
> `/v2`. All repository clients, SDKs, docs and tests migrate atomically before
> the first release.

This assumption does real work throughout: it is why the plan renames `before` →
`cursor` and `after` → `since` outright, changes bare arrays to envelopes, moves
every settings path, and introduces cursor format `v:1` with no back-compat
branch. It also sets the hard deadline — **every one of these changes must land
before the first external consumer exists**, because each becomes a breaking
change the moment one does. Sequencing below is ordered accordingly: the
contract-shaping work comes first, the conveniences last.

## Context

The REST surface is organized by **consumer**, not by **data model**. The same
rows are reachable through different URLs depending on who is asking, so every
new surface (peer inbox, Android SDK, contact history) has added another endpoint
returning a shape we already had.

Five routes are all "list conversations":

| route | what it actually is |
|---|---|
| `GET /v1/me/conversations` | `participant = <caller>` |
| `GET /v1/admin/assignments` | `kind=support`, assignment-shaped, keyset-paginated |
| `GET /v1/admin/contacts/conversations` | `participant = <external_user_id>` |
| `GET /v1/admin/peer-conversations` | `kind=peer`, keyset-paginated |
| `GET /v1/admin/search?q=&peer=` | any of the above + free text |

Three routes are all "open a support request" (`POST /v1/me/support-requests`,
`POST /v1/admin/support-requests`, `POST /v1/conversations` with
`open_assignment:true`). Three are "read the active brand" (`/v1/public/brand`,
`/v1/me/brand`, `/v1/admin/brands`). Two are "send a message"
(`POST /v1/conversations/:id/messages`,
`POST /v1/admin/peer-conversations/:id/messages`).

The cost is not just duplication. The support/peer split is a **security
boundary** that is currently restated in each query builder —
`search/common.go:buildFilters` carries a comment explaining that without its
`c.kind = 'support'` clause a non-peer-access agent could recover peer content
through the shared search, even though the peer list and message endpoints are
gated. Every new list path has to remember that rule independently.

Pagination has fragmented the same way: four different conventions across six
list endpoints (see "One pagination system" below).

**Outcome:** one resource per data model; the credential decides *which subset*
of that resource you see, never *which URL* you call. And one way to page
through any collection.

## Principle to record in `ARCHITECTURE.md`

Add to §5 Cross-cutting concerns, next to Tenancy:

> **APIs are data-model-oriented, not consumer-oriented.** A path names *what* is
> being addressed. The credential names *who* is asking, and therefore what
> subset of that model they may see. Two consumers reading the same model call
> the same URL — the inbox, the web drop-in, the native SDK and a host backend
> all `GET /v1/conversations`; they differ only in the scope the principal
> resolves to. The sole exception is **authentication**: obtaining a credential
> genuinely differs per consumer, so `/v1/admin/auth/*` stays consumer-shaped.
> A route that exists because "the inbox needs it" and not because it addresses
> a distinct model is a bug.
>
> A resource may be a **read model** rather than a table — `contacts` projects
> over `conversation_members` — provided its projection rules (identity,
> precedence, ordering, what counts) are written down. What is forbidden is a
> resource that exists because a screen wanted it.
>
> **Collections page one way.** Every list response is
> `{ items, next_cursor, has_more }` and every list request takes `?limit=` and
> `?cursor=`. The cursor is opaque to clients. A resource does not get to invent
> its own paging dialect.
>
> **Authorization is attribute-based and lives in one package.** Decisions depend
> on principal kind, role, `peer_access`, membership, assignment existence,
> conversation kind and status. `policy.Authorize` decides a named action against
> a resource; `policy.Scope` produces the ceiling for a collection. Handlers do
> not read `Role` or `PeerAccess`; repositories take a scope only the policy layer
> can construct; **REST and realtime consume the same decisions**, because two
> implementations of one policy is how a socket leaks what an endpoint refuses.

Also update `docs/chat-platform-spec.md` §4, which is currently structured as
§4.1 Integration / §4.2 User / §4.3 Admin — the spec itself is written
consumer-first. Restructure to one subsection per resource, with an auth matrix
per route.

---

## One pagination system

### What exists today

Six list endpoints, four conventions, and no two agree:

| endpoint | request | response | cursor is |
|---|---|---|---|
| `GET /v1/admin/assignments` | `?limit=&cursor=` | `{items, next_cursor, has_more, …counts}` | base64(JSON `{v,id}`) keyset |
| `GET /v1/admin/peer-conversations` | `?limit=&cursor=` | `{conversations, next_cursor, has_more}` | same, reuses `decodeQueueCursor` |
| `GET /v1/admin/search` | `?limit=&before=` | `{conversations}` | raw conversation id |
| `GET /v1/conversations/:id/messages` | `?limit=&before=` | `{messages, has_more}` | raw message id |
| `GET /v1/me/conversations` | — | bare `[…]` | none |
| `GET /v1/admin/contacts/conversations` | — | `{conversations}` | none |

Concrete consequences:

- **Search cannot be paged at all.** It accepts `?before=` but returns neither
  `next_cursor` nor `has_more`, so a client has no way to know a second page
  exists or to ask for it (`search/backend.go:19`, `search/common.go:138`).
- **Three envelope keys** for the same kind of payload — `items`,
  `conversations`, `messages` — plus bare arrays from `/me/conversations`,
  `/admin/team`, `/admin/api-keys`, `/admin/webhooks`, `/admin/webhooks/:id/deliveries`.
  A client cannot write one list parser.
- **Two cursor encodings.** The queue's opaque base64 keyset vs. search's and
  messages' raw ids. The raw-id form leaks the sort key into the URL and locks
  the endpoint to id-ordering forever.
- `has_more` is present on the queue and on `messages?before=`, absent on
  `messages?after=`, absent on search, meaningless on the unpaginated three.

Unifying the five list routes into `GET /v1/conversations` forces this to be
resolved anyway: the merge combines one keyset-paginated query, one id-offset
query and two unpaginated ones.

### The contract

**Request** — every collection endpoint:

```
?limit=<1..100>     default 30 (messages: 50); clamped server-side, never errors
?cursor=<opaque>    omit for the first page; 400 on malformed
?order=asc|desc     where the resource has a meaningful direction
```

**Response** — every collection endpoint, without exception:

```json
{ "items": [ … ], "next_cursor": "<opaque>|null", "has_more": true }
```

`next_cursor` is `null` exactly when `has_more` is `false`. Resource-specific
extras sit beside the envelope, never inside it — the support queue's counters
stay as `counts: { open, you, unassigned, closed }` at the top level.

**The cursor is opaque, versioned, and fully bound.** base64url(JSON). A cursor
carries everything that must not change between pages, so a mismatched replay is
a 400 instead of a wrong answer under a 200:

```json
{ "v": 1,            // cursor format version
  "r": "conversations", // resource — rejects cross-resource replay
  "s": "last_activity", // sort key
  "o": "desc",          // direction
  "f": "9f2c…",         // fingerprint of the filter set
  "k": "2026-07-25T…",  // position value (absent for id-only sorts)
  "id": "c_01J…" }      // position tiebreak
```

`f` is a hash over **every normalized query parameter except the paging ones
(`limit`, `cursor`), plus every path parameter** — computed generically from the
request, never from a hand-maintained field list. A hand-maintained list is
wrong the first time someone adds a filter and forgets to add it: `role=` on the
peer list is exactly that case: present in the route, easy to miss in a list.

Binding path parameters matters as much as query ones. A message cursor minted
for conversation A must not be accepted for conversation B — the ids are
comparable ULIDs, so the scan would succeed and return a wrong, plausible page.
Including `:id` in `f` makes that a 400.

Changing any filter mid-scan makes the old position meaningless — the row it
named may not even be in the new result set — so the client restarts rather than
receiving a silently truncated page.

Not HMAC-signed: every value ends up as a bound SQL parameter, so a forged cursor
can misposition a scan the caller is already authorized for, nothing more.
Versioning and binding are the architectural requirements; signing can be added
later without a wire change because `v` exists.

Clients round-trip it verbatim. SDK and widget types model it as a **string**,
never a parsed object.

**Keyset, not offset — but keyset is not automatically stable.** Keyset paging is
stable under concurrent writes only when the ordering key is **immutable**. The
default conversation sort is not: `last_activity` is
`COALESCE(last_message_at, created_at)`, and `TouchLastMessage` rewrites it on
every inbound message. A conversation on page 3 that receives a message jumps to
the front — the reader either sees it twice or misses a row that shifted past
the cursor. Sorting by `created` or by id is stable; sorting by `last_activity`
or `waiting_since` is not.

This is inherent to a live queue, not a bug to engineer away, so the plan picks
**live-view semantics** and states them:

- Ordering by a mutable key is best-effort. Duplicates and gaps are possible
  across page boundaries while the underlying rows are changing.
- Clients **deduplicate by conversation id** when appending a page. The inbox
  already keys rows by id, so this is a documented invariant rather than new work.
- A realtime event that reorders the list invalidates the scan: the inbox
  refreshes page one rather than continuing to append. Again, close to what
  `syncQueue` already does on scope changes.
- Sorts on immutable keys (`created`, id) are exact, and the archived-message
  path is exact by construction.

Snapshot semantics — a watermark in the first cursor, later pages reading
as-of that instant — would make mutable-key paging exact, at the cost of showing
operators a stale queue. Rejected for the queue, and noted here so the choice is
visible rather than accidental. `?page=`/`?offset=` remain excluded everywhere.

### Catch-up is not pagination

`GET /v1/conversations/:id/messages?after=<id>` is the realtime reconnect
primitive (spec §5.4): "give me everything since X", oldest-first. It is a
**sync read**, not a page, and conflating the two is what produced the current
asymmetry — `after` returns `{messages}` with no `has_more` while `before`
returns `{messages, has_more}` from the same handler (`api/shared.go:144`).

It is also **silently truncating today.** `messageRepo.ListAfter` hard-caps at
`Limit(500)` (`store/gormstore/message.go:71`) and the handler reports no
`has_more`, so a client that missed more than 500 messages gets a short answer
that looks complete and permanently loses the remainder. Fixing this is part of
the work, not incidental to it.

- `?cursor=` → pagination, newest-first, bounded by `limit`, returns the envelope.
- `?since=<message_id>` → catch-up, oldest-first, bounded by `limit`, returns
  `{ items, next_cursor: null, has_more }`. Renamed from `after` so the two are
  not mistaken for a matched pair. `next_cursor` is always `null` here — the
  envelope stays uniform so clients keep one list parser, and continuation uses
  `since=<last id>` rather than a cursor.

**Normative continuation rule for `since=`:** there is no cursor, because the
message id *is* the position. While `has_more` is true the client re-issues with
`since=<id of the last item received>`. State this in the spec §5.4 text and in
each SDK's reconnect path — a client that re-sends the original `since` loops on
the same page forever. The acceptance test seeds `limit + 1` messages and asserts
that a two-call catch-up returns each message exactly once.

`?before=` disappears from both messages and search, replaced by `?cursor=`.

### Archived history pages through the same contract

`GET /v1/conversations/:id/messages` has a second implementation that never
touches the database: when a tombstone exists, `ArchivedMessages` reads the whole
conversation back from the archive sink and the handler returns it with
`has_more: false` hardcoded (`api/shared.go:141`, `domain/archive_read.go:45`).
`gormstore.Paginate` cannot see this path — it is JSON in GCS/S3, not SQL — so
"every collection, no exceptions" is false unless it is handled explicitly.

The sink payload is an ordered `[]map[string]any` of ULID-keyed messages already
filtered for visibility, so the same cursor works in memory:

- `domain.ArchivedMessages` gains the same `store.PageParams` and returns
  `store.Page[map[string]any]`, slicing on `id` with the identical
  keyset/`limit+1`/mint logic — factored into a `store.SlicePage` helper shared
  with `Paginate` so the two cannot drift.
- `?since=` works the same way against the slice.
- Cursors are interchangeable across the two implementations: a conversation
  archived *between* two page requests must continue from the hot cursor without
  the client noticing. This is the one case where the framework's opacity earns
  its keep, and it needs a test that archives mid-pagination.

Cost, unchanged from today: each page re-reads the whole blob. Acceptable — the
comment at `api/shared.go:139` notes archived reads are rare (the inbox only
touches open conversations) — but it belongs in the code, and a per-conversation
sink read cap belongs in the follow-up if archive reads ever become hot.

### Every collection, no exceptions

| endpoint | today | after | default limit |
|---|---|---|---|
| conversations list (all five merged sources) | mixed | `?limit=&cursor=` | 30 |
| conversation search (`?q=`) | unusable | same cursor as the unfiltered list | 30 |
| contact history (`?participant=`) | unpaginated | paginated | 30 |
| user's own conversations | unpaginated bare array | paginated | 30 |
| contacts (`GET /v1/contacts`) | new | paginated | 30 |
| messages | `before`/`after` | `cursor` / `since` | 50 |
| `/v1/team` | bare array | paginated | 100 |
| `/v1/api-keys` | bare array | paginated | 100 |
| `/v1/webhooks` | bare array | paginated | 100 |
| `/v1/webhooks/:id/deliveries` | bare array | paginated | 100 |

The settings collections are genuinely paginated — same cursor, same envelope,
same store code path — not envelope-shaped stubs. Their default limit is 100
rather than 30 so that in practice one page still covers a tenant's whole team or
key list, and the settings screens need no infinite scroll on day one. But the
mechanism is real: seed 150 API keys and the second page works, because nothing
about that path is special-cased.

These collections are keyed by ULID id alone. Since ULIDs are chronological and
unique (`ARCHITECTURE.md` §5 Identifiers), `id` is a complete keyset — no
timestamp, no tiebreak column. That is why the framework has to support an
id-only cursor, and why supporting it costs nothing.

### The framework

One set of types in `store`, used by every repo (Go 1.25, generics available):

```go
// store/page.go

// SortKey names the column a cursor is positioned on. It travels inside the
// cursor, together with the direction, so a cursor can never be replayed under
// an ordering it was not minted for — reversing direction mid-scan would return
// the rows BEFORE the position instead of after it, silently and with a 200.
type SortKey string

const (
    SortID           SortKey = "id"            // ULID — chronological AND unique: a complete keyset on its own
    SortLastActivity SortKey = "last_activity" // COALESCE(last_message_at, created_at)
    SortCreated      SortKey = "created"
    SortWaitingSince SortKey = "waiting_since"
)

type Order string // "asc" | "desc"

// Cursor is the keyset position of the last row returned. Value is nil for
// SortID, where the id alone positions the scan. Key and Order are carried so
// the parser can reject a replay under a different ordering.
type Cursor struct {
    Key   SortKey
    Order Order
    Value *time.Time
    ID    string
}

type PageParams struct {
    Limit  int
    Sort   SortKey
    Order  Order
    Cursor *Cursor // nil for the first page
}

type Page[T any] struct {
    Items      []T
    NextCursor *Cursor
    HasMore    bool
}
```

**One place keyset SQL is written**, in `gormstore`:

```go
// gormstore/page.go

// Paginate applies the keyset predicate, ordering and the limit+1 probe to any
// query, then splits the overflow row off into HasMore/NextCursor. exprs maps
// each sort key this table supports to its SQL expression; a PageParams naming
// a key the table does not support is a programming error, not a request error.
func Paginate[T any](
    q *gorm.DB,
    p store.PageParams,
    exprs map[store.SortKey]string,
    idExpr string,
    cursorOf func(*T) store.Cursor,
) (store.Page[T], error)
```

Every list repo calls it. For `api_keys` that is
`exprs{SortID: "api_keys.id"}`; for the conversation list,
`exprs{SortLastActivity: "COALESCE(conversations.last_message_at, conversations.created_at)", …}`.
The `limit+1` probe, the `(value, id) < (?, ?)` predicate, the `DESC/ASC`
flip and the cursor mint all live here once — today they are copy-pasted between
`assignmentRepo.ListQueue` and `conversationRepo.ListPeers`
(`backend/internal/store/gormstore/conversation.go:75`).

**One parser, one responder** at the HTTP edge:

- `apiutil.PageParams(c, defaults)` — reads `limit`/`cursor`/`sort`/`order`,
  clamps the limit to `[1,100]`, decodes the cursor, and writes a 400 if it is
  malformed **or** if its `Key` disagrees with the requested sort **or** its
  `Order` disagrees with the requested direction. Both halves matter: a cursor
  minted under `order=desc` and replayed with `order=asc` flips the comparison
  operator, so the query returns the rows *preceding* the position — a wrong
  answer with a 200 status, which is the worst failure mode a paginator has.
  Absorbs `atoiDefault` + `decodeQueueCursor` from `api/admin.go:232`.
- `httpx.Page(c, page)` — writes `{items, next_cursor, has_more}`, encoding the
  cursor as opaque base64url(JSON). Absorbs `encodeQueueCursor` (`admin.go:221`).

Every handler that currently hand-rolls a list response goes through these two:
`listAssignments`, `listPeerConversations`, `adminSearch`, `listMessages`,
`listMyConversations`, `listContactConversations`, `listTeam`, `listAPIKeys`,
`listWebhooks`, `listDeliveries`.

`store.QueueCursor`, `queueCursorDTO`, `encodeQueueCursor`, `decodeQueueCursor`
and the ad-hoc `before` handling in `search/common.go:138` are deleted.

**Client side**, mirroring the server: one `Page<T>` type and one `fetchPage`
helper in `inbox/src/api/client.ts`, plus a `collectAll(fetchPage)` used by the
settings screens that want the whole collection without building scroll UI.
Same two primitives in `web/src/core/client.ts` and `SildApi.kt`.

The test that this is a framework and not a convention: adding a new list
endpoint should require zero pagination code — a repo call to `Paginate`, an
`apiutil.PageParams` at the top of the handler, an `httpx.Page` at the bottom.

---

## The unified surface

### `conversations`

```
GET /v1/conversations
    ?kind=support|peer                 ?participant=me|<external_user_id>
    ?assignee=me|<actor_id>|none       ?status=open|closed
    ?assignment_status=queued|assigned|closed
    ?q=<mixed-token search bar string>
    ?sort=last_activity|created|waiting_since   ?order=asc|desc
    ?limit=  ?cursor=

    -- sort=waiting_since requires kind=support (400 otherwise): the key comes
    --   from assignments, which peer and participant-scoped rows do not have,
    --   and a keyset over a NULL-bearing key is undefined. The framework treats
    --   an unsupported sort key as a programming error, but here the CLIENT
    --   picks it, so this one is validated as a request error.
 → { items[], next_cursor, has_more, counts: { open, you, unassigned, closed } }
```

Replaces `GET /v1/me/conversations`, `/v1/admin/assignments`,
`/v1/admin/contacts/conversations`, `/v1/admin/peer-conversations`,
`/v1/admin/search`. `counts` is emitted only when the caller is an operator and
the query is the support queue (today's `listAssignments` behaviour).

**Two state machines, two filters.** `conversations.status` is `open|closed`
(terminal, no reopen); `assignments.status` is `queued → assigned → closed` with
`assigned → queued` legal (`store/models/enums.go:43,65`). They are not the same
axis and a single `?status=` cannot express the queue. `?status=` filters the
conversation; `?assignment_status=` filters the assignment and is only meaningful
alongside `kind=support`.

**"Closed" in the inbox means the *conversation* is closed.** This is the part
that is easy to get backwards, and getting it backwards silently changes every
scope. `QueueParams.ExcludeClosed` compiles to `c.status != 'closed'` — the
conversation, not the assignment — and the repo comment says so outright:
"closing an assignment is a separate, API-only state"
(`gormstore/conversation.go`, `ListQueue`). Three independent places agree:

- `deriveStatus` returns `"closed"` on `conv.status === "closed"` before it ever
  looks at the assignment (`inbox/src/store/map.ts:120`).
- `CountQueue` buckets a row into `Closed` on `conv_status`, and only counts
  `You`/`Unassigned` for conversations that are *not* closed.
- `RootStore.ts:319` sets `excludeClosed: true` unconditionally for Unassigned,
  and `!showClosed` for You and All.

So `?exclude_closed=true` maps to `?status=open` — the conversation axis — and
the assignment filter is carried across unchanged. Correct parity mapping:

| inbox scope | today | after |
|---|---|---|
| You | `?assignee=me&status=assigned&exclude_closed=true` | `?kind=support&assignee=me&assignment_status=assigned&status=open` |
| You, showing closed | `?assignee=me&status=assigned` | `?kind=support&assignee=me&assignment_status=assigned` |
| Unassigned | `?status=queued&exclude_closed=true` | `?kind=support&assignment_status=queued&status=open` |
| All | `?exclude_closed=true` | `?kind=support&status=open` |
| All, showing closed | *(no params)* | `?kind=support` |

**Every inbox queue request carries `kind=support`**, including the two that
otherwise have no filters. Omitting it would change the All tab's row set for a
`peer_access` operator — an unfiltered `GET /v1/conversations` admits peer rows —
which is exactly what the parity test forbids. It also gives `counts` a
well-defined trigger: the counters are emitted when the query is the support
queue, and "the query is the support queue" has to be expressible.

There is no "Closed" scope tab — `showClosed` is a toggle over the three scopes,
and `counts.closed` labels it. Mapping closed-ness onto the assignment state
machine, or inventing a fourth scope, silently changes every tab.

`assignment_status` is **single-valued**, mirroring `QueueParams.Status` exactly.
No scope needs a set, and a comma list would be untested surface area.

```
POST   /v1/conversations                    -- create; open_assignment implies kind
GET    /v1/conversations/:id
POST   /v1/conversations/:id/messages       -- absorbs the peer send path
GET    /v1/conversations/:id/messages       -- ?cursor= (page) | ?since= (catch-up)
POST   /v1/conversations/:id/read
POST   /v1/conversations/:id/typing
POST   /v1/conversations/:id/close
POST   /v1/conversations/:id/members
DELETE /v1/conversations/:id/members/:user_id
POST   /v1/conversations/:id/members/remap  -- API key only (guest claim)
POST   /v1/conversations/:id/assignments
```

`POST /v1/conversations` replaces both support-request routes.

**Creating a peer conversation stays API-key-only.** `kind` is derived from
`open_assignment` (`domain/conversation.go:37` — `KindPeer` unless
`OpenAssignment`), so merging the create routes naively would let a user JWT
omit `open_assignment` and mint a peer conversation. Today users and operators
can only reach `OpenSupportRequest`; unrestricted creation is behind the API-key
group (`api/handler.go:73,82`). Worse than a capability expansion: a peer
conversation is visible to every `peer_access` operator, so a user could inject
rows into the operator peer inbox.

Per-principal contract:

| principal | members | `open_assignment` | resulting `kind` |
|---|---|---|---|
| user JWT | forced to `[self]` | forced `true` | `support` |
| admin session | may name one `external_user_id` | forced `true` | `support` |
| API key | arbitrary | caller's choice | `support` or `peer` |

A non-key principal sending `open_assignment:false` gets **403**, not a silent
upgrade — the client asked for something it may not have. Extending peer creation
to other principals is a separate decision, not a side effect of this refactor.

`POST /v1/conversations/:id/messages` absorbs `postPeerMessage`. The implicit
join (add operator as participant + system join-note) fires when the principal is
an operator **and** the conversation is peer — a property of the data, not of the
URL the client chose.

### `assignments`

```
PATCH /v1/assignments/:id   { "assignee_actor_id": "me", "status": "closed" }
```

Replaces `POST /v1/admin/assignments/:id/claim` and `.../close`. The queue
listing moves to `GET /v1/conversations?kind=support`.

**Scope-preserving by default.** Today `claimAssignment` always claims for the
authenticated operator — `middleware.Get(c).AdminID`, never a body field
(`api/admin.go:291`, `domain.ClaimAssignment`). A `PATCH` that accepts an
arbitrary `assignee_actor_id` would silently introduce reassignment, letting a
plain agent move another operator's work. Per-field RBAC:

| field | value | who | maps to |
|---|---|---|---|
| `assignee_actor_id` | `"me"` | any operator with access to the conversation | today's claim |
| `assignee_actor_id` | `"<other id>"` | **not accepted — 400** | *(no equivalent today)* |
| `assignee_actor_id` | `null` | **not accepted — 400** | *(no equivalent today)* |
| `status` | `"closed"` | any operator with access | today's close |
| `status` | `"queued"` / `"assigned"` | **not accepted — 400** | *(no equivalent today)* |

Transitions, matching `domain.transition`:

- `queued|assigned → assigned` via `assignee_actor_id:"me"`; **409** if already
  closed (`ClaimAssignment` returns `ErrConflict`).
- `queued|assigned → closed` via `status:"closed"`; already-closed is a **no-op
  200**, preserving `CloseAssignment`'s idempotence.
- Both fields in one request: **400**. Claim-then-close is two calls; allowing
  the combination would need a transactional ordering rule for no gain.

Assigning to another operator and returning to the queue are both legal in the
state-machine enum but have no route today. They belong to a follow-up that also
specifies tenant-membership validation of the target actor — I have deliberately
left them out rather than smuggling them in behind a verb change.

### `contacts` (new resource)

```
GET /v1/contacts?q=&limit=&cursor=   → matching participants (people, not threads)
GET /v1/contacts/:external_user_id
```

Contacts and conversations are **both** searchable, as separate resources — a
contact search returns people, a conversation search returns threads. A given
contact's history is `GET /v1/conversations?participant=<external_user_id>`, so
there is no `/contacts/:external_user_id/conversations`.

**The path parameter is `external_user_id` everywhere, and stays that way.** Even
with the durable projection, the contact's identity on the wire is the host's own
user id — the projection's own row id is internal and never addressed. Switching
later to a contact ULID would be an API break, so the choice is made now rather
than drifting.

That id is host-supplied and opaque to the platform, so its URL contract needs
stating: non-empty UTF-8, at most 128 bytes, no `/`, no control characters, not
normalizing to empty. Clients percent-encode it (`encodeURIComponent`) —
`inbox/src/api/admin.ts` already does this on the query-parameter form, and the
path form is easier to get wrong.

**The validation has to happen at write time, not in the path handler.** Gin
unescapes the path before matching, so an id containing `/` sent as `%2F` never
reaches a handler that could reject it — it mis-routes or 404s first. So the
charset rule is enforced where ids enter the system: `POST /v1/tokens` (subject),
member add, and remap. Enforcing it only on read would be unreachable for exactly
the values it targets, which is the failure mode worth naming rather than
discovering.

Reuses the member-text machinery already built for search: `member_search_text`,
built from the tenant's `searchable_metadata_keys`. It does **not** reuse peer
search's raw-metadata fallback — see the disclosure rule below.

**`contacts` is a declared read model, not a table.** This is the one place the
"one resource per data model" principle needs qualifying rather than applying: a
contact has no model. It is a *projection* over `conversation_members`, where one
`external_user_id` has a membership row per conversation, each with its own ULID
and its own metadata snapshot from when it was created.

**Take the derived read model — and make archival stop deleting the rows it
reads.** The alternatives all work around one fact — `PurgeHot` deletes membership
rows, and the conversation row with them. A derived model loses contacts to
archival; a durable `contacts` table cannot be scoped safely as a single
aggregated row; a mirror table duplicates `conversation_members` permanently.
That deletion is ours to change, so change it.

**Archival retains structure, purges bulk.** `PurgeHot` keeps `conversations` and
`conversation_members`, marking the conversation `archived_at`, and deletes what
is actually large: messages, attachments, read receipts, assignments, email
threads. The volume archival exists to reclaim is in messages — a conversation is
one row and a handful of member rows, unbounded message history is not — so
retaining the small part costs close to nothing and removes the entire
duplication problem.

Contacts is then a **pure derived read model** over
`conversation_members JOIN conversations`, with no new tables at all. Metadata
stays exactly where it has always lived, on the membership row.

This also simplifies code that predates the change: `ArchivedConversation` and
`IsArchivedMember` currently rehydrate a membership snapshot out of the tombstone
JSON to answer questions SQL can now answer directly.

**What has to change with it** — several places detect "archived" by
`Conversations().Get` returning `ErrNotFound`, and must switch to reading
`archived_at`:

- `ClassifyAgentAccess` (`domain/peer.go`) — the hot-lookup-then-tombstone
  fallback collapses into one read, since `kind` is now on a live row;
- the `getConversation` and `listMessages` archive fallbacks
  (`api/shared.go:141`) — the conversation shell and membership come from SQL,
  only the *messages* come from the sink;
- `domain/archive_read.go` — `ArchivedMessages` still reads the sink;
  `ArchivedConversation` no longer needs to.

The tombstone keeps its membership snapshot regardless, so an archive export
stays self-contained.

**Migration.** Conversations archived before this change have already lost their
rows. The backfill restores `conversations` + `conversation_members` from
tombstone snapshots — which carry `external_user_id`, `conv_role`, `member_kind`,
`metadata` and `joined_at` — with `archived_at` set. Restored rows get synthetic
membership ids, so the metadata tie-break below can use the membership id
uniformly rather than falling back to `conversation_id`.

**Known backfill gap:** the snapshot is built from `Members().ListActive`
(`archive/job.go:50`), so members who *left* before archival are absent from it.
The existence rule counts left memberships, but for already-archived
conversations that history is unrecoverable — accept it and say so, rather than
implying the backfill is complete. Conversations archived after this change keep
their departed members, since the rows are no longer deleted.

(Unrelated but adjacent: the field-list comment on `models.ConversationArchive`
omits `metadata` and `joined_at`, which the actual `views.Member` snapshot does
contain. Fix the comment so an implementer does not trust it over the code.)

**The query rules still hold, because they were about the query, not the
storage.** Scope is applied to membership rows *before* aggregation, for the
three reasons below; existence and activity remain different questions. The only
thing that changed is that nothing needs mirroring to make it possible.

Aggregating under scope means the read path is a `GROUP BY` with the keyset
predicate in `HAVING` — `gormstore.Paginate` needs its aggregate-aware variant.
That was true of every version of this design and is the honest cost of
per-caller correctness.

**No synchronization contract is needed, and that is the main reason to prefer
this shape.** A mirror table would have duplicated authorization-sensitive facts
— assignment state, membership state, archive state, activity — each maintained
by a different mutation path: membership add/remove/remap, assignment
claim/close/return-to-queue, message activity, archival, metadata edits. Every
one of those would need a same-transaction projection write, and a missed trigger
on the assignment or archive flag is not a stale read but an **authorization
leak**. Reading the source tables directly removes that entire failure class,
along with the reconciliation job that would otherwise be needed to detect it.

### Why scope must be applied before aggregation

A single pre-aggregated row per contact — which is what a `contacts` table would
have stored — cannot be scoped safely:

- **Metadata leak.** If a contact's newest membership is in a peer conversation,
  a stored winner is peer-derived, and an operator without `peer_access` reads
  it — scope having been applied to the counters but not to the field the winner
  was chosen from.
- **Ordering leak.** A contact ranks higher because of peer activity the caller
  cannot see. Correcting the *returned* counters does not help: position in the
  list, and therefore the cursor, already encodes the hidden activity.
- **Broken pagination.** Filtering unauthorized contacts out of a fetched page
  yields short pages, and a keyset cursor taken from a filtered page can skip
  authorized rows.

Per-scope precomputation is also fragile: `kinds` is a small closed set today, so
support/peer columns would just about work, but every future scope dimension
multiplies the column count, and a `participant`-scoped read (the user-JWT case)
has no bounded column form at all. Deriving per request keeps the cost
proportional.

### Contact data lifecycle

Retaining conversation and member rows past archival means member metadata is now
retained for as long as the tenant exists, where previously it died with the
purge. That is the intended outcome — it is what makes the directory durable —
but it must be deliberate:

- **Tenant deletion** cascades to conversations and members, as today.
- **Per-subject erasure.** An operation that anonymizes a contact across a
  tenant: `external_user_id` replaced with a tombstone value and member
  `metadata` cleared, while the rows themselves stay so history and counts remain
  coherent. This is now the *only* mechanism that removes personal data from
  archived conversations, since the purge no longer does it — worth stating
  plainly rather than discovering during a deletion request.
- **Retention horizon** (optional, deferrable): a job that hard-deletes archived
  conversation and member rows past a configurable age, restoring the old
  behaviour for tenants that want it.

*Identity.* A contact is the pair `(tenant_id, external_user_id)`. Rows are
grouped by `external_user_id`; membership ULIDs are never exposed, because they
identify a *membership*, not a person.

*Which metadata wins.* The most recently joined **authorized** membership —
`MAX(joined_at)` over the scope-filtered membership rows. Rationale: metadata is
a snapshot at join time (phone, app version, plate), so the newest row is the
freshest known state of that person. This is a real choice with visible
consequences — a contact whose phone changed shows the new one, and two operators
with different scopes may legitimately see different metadata for the same person
— and it must be written down rather than falling out of whatever the query
planner returns.

*Tie-break.* `joined_at` descending, then **membership id descending**. This
works because membership rows now survive archival, and the backfill mints
synthetic ids for rows restored from tombstones. Independently worth fixing:
`views.Member` omits the membership id (`views/views.go:73`), so archived
snapshots cannot express this tie-break on their own — add it, so an export
remains sufficient to rebuild the projection.

*Metadata is not merged across memberships.* One membership's blob wins whole;
keys are not unioned. Merging would fabricate a person who never existed —
a phone from one trip, a plate from another — and make the result order-dependent.
A single snapshot is explainable: "this is what we knew when they last joined."

*Existence and activity are different questions.* Conflating them produces a
contradiction — archived-only contacts must stay findable, yet contacts with no
*current* conversation must not inflate the working set. Separate the two:

- **Existence** — a contact appears if it has **at least one authorized
  membership row**, whatever its state: left, closed, or archived. Archived
  conversations are authorized on the same terms as hot ones (an agent reaches an
  archived formerly-support conversation via the tombstone, per
  `ClassifyAgentAccess`), so "authorized" is one rule, not two.
- **`last_activity`** — computed over **all** authorized rows, archived and left
  included. It answers "when did we last hear from this person", which does not
  become unknowable because a conversation aged out.
- **`conversation_count`** — counts only **current** rows: not left, not
  archived. It answers "how many live threads do they have".

The two deliberately range over different sets. Collapsing them is what makes
"activity excludes archived rows" and "archived-only contacts keep a timestamp"
look contradictory; separated, both hold.

So a contact whose conversations are all archived is findable, with
`conversation_count: 0` and a real `last_activity` from their last active period
— present in the directory, absent from the working set. A contact with **no**
authorized membership row at all does not appear, which is the peer-enumeration
guard, and it applies before aggregation, not after.

*Peer and support memberships are the same person.* No separate namespace — the
identity is `external_user_id`. But scope applies before aggregation, so an
operator without `peer_access` sees that person's support-side activity only, and
a contact with no visible conversations does not appear at all. Two operators can
therefore legitimately see different `conversation_count` values for the same
contact; that is the scope working, not an inconsistency.

*No server-side display name.* The response returns `external_user_id` and the
winning `metadata` blob; the client derives the label with the `memberDisplayName`
logic it already has (`inbox/src/store/map.ts`). Duplicating naming rules on the
server would give two sources of truth for what a person is called.

*Searchable-field disclosure.* `?q=` matches the same fields conversation search
matches — `member_search_text` (built from the tenant's
`searchable_metadata_keys`) plus `external_user_id` — and the response returns
the winning metadata blob whole. A tenant that puts a sensitive value in member
metadata has already exposed it to every operator through the Details panel; this
resource must not *widen* that, so it does **not** adopt the peer search's
raw-metadata fallback, which matches keys outside the tenant's allowlist.

*Shape.*
```json
{ "external_user_id": "u_123",
  "metadata": { … },          // from the winning membership
  "last_activity": "…",       // newest activity across visible conversations
  "conversation_count": 4 }   // visible conversations only
```

*Ordering and cursor.* `last_activity` descending, `external_user_id` as
tiebreak. The cursor is
`{Key: SortLastActivity, Order: desc, Value: <last_activity>, ID: <external_user_id>}`.
`Cursor.ID` here is not a ULID; the type is a string precisely so a projection
can key on its own identity. Because the sort value is an aggregate over
scope-filtered membership rows, the keyset predicate lives in `HAVING` — the one
place `gormstore.Paginate` needs an aggregate-aware variant.

*Scope is applied before aggregation, never after.* This is the whole design and
deserves its own line in the code: filtering a fetched page produces short pages
and skipped rows, and any aggregate computed over unauthorized rows leaks through
ordering even when the returned numbers are corrected. The scope goes into the
`WHERE` on the membership join; everything downstream sees only authorized
rows.

Because the aggregate is per-caller, two operators can legitimately see different
`conversation_count`, different `last_activity`, and different metadata for the
same contact. That is the scope working, not an inconsistency — worth a comment,
since it will otherwise be reported as a bug.

### `brands`

```
GET /v1/brands/active?app_id=   -- any principal; app_id only for the unauth widget load
GET /v1/brands                  -- owner/admin
PUT /v1/brands                  -- owner/admin
```

Replaces `/v1/public/brand`, `/v1/me/brand`, `/v1/admin/brands`.

**`/v1/brands` is a singleton configuration aggregate, and is exempt from the
collection contract.** It returns `{brands, active_brand_id}`, not the page
envelope. That is a real exception to "every collection, without exception", so
it needs stating rather than quietly existing — which is why it was missing from
the pagination conversion table.

The justification is that it is not a collection. `PUT /v1/brands` replaces the
entire set atomically (`BrandRepo.Replace` = delete-all + insert) and
`active_brand_id` is a property *of the set*, not of any member. A paginated read
whose write is whole-set replacement is incoherent: page 2 could not be saved
without page 1. The resource is one configuration document that happens to
contain a list.

Recorded in `ARCHITECTURE.md` as the narrow exemption: a resource may be an
aggregate rather than a collection when it is read and written as a unit. The
test is the write path — if `PUT` replaces everything, `GET` is not a page.
Individual brands are not separately addressable, so there is no `/v1/brands/:id`.

### Drop the `/admin` prefix (settings resources)

`/v1/api-keys`, `/v1/webhooks`, `/v1/webhooks/:id/deliveries`, `/v1/team`,
`/v1/team/:id`, `/v1/team/:id/password`, `/v1/channels/email`,
`/v1/push-tokens` (was `/v1/me/push-tokens`), `/v1/realtime/token`,
`/v1/principal` (was `/v1/admin/me` — see below).

**This is not mechanical, and the plan must not pretend it is.** The `/me`,
`/admin` and `priv` prefixes currently *select the authentication middleware* —
`me` is `UserJWT()`, `admin` is `Admin()`, `priv` is `Admin()` +
`RequireRole(owner, admin)` (`api/handler.go:82,101,114`). Removing a prefix
removes the only thing that assigns a route its credential. The route table must
therefore keep the same five groups and register each moved route into the group
it had before; the *path* stops encoding auth, the *group* still does.

Binding matrix — every route, its group, and whether that binding changes:

| routes | group | middleware | change |
|---|---|---|---|
| `GET/POST/DELETE /v1/conversations/:id/*` (read, messages, typing, close) | `any` | `Any()` | — |
| `POST /v1/uploads` | `any` | `Any()` | — |
| `GET /v1/conversations` | `any` | `Any()` | new route; scope narrows per principal |
| `POST /v1/conversations` | `any` | `Any()` | key-only → all three, with the `open_assignment` rule above |
| `POST /v1/tokens` | `key` | `APIKey()` | — |
| `POST /v1/conversations/:id/members`, `DELETE …/:user_id`, `POST …/members/remap` | `key` | `APIKey()` | — |
| `POST /v1/conversations/:id/assignments` | `key` + `admin` | `Any()` + operator check | key-only → key \| admin |
| `POST /v1/push-tokens`, `DELETE /v1/push-tokens` | `user` | `UserJWT()` | — (path move only) |
| `GET /v1/brands/active` | *(none)* | optional auth | `?app_id=` unauth, or any credential |
| `PATCH /v1/assignments/:id` | `admin` | `Admin()` | — |
| `GET /v1/realtime/token` | `admin` | `Admin()` | — (path move only) |
| `GET /v1/principal` | `any` | `Any()` | `Admin()`-only → any principal |
| `GET /v1/contacts`, `GET /v1/contacts/:external_user_id` | `admin` | `Admin()` | new |
| `/v1/api-keys*`, `/v1/webhooks*`, `/v1/team*`, `/v1/channels/email`, `/v1/brands` (GET/PUT) | `priv` | `Admin()` + `RequireRole` | — (path move only) |
| `/v1/admin/auth/*` | *(none)* | — | — |

### `/v1/me` becomes `/v1/principal`

An `Admin()`-only `/v1/me` is still consumer-shaped — it means "me the operator",
which is exactly the framing this refactor rejects. And my reason for keeping it
admin-only (three response shapes for one path) is answered by making the shape
*discriminated* rather than by restricting the route:

```json
{ "kind": "admin",
  "tenant_id": "t_01J…",
  "subject": { "id": "au_01J…", "role": "agent" },
  "grants": [
    { "action": "conversations.list",
      "scope": { "kinds": ["support", "peer"], "requires_assignment": true } },
    { "action": "assignments.claim" },
    { "action": "messages.send" }
  ] }
```

`kind` is `admin | user | apikey`; `subject` varies by kind.

**Grants are scoped, not flat.** A flat capability list cannot express the thing
the inbox actually needs to know. An agent with `peer_access` and one without
*both* hold `conversations.list` — what differs is the scope it is held over. A
frontend given `["conversations.list", …]` still cannot decide whether to render
the peer surface, so it would fall back to reading `peer_access` itself, and the
duplicate policy logic this was meant to remove survives.

So each grant carries a **`GrantScopeDTO`** — an explicit, allowlisted projection
of the internal `ResourceScope`, with its own conversion function. Not the scope
itself: its fields are deliberately unexported so only `policy` can construct it,
which means `json.Marshal` yields `{}`. Serializing it directly would produce an
empty object *and* couple the public API to policy internals, so that every
future scope attribute leaks to clients by default. The DTO inverts that — new
attributes are private until explicitly added.

The inbox renders the peer surface when `conversations.list` is granted over
`kinds` containing `peer`. One derivation, two representations: the opaque scope
the repos consume, the allowlisted DTO clients see.

(The alternative — minting `peer_conversations.list` as a distinct action —
keeps the payload flat but multiplies the action catalog by every scope
dimension, and the next attribute would double it again. Scoped grants stay
proportional.)

`GET /v1/principal`, `Any()` credential. `/v1/me` and `/v1/admin/me` both go.

### `GET /v1/brands/active` — downgrade and caching rules

Merging a public and an authenticated read on one path needs rules written down,
because the current public handler sets `Cache-Control: public, max-age=60`
(`api/brand.go:88`) and an authenticated response inheriting that would populate
a shared cache with one tenant's brand under a URL another tenant will request.

- **No silent downgrade.** A request carrying a credential that is present but
  invalid or expired returns **401**. It must never fall back to public mode —
  that would turn an expired session into a silent tenant switch.
- **Precedence:** credential first, `?app_id=` only when no credential is
  present. An authenticated caller's `app_id` is ignored, so the public path
  cannot read another tenant's brand while authenticated.
- **Caching:** public responses `Cache-Control: public, max-age=60` keyed by
  `app_id`; authenticated responses `Cache-Control: private, no-store`. Plus
  `Vary: Authorization, Cookie`, so no shared cache can serve an authenticated
  response to an anonymous request.
- A missing `app_id` on an unauthenticated request is **400**, not an empty
  brand.

### Unchanged

`/.well-known/jwks.json`, `/widget.js`, `/healthz`, `/readyz`,
`POST /v1/tokens`, `POST /v1/uploads`, `/v1/uploads/local/*`,
`POST /v1/email/inbound`, `/v1/ws`, `/v1/ws/sse`, and **`/v1/admin/auth/*`** —
the deliberate consumer-shaped exception.

---

## Full endpoint map (before → after)

62 route registrations → 55, numbered in the inventory below. Auth column shows
what changes; `—` means unchanged. Route
totals are unaffected by the `/v1/me` → `/v1/principal` decision (one route
either way) and by the `/v1/brands` aggregate exemption (shape, not count).

### Conversations: list & search — 5 routes → 1

| # | Before | After | Auth |
|---|---|---|---|
| 1 | `GET /v1/me/conversations` | `GET /v1/conversations` | — (scope forces `participant=sub`) |
| 2 | `GET /v1/admin/assignments?status=&assignee=&exclude_closed=&sort=&order=&limit=&cursor=` | `GET /v1/conversations?kind=support&assignment_status=&assignee=&sort=&order=&limit=&cursor=` | — |
| 3 | `GET /v1/admin/contacts/conversations?external_user_id=` | `GET /v1/conversations?participant=<id>` | — |
| 4 | `GET /v1/admin/peer-conversations?role=&limit=&cursor=` | `GET /v1/conversations?kind=peer&role=&…` | route guard → scope filter |
| 5 | `GET /v1/admin/search?q=&before=&limit=` | `GET /v1/conversations?q=&cursor=&limit=` | — |
| 6 | `GET /v1/admin/search?peer=true&q=` | `GET /v1/conversations?kind=peer&q=` | route guard → scope filter |

Renames inside this merge: `?before=` → `?cursor=` (the two list families spell
the same keyset cursor differently today), and `?peer=true` → `?kind=peer`.

### Conversations: create, read, write

| # | Before | After | Auth |
|---|---|---|---|
| 7 | `POST /v1/conversations` | same | key → key \| user JWT \| admin; `open_assignment:false` (⇒ `kind=peer`) stays key-only, 403 otherwise |
| 8 | `POST /v1/me/support-requests` | `POST /v1/conversations` `{open_assignment:true}` | — (members forced to self) |
| 9 | `POST /v1/admin/support-requests` | `POST /v1/conversations` `{open_assignment:true, members:[…]}` | — |
| 10 | `GET /v1/conversations/:id` | same | — |
| 11 | `GET /v1/conversations/:id/messages?before=` | `…?cursor=` (+ `?since=` for catch-up) | — |
| 12 | `POST /v1/conversations/:id/messages` | same | — |
| 13 | `POST /v1/admin/peer-conversations/:id/messages` | `POST /v1/conversations/:id/messages` | route guard → scope filter |
| 14 | `POST /v1/conversations/:id/read` | same | — |
| 15 | `POST /v1/conversations/:id/typing` | same | — |
| 16 | `POST /v1/conversations/:id/close` | same | — |
| 17 | `POST /v1/conversations/:id/members` | same | — |
| 18 | `DELETE /v1/conversations/:id/members/:user_id` | same | — |
| 19 | `POST /v1/conversations/:id/members/remap` | same | — (API key only) |
| 20 | `POST /v1/conversations/:id/assignments` | same | key → key \| admin |

Row 13 is the implicit-join path: it fires on the shared route when the
principal is an operator and the conversation is peer.

### Assignments — 2 → 1

| # | Before | After |
|---|---|---|
| 21 | `POST /v1/admin/assignments/:id/claim` | `PATCH /v1/assignments/:id` `{"assignee_actor_id":"me"}` |
| 22 | `POST /v1/admin/assignments/:id/close` | `PATCH /v1/assignments/:id` `{"status":"closed"}` |

`"me"` is the only accepted assignee and `"closed"` the only accepted status —
see the per-field RBAC table above. The `PATCH` is a spelling change, not a
capability change.

### Contacts — new

| # | Before | After |
|---|---|---|
| 23 | — | `GET /v1/contacts?q=&limit=&cursor=` |
| 24 | — | `GET /v1/contacts/:external_user_id` |

### Brands — 4 → 3

| # | Before | After | Auth |
|---|---|---|---|
| 25 | `GET /v1/public/brand?app_id=` | `GET /v1/brands/active?app_id=` | — (unauthenticated) |
| 26 | `GET /v1/me/brand` | `GET /v1/brands/active` | user JWT → any principal |
| 27 | `GET /v1/admin/brands` | `GET /v1/brands` | — |
| 28 | `PUT /v1/admin/brands` | `PUT /v1/brands` | — |

### Identity, realtime, push — path moves only

| # | Before | After |
|---|---|---|
| 29 | `GET /v1/admin/me` | `GET /v1/principal` (any principal; scoped grants) |
| 30 | `GET /v1/admin/realtime/token` | `GET /v1/realtime/token` |
| 31 | `POST /v1/me/push-tokens` | `POST /v1/push-tokens` |
| 32 | `DELETE /v1/me/push-tokens` | `DELETE /v1/push-tokens` |

### Settings — prefix drop, plus the list envelope

| # | Before | After |
|---|---|---|
| 33–35 | `POST\|GET /v1/admin/api-keys`, `DELETE /v1/admin/api-keys/:id` | `/v1/api-keys`, `/v1/api-keys/:id` |
| 36–40 | `POST\|GET /v1/admin/webhooks`, `PATCH\|DELETE /v1/admin/webhooks/:id`, `GET /v1/admin/webhooks/:id/deliveries` | `/v1/webhooks`, `/v1/webhooks/:id`, `/v1/webhooks/:id/deliveries` |
| 41–42 | `GET\|PATCH /v1/admin/channels/email` | `/v1/channels/email` |
| 43–46 | `GET\|POST /v1/admin/team`, `PATCH /v1/admin/team/:id`, `POST /v1/admin/team/:id/password` | `/v1/team`, `/v1/team/:id`, `/v1/team/:id/password` |

RBAC unchanged. The four `GET` list routes here become genuinely paginated —
`?limit=&cursor=`, the standard envelope, the same `Paginate` code path as
conversations — keyed on ULID id. Default limit 100, so one page covers a
tenant's whole team or key list in practice.

### Unchanged

| # | Route | Why it stays |
|---|---|---|
| 47 | `POST /v1/tokens` | mints a credential; not a model |
| 48 | `POST /v1/uploads` | already shared across all principals |
| 49–50 | `PUT\|GET /v1/uploads/local/*objectKey` | storage-backend detail |
| 51 | `POST /v1/email/inbound` | connector ingress; signature is the gate |
| 52–53 | `GET /v1/ws`, `GET /v1/ws/sse` | transport |
| 54 | `GET /.well-known/jwks.json` | well-known |
| 55 | `GET /widget.js` | static asset |
| 56–57 | `GET /healthz`, `GET /readyz` | infra |
| 58–62 | `/v1/admin/auth/{google,google/callback,google/dev,password,logout}` | **the deliberate exception** — obtaining a credential is genuinely consumer-specific |

---

## Authorization: one scope, one place

The heart of the change. Today each route encodes its own scope. Replace with a
single value resolved from the principal.

Sild's authorization is **attribute-based**, not role-based, and pretending
otherwise is how it fragmented. A decision depends on principal kind, platform
role, `peer_access`, membership, assignment existence, conversation kind,
conversation status, and tenant — eight attributes, currently consulted ad hoc by
whichever handler needs them. A conversation-specific `ScopeFor` would fix the
list path and leave the rest.

### The policy layer

**First, break the import cycle — the naive layering does not compile.**
`middleware` imports `store` to resolve credentials (`middleware/auth.go:10`).
If `policy` takes a `middleware.Principal` and `store` takes a
`policy.ResourceScope`, the graph is `store → policy → middleware → store`.

Fix: move the principal *data type* into a dependency-neutral package.

```
internal/principal   -- Principal, PrincipalKind. No imports beyond models.
        ↑        ↑
   middleware   policy        middleware constructs it; policy reads it
        ↑        ↑
      store ─────┘            store depends on policy (scope) + principal only
```

`policy` must not import `middleware` or the top-level `store` package. If
`policy` needs to read data to decide (assignment existence, membership), it
takes narrow interfaces it declares itself, satisfied by the repos — the same
consumer-side-interface pattern the repos already use.

New package `backend/internal/policy`, with exactly two operations:

```go
// Authorize decides a single action against a known resource.
func Authorize(p *principal.Principal, a Action, r ResourceAttrs) error

// Scope produces the ceiling for a collection query. The returned value is the
// ONLY way to construct a repo-visible scope (unexported fields, constructed
// here), so a query that forgot authorization does not compile.
func Scope(p *principal.Principal, a Action) ResourceScope
```

Actions are stable strings, versioned with the API. The catalog must cover
**every** manifest route — a route without an action cannot be registered, so
gaps here are build failures, not oversights:

```
conversations.list            conversations.read
conversations.create_support  conversations.create_peer
conversations.close           members.manage        members.remap
messages.read                 messages.send
receipts.write                typing.write
assignments.create            assignments.claim     assignments.close
contacts.list                 contacts.read
uploads.issue                 uploads.read
tokens.mint                   realtime.token
push_tokens.manage            principal.read
brands.read_active            brands.read           brands.write
settings.read                 settings.write
api_keys.manage               webhooks.manage       webhooks.read_deliveries
team.manage                   email.inbound
```

`brands.read_active` is separate from `brands.read` because the former is
reachable with no credential at all; collapsing them would make the public path
inherit an operator capability.

Rules the layer enforces, and the handlers therefore stop restating:

- Roles map to capabilities in **one** table. Nothing else reads
  `models.PlatformOwner`.
- Membership, conversation kind, assignment existence and `peer_access` are
  policy *attributes*, not handler conditionals.
- **Handlers never inspect `Role`, `PeerAccess` or `Kind` directly.** A grep for
  `p.PeerAccess` outside `policy/` is a lint failure — that is the enforceable
  version of this principle, and worth adding as a CI check.
- Collections get a policy-produced scope; object reads and mutations call
  `Authorize` with resource attributes. Same table, two entry points.
- Client filters may only *narrow* a scope.

`ResourceScope` for conversations carries kinds, participant and
requires-assignment. Fields are unexported with no public constructor — but
**inspection must be exported, or the store cannot read it**: Go gives no package
access to another package's unexported fields, so a scope the repo can receive
but not read is useless.

```go
type ResourceScope struct {
    kinds              []models.ConversationKind
    participant        *string
    requiresAssignment bool
}

// Getters return copies; the slice is never handed out by reference.
func (s ResourceScope) AllowedKinds() []models.ConversationKind
func (s ResourceScope) Participant() (string, bool)
func (s ResourceScope) RequiresAssignment() bool
```

Construction restricted, inspection open. A repo can build its `WHERE` clause; no
package outside `policy` can fabricate a scope or widen one it was given.

`AssigneeActorID` stays out of `ResourceScope` entirely and lives only in
`ConversationQuery` — it is a client-chosen filter (the You tab), never an
authorization ceiling. Keeping the two in separate types is what stops the
`AssignedOnly` confusion recurring.

`ConversationRepo.List` takes the scope as a distinct first argument:

```go
List(ctx context.Context, scope policy.ResourceScope, q ConversationQuery) (Page[ConversationItem], error)
```

That is the structural point. Today a repo takes `tenantID string` and trusts
the caller; a missing scope is invisible. With a scope type only the policy layer
can build, forgetting authorization is a compile error rather than a leak.

**`RequiresAssignment` means the conversation carries *an* assignment — anyone's,
or nobody's.** Not "assigned to the caller" — that reading is a permission
narrowing this plan does not intend, and it would empty an agent's All and
Unassigned tabs and strip their socket of every unclaimed conversation.

The actual rule, in three places that agree:

- `apiutil.AuthorizeConversation` → `ClassifyAgentAccess` →
  `HasAssignment(ctx, tenant, convID)`, which is
  `Assignments().GetByConversation(...) == nil` — existence of an assignment, no
  assignee comparison (`domain/archive_read.go:13`).
- `listAssignments` applies no per-agent narrowing at all; `assignee=me` is a
  client-chosen filter for the You tab, and `assignee=<any id>` is already
  accepted (`api/admin.go`).
- `agentSubscriptions` subscribes to `Assignments().ConversationIDs(tenantID)` —
  tenant-wide (`realtime/node.go`).

Since `kind` is set to `support` exactly when `open_assignment` creates an
assignment, and assignments are never deleted outside archival, a support
conversation essentially always carries one. `RequiresAssignment` is therefore
close to a no-op in practice — which is the point: it is the current behaviour,
and encoding anything stricter into `policy.Scope` would silently narrow
permissions in phase 1 and cascade into realtime, contacts and the authorization
matrix.

Archived formerly-support conversations remain open to any agent unconditionally,
as today.

Behaviour preserved exactly (this is a refactor, not a permission change):

| principal | scope |
|---|---|
| API key | all kinds, tenant-wide |
| owner/admin **with** `peer_access` | all kinds, tenant-wide |
| owner/admin **without** `peer_access` | `Kinds={support}`, tenant-wide |
| agent **with** `peer_access` | `Kinds={support,peer}`, `RequiresAssignment` for support |
| agent **without** `peer_access` | `Kinds={support}`, `RequiresAssignment` |
| user JWT | `Participant=<sub>` |

**List/search intersect; single-object reads 403.** A list request for
`kind=peer` without access returns an empty page (the scope is a ceiling, not a
gate). `Authorize(conversations.read)` returns 403 for a named object, because
there the caller identified something specific and silence is a worse answer than
a refusal.

`search.Query.PeerOnly` is deleted; `buildFilters` consumes the scope's kinds and
stops deciding for itself. **Still the riskiest edit in the plan** — that clause
is the boundary quoted in Context.

### Realtime uses the same policy

`realtime/node.go:121` (`agentSubscriptions`) is a *second* authorization
implementation: it independently reads `admin.PeerAccess` and the assignment id
set to build a channel list. It happens to agree with `apiutil.AuthorizeConversation`
today. Nothing enforces that, and a policy change applied to one surface and not
the other is a silent leak — the socket is precisely where a leak is least
visible.

`agentSubscriptions` is rewritten to derive its channel set from
`policy.Scope(p, conversations.list)`. Subscription and REST visibility then
cannot diverge, because there is one function.

**The parity test asserts event delivery, not set equality.** Comparing "visible
conversation ids" against "subscribed channels" is not literally meaningful: the
channel set mixes granularities — `user:<adminID>`, `agents:<tenant>`,
`conv:<id>`, `conv:<id>:internal`, and `peer:<tenant>`, where *one* tenant channel
covers every peer conversation (`realtime/node.go:117`). There is no conversation
id to compare it against.

So the test publishes representative events — a support message, an assignment
update, an internal note, a peer message — and asserts each is received **iff**
policy permits that principal to read it. That is the property that actually
matters, and it survives future changes to channel granularity.

It must also cover **live `peer_access` grant and revoke**, since
`reconcilePeerSubscriptions` exists precisely so a flag change takes effect
without a reconnect (`realtime/publisher.go:66`). A parity test that only checks
connect-time state would pass while revocation silently failed.

### One authorization test

Table-driven over the full cross product: every principal kind × platform role ×
`peer_access` × action × resource state (support/peer, open/closed, member/
non-member, assigned/unassigned, own-tenant/cross-tenant). Expected outcome per
row. That table replaces the current spot-checks in `auth_rbac_test.go` and
`agent_scope_test.go`, and adding an action without adding rows fails.

## Route contract manifest

Dropping `/admin` removes the thing that currently assigns a route its
credential. A binding matrix in a markdown file does not prevent that; an
enumerable manifest does.

One descriptor per route, declared next to the route:

| field | purpose |
|---|---|
| method, path | identity |
| classification | `actions: [<name>…]` + resolver \| `public` \| `signed_ingress` \| `infrastructure` |
| principals | allowed principal kinds |
| body_limit | max request bytes |
| rate_class | rate-limit bucket |
| idempotent | whether `Idempotency-Key` is honoured/required |
| request, response | schema refs |

`Mount` builds the router *from* the manifest, so a route cannot exist without a
declared classification and principal set. OpenAPI is generated from the same
source rather than maintained beside it.

**A route declares a *set* of possible actions, not one.** Unification is exactly
what breaks the one-route-one-action assumption — the merged routes select their
action from the request:

| route | actions | selected by |
|---|---|---|
| `POST /v1/conversations` | `conversations.create_support` \| `conversations.create_peer` | `open_assignment` in the body |
| `PATCH /v1/assignments/:id` | `assignments.claim` \| `assignments.close` | which field is present |
| `GET /v1/brands/active` | *public* \| `brands.read_active` | whether a credential was presented |

So each descriptor declares the finite action set plus a **resolver** — a
function run after authentication and decoding, before any domain mutation,
returning one member of the declared set. Declaring the set in the manifest keeps
coverage checkable: the contract test enumerates every action a route can select
and exercises each, and a resolver returning an undeclared action is a failure.

Without this, RBAC, audit records and manifest coverage would each guess
differently about what a unified route did — the audit trail being the one that
matters, since "operator changed an assignment" is not a useful record when the
question is whether they claimed or closed it.

**Every route is classified; not every route has an action.** Some routes
legitimately authorize nothing, and silently exempting them would hollow out the
invariant. Four classifications, each explicit:

| classification | routes | meaning |
|---|---|---|
| `action` | everything in the catalog above | authorized by `policy` |
| `public` | `GET /v1/brands/active` (unauth form), `/v1/admin/auth/*` | reachable with no credential; a credential-acquisition route cannot require a credential. Still carries `rate_class` and `body_limit` |
| `signed_ingress` | `POST /v1/email/inbound` | a signature is the gate, not a principal |
| `infrastructure` | `/.well-known/jwks.json`, `/widget.js`, `/healthz`, `/readyz` | static or operational; no tenant data |

The local upload routes are **not** exempt: they get real actions,
`uploads.write` and `uploads.read`. Classifying them `public` would encode the
current vulnerability as intended behaviour.

But an action needs a principal to authorize, and these routes have no
credential — which is why they also need a **signed-capability principal**:

```
PrincipalKind = apikey | user | admin | signed
```

A `signed` principal is minted by verifying the URL's HMAC, and carries exactly
what the signature binds: **tenant, object key, permitted method, expiry**. Its
capability is that one object and that one verb, nothing else — it cannot list,
cannot read another key, cannot outlive the expiry. `policy.Authorize` then
handles `uploads.read`/`uploads.write` the same way it handles everything else,
and the manifest's `principals` field can finally describe these routes truthfully
instead of leaving them as a hole.

This is why the HMAC work is a release prerequisite rather than hardening: until
the signature exists there is nothing to build the principal from, so the routes
cannot be expressed in the manifest at all.

The contract test enumerates every mounted route and asserts, for each:

- unauthenticated → the documented status (401, or 200 for the public ones);
- wrong credential type (user JWT at an operator route, etc.) → 401/403;
- authenticated but insufficient capability → 403;
- a resource id belonging to another tenant → the documented 403/404, never 200;
- a valid principal → success;
- the response validates against the declared schema.

This is what makes "dropping `/admin` cannot drop authentication" a mechanical
guarantee instead of a review promise. It also replaces spot-checks: a new route
is covered the moment it is registered.

## Body limits are middleware, not decoding

A body cap inside the JSON decoder does not close this vulnerability.
`localUploadPut` does a raw
`io.Copy(f, c.Request.Body)` (`api/uploads_local.go:18`) — it never decodes JSON,
so a decoder-level cap never runs, and that route is precisely the
unauthenticated unbounded write at `SECURITY-REVIEW.md:58`. Inbound email
(`io.ReadAll` in `api/email.go`) has the same shape.

So `body_limit` is **route middleware**, applied from the manifest via
`http.MaxBytesReader` before the handler runs — JSON, raw upload, and webhook
ingress alike. Limits differ by route (a JSON mutation gets kilobytes, an
attachment PUT gets the configured upload cap), which is exactly why it belongs
in the per-route descriptor rather than in one decoder.

The local upload routes therefore move out of "Unchanged": they gain a body limit
like everything else.

**But a body limit alone makes that route *worse*, not better.** `localUploadPut`
calls `os.Create(full)` — which truncates — *before* it starts copying
(`uploads_local.go:25`). `MaxBytesReader` can only reject an oversized stream
once reading is underway, by which point the existing object has already been
destroyed. Adding the cap without fixing the write turns "upload something huge"
into a reliable way to delete any attachment whose key you can guess.

So the write path changes with the limit, not after it: **write to a temporary
file, `fsync`, then atomically rename into place**, so a rejected or interrupted
upload leaves the previous object intact. This is a prerequisite of the body
limit, not an independent improvement.

Two more items belong to the same route and are tracked as a **pre-release
security prerequisite** below rather than as hardening: HMAC/expiry validation on
the signed URL (the handler validates only the path shape, so the URL is a
permanent bearer token) and tenant-bound object keys (so one tenant cannot reach
another's object by guessing a key).

## Uniform request decoding

Every mutation decodes through one helper rather than a bare
`c.ShouldBindJSON(&req)`:

- JSON `Content-Type` enforcement;
- malformed JSON and trailing-data rejection;
- **unknown-field rejection** on mutation DTOs — a client sending `assigneeId`
  instead of `assignee_actor_id` learns immediately rather than silently having
  it ignored;
- structured per-field validation errors, feeding the `fields` map below;
- **per-principal writable fields.**

That last rule is what makes shared URLs safe. A common path must not imply a
common body: `POST /v1/conversations/:id/messages` lets an API key set
`sender_kind`, `internal_actor_id` and `external_user_id`, which a user or admin
principal must never set — today enforced by a `switch p.Kind` inside the handler
(`api/shared.go:79`) that silently ignores the fields. Under the unified surface
this becomes declarative: fields carry a principal allowlist, and a disallowed
field is a **400 naming the field**, not a silent drop. Silent drops on a shared
route are how a client ends up believing it sent an internal note as an agent.

## Error contract

The envelope is already uniform; the codes are not useful. Six generic values —
`bad_request`, `forbidden`, `not_found`, `conflict`, `unauthorized`, `internal`
(`httpx/respond.go:7`) — mean an SDK must parse human messages to distinguish
"assignment already closed" from "invalid cursor".

```json
{ "error": {
    "code": "assignment_already_closed",
    "message": "Assignment is already closed",
    "request_id": "req_01J…",
    "fields": { "assignment_status": "unsupported value" } } }
```

`message` is for humans and may be reworded freely; `code` is API surface and
changes only with a version. SDKs branch on `code`, never on `message` — state
that normatively so the Kotlin/TS clients are written that way from the start.

Centralized domain-error → HTTP mapping, replacing the per-handler
`apiutil.Fail`. Documented rules:

- **400 vs 422** — 400 for malformed/undecodable input and unknown fields; 422
  for well-formed input failing a semantic rule.
- **401 vs 403** — 401 when no valid credential was presented; 403 when a valid
  credential lacks the capability. Never 401 for an authenticated caller.
- **403 vs 404** for inaccessible resources — pick per resource and document it.
  Conversations return **403**, matching today's `AuthorizeConversation`;
  cross-tenant ids return **404**, so tenant membership is not probeable. These
  differ deliberately and the reason belongs in the code.
- **Conflict codes** are specific: `assignment_already_closed`,
  `conversation_closed`, `idempotency_key_reused`, `stale_version`.
- **Preconditions** on `If-Match` resources: **428** `precondition_required` when
  the header is absent, **400** when it is present but malformed, **412**
  `stale_version` when it is well-formed but stale. One status per cause; 400 is
  never used for a missing precondition.
- **413** `payload_too_large` when a route's `body_limit` is exceeded.
- **429/503** carry `Retry-After`; SDKs retry only these plus network errors.

## Uniform mutation semantics

- `POST` create → **201** with `Location`.
- `PATCH` → partial update; absent means "leave alone", explicit `null` means
  "clear". The decoder must distinguish them (pointer fields), or `PATCH` is
  indistinguishable from `PUT`.
- `DELETE` → **204**.
- Identical retries are safe; concurrent conflicting changes are detectable.

**`Idempotency-Key`** on retryable creates and commands — conversation creation,
support requests, assignment mutations, API-key and webhook creation.

The record's unique key is
`(tenant, principal, method, normalized_route, idempotency_key)`, with the
request hash stored as an **ordinary column** and compared on replay. The hash
must not be part of the key: the same key with a different body would then hash to
a different key and create a second record, making the 409 undetectable by
construction. Same key + same hash →
replay the stored response; same key + different hash → **409
`idempotency_key_reused`**.

`normalized_route` is the route pattern (`/v1/conversations/:id/messages`), not
the concrete path, so the key does not vary by resource id — the id is part of
the request hash instead. This generalizes what messages already do: `client_msg_id`
stays as the message-specific spelling, documented as an instance of the generic
contract rather than a separate mechanism. It also gives the Android duplicate-send
bug (`SECURITY-REVIEW.md:88` H6 — a fresh UUID per retry defeats dedupe) a
server-side backstop, though the SDK fix is still needed.

**Optimistic concurrency** for mutable configuration. `PUT /v1/brands` replaces
a tenant's entire brand set (`BrandRepo.Replace` is delete-all + insert), so two
operators editing Appearance concurrently means last-write-wins with no
indication anything was lost. Add a `version`/`ETag` on brands and the email
channel, require `If-Match`, and return **412** on mismatch.

**This is pre-release work, not hardening.** Making a previously optional request
header *required* is a breaking change — every existing client starts getting 412 — so `If-Match`
cannot be introduced as mandatory after the first consumer exists without a
`/v2`. Either it ships required now, or it never becomes required. It ships now:
two resources, and the alternative is silently discarding an administrator's
concurrent edits forever.

## Audit and observability

`ARCHITECTURE.md:71` lists `request-id` among the middleware, but `server.New`
mounts only `Recovery` and `CORS` (`server/server.go:24`). The doc describes
something that does not exist.

- **Request ID** middleware, on every response *and* every error body (the
  `request_id` field above). The canonical id is always **server-generated**. An
  inbound `X-Request-Id` is accepted only as a correlation hint: validated
  against a strict charset (`[A-Za-z0-9_-]`, ≤64 bytes) and logged as a separate
  field, never echoed as the response id. Reflecting an unvalidated client string
  into every log line and error body is log injection with extra steps.
- **Structured access log**: route, action, status, latency, principal kind,
  tenant. Route pattern, never the raw path — `/v1/conversations/c_01J…` in logs
  is both high-cardinality and a leak.
- **Security audit records** for API-key create/revoke, team and role changes,
  `peer_access` grants and revocations, webhook changes, assignment transitions,
  archived reads, and peer implicit joins. That list is not arbitrary: each is an
  action whose blast radius outlives the request.
- **Never in audit records:** message bodies, tokens, secrets, raw member
  metadata, attachment URLs (they are signed and grant access).
- **Metrics** per action for authorization denials and rate-limit rejections. A
  spike in `conversations.read` denials is how a scope regression surfaces
  before a customer reports it.

Rate limiting itself (`SECURITY-REVIEW.md:99` — absent on password login, token
mint, inbound email) gets its bucket from the manifest's `rate_class`. The
manifest makes it declarative; the limiter is separate work.

## Store layer

One query replaces three. `ConversationRepo` gains:

```go
List(ctx context.Context, scope policy.ResourceScope, q ConversationQuery) (Page[ConversationItem], error)
```

`ConversationQuery` merges today's `QueueParams`, `PeerParams` and the
`ListForUser` filter, embeds `PageParams`, and carries the scope's
`Kinds`/`Participant`/`RequiresAssignment`. `AssignmentRepo.ListQueue`,
`ConversationRepo.ListPeers` and `ConversationRepo.ListForUser` are folded into
it and deleted.

Reuse as-is:

- the keyset shape: `COALESCE(last_message_at, created_at)` + `id` tiebreak,
  identical in `assignmentRepo.ListQueue` and `conversationRepo.ListPeers`
  (`backend/internal/store/gormstore/conversation.go:75`)
- the batched member fetch in `ListPeers` (one `IN` query, no N+1) — apply it to
  all list paths, which also fixes the per-row queries in
  `domain.ListUserConversations` and `domain.ListContactConversations`
  (`backend/internal/domain/membership.go:35,95`)
- `views.QueueRow` / `views.Conversation` as the single row renderer, **extended
  with an optional `snippet`** — see below
- `CountQueue` for the `counts` block

`ListArchivable`, `CountOpen`, `CountOpenSupport` are untouched.

### Search results must keep their snippet

Folding search into `ConversationRepo.List` loses something the queue never had.
`collectHits` attaches a per-conversation `Snippet` — the matching fragment from
whichever message matched, found via `q.Keywords[0]` (`search/common.go:135`) —
and the inbox *replaces* the ordinary last-message preview with it:
`if (hit.snippet) c.preview = hit.snippet` (`RootStore.ts:847`). Neither
`views.QueueRow` nor `views.Conversation` carries such a field. Without it,
searching for a phrase in an old message returns a row previewing the newest
message instead, and the operator cannot see why the row matched.

So: rows returned for a `?q=` query carry optional **representation annotations**
— the resource stays a conversation, and these describe *why this row is here*:

```json
{ "id": "c_01J…", "status": "open", "members": [ … ],
  "snippet": "…refund for trip 8842…",
  "matched_fields": ["message.body"] }
```

`matched_fields` is new and worth the small cost: `buildFilters` matches across
message bodies, `member_search_text`, `external_user_id` and — in peer search —
raw metadata, so a row can appear for reasons the snippet cannot show. A hit on a
phone number in member metadata currently renders as a conversation with an
unrelated preview and no explanation. Values mirror the filter sources:
`message.body`, `member.metadata`, `member.external_user_id`.

Both fields are omitted entirely when there is no `?q=`, so the queue payload is
unchanged.

The acceptance test must match on a term appearing **only in an older message** —
a term in the last message would pass even with the snippet dropped, since the
preview would coincidentally show it. That is the test the current code would
need too.

Worth taking while here: today the inbox issues `getConversation` + `listMessages`
per hit (`RootStore.ts:845`) because search returns only ids and a snippet. Once
search returns full conversation rows from the shared renderer, that N+1 on every
keystroke-driven search disappears.

## Files

**Backend**

- `backend/internal/api/handler.go` — the whole route table
- `backend/internal/api/conversations.go` *(new)* — absorbs `shared.go`,
  `integration.go`, `user.go`, and the conversation parts of `admin.go`/`peer.go`
- `backend/internal/api/contacts.go` *(new)*
- `backend/internal/principal/` *(new)* — `Principal`, `PrincipalKind`, moved out
  of `middleware` so `store` → `policy` → `middleware` → `store` cannot form
- `backend/internal/policy/` *(new)* — `Action`, capability table, `Authorize`,
  `Scope`, `ResourceScope`. Absorbs `apiutil/authz.go`. Imports neither
  `middleware` nor `store`
- `backend/internal/store/gormstore/archive.go` — `PurgeHot` retains
  `conversations` (marked `archived_at`) + `conversation_members`, purging only
  message bulk; migration adds `archived_at` and backfills from tombstones
- `backend/internal/views/views.go` — `Member` gains the membership id, so future
  archived snapshots carry a finer tie-break key than `conversation_id`
- `backend/internal/api/manifest.go` *(new)* — route descriptors; `Mount` builds
  the router from them
- `backend/internal/httpx/decode.go` *(new)* — body caps, content-type,
  unknown-field and per-principal writable-field enforcement
- `backend/internal/httpx/errors.go` *(new)* — domain-error → HTTP mapping, code
  vocabulary, `request_id`, `fields`
- `backend/internal/middleware/requestid.go` *(new)* — the middleware
  `ARCHITECTURE.md:71` already claims exists
- `backend/internal/realtime/node.go` — `agentSubscriptions` derives from
  `policy.Scope` instead of reading `PeerAccess` itself
- `backend/internal/server/server.go` — mount request-id + access log
- `backend/internal/middleware/auth.go` — `OptionalAuth()` for `/v1/brands/active`
- `backend/internal/domain/archive_read.go` — `ArchivedMessages` takes
  `PageParams`, returns `Page[…]`
- `backend/internal/store/slice.go` *(new)* — `SlicePage`, the in-memory twin of
  `Paginate`, shared so hot and archived paging cannot drift
- `backend/internal/apiutil/paging.go` *(new)* — `PageParams` parser
- `backend/internal/httpx/respond.go` — `Page` responder
- `backend/internal/store/page.go` *(new)* — `SortKey`, `Cursor`, `PageParams`, `Page[T]`
- `backend/internal/store/gormstore/page.go` *(new)* — `Paginate`, the one place
  keyset SQL is written
- `backend/internal/store/gormstore/{auth,connector,tenant}.go` — the settings
  repos take `PageParams` and return `Page[T]`
- `backend/internal/domain/conversation.go`, `peer.go`, `membership.go`,
  `assignment.go`, `search.go` — one `ListConversations` entry point
- `backend/internal/search/{query,common}.go` — drop `PeerOnly` and `Before`,
  take `Kinds` + `PageParams`
- `backend/internal/store/repos.go`, `store/gormstore/conversation.go`
- `backend/internal/api/brand.go`, `channels.go` — path moves only

**Clients** (hard cutover — must land in the same change)

- `inbox/src/api/client.ts` — one `Page<T>` type + list helper
- `inbox/src/api/admin.ts` — `listAssignments`, `contactConversations`,
  `listPeerConversations`, `search`, `searchPeer`, `postPeerMessage` collapse
  into `listConversations` / `searchContacts`; every `/admin/*` settings path
  loses its prefix; bare-array readers move to `items`
- `inbox/src/store/RootStore.ts`, `map.ts` — call sites
- `web/src/core/client.ts` — `/me/conversations` → `/conversations`,
  `/me/support-requests` → `POST /conversations`, `/me/brand` +
  `/public/brand` → `/brands/active`, `before` → `cursor`
- `sdks/kotlin/core/src/main/kotlin/io/sild/core/SildApi.kt` — same paths, plus
  the envelope and `since=` for reconnect catch-up
- `backend/internal/webasset/widget.js`, `demo.html`
- `backend/cmd/sild-dev/main.go` — dev seed + `/v1/dev/*` helpers
- `e2e/support/peer.ts`, `e2e/setup/admin.setup.ts`

**Docs**

- `ARCHITECTURE.md` §5 — the two principles above
- `docs/chat-platform-spec.md` §4 — restructure resource-first; §5.4 to describe
  `since=` as a sync read distinct from pagination

## Scope check: this is no longer one change

The plan started as a URL refactor. With the policy layer, route manifest,
decoder, error contract, mutation semantics and observability, it is an API
platform rework — roughly 4–6× the original. That is the right call given the
pre-release window (every item becomes breaking once a consumer exists), but the
document must not pretend it is one pull request.

Split by whether the item **shapes the wire contract** — and therefore must land
before first release — or is **internal machinery** that can follow without a
breaking change:

| | must precede first release | can follow |
|---|---|---|
| **A. Contract** | route paths, pagination envelope + cursor format, error codes, `since`/`cursor` split, `/v1/principal`, brand caching rules, snippet/`matched_fields`, per-principal writable fields | — |
| **B. Enforcement** | policy layer + realtime parity (a leak, not a rename) | lint rule banning `PeerAccess` outside `policy/` |
| **C. Machinery** | body-limit middleware (`SECURITY-REVIEW.md:58` is live); **ETag + required `If-Match`**; rate limits on auth + unauthenticated ingress | general rate limiting, idempotency keys, audit records, metrics |

Rate limiting splits rather than deferring wholesale: password login, token
issuance and unauthenticated ingress (inbound email, local upload) are security
controls on routes reachable by anyone, and there is currently **no limit
anywhere** (`SECURITY-REVIEW.md:99`). Shipping those pre-release; per-route limits
on authenticated endpoints can follow.

`If-Match` moved out of "can follow" on review: promoting an optional header to
required is itself a breaking change, so it is required now or never.
`Idempotency-Key` stays deferrable because it is *accepted-if-present* — adding
support later breaks nobody.

Group A and the policy layer are the deadline-bound work. Rate limiting,
idempotency and audit are additive — they can ship the week after first release
without breaking anyone — but the **manifest fields that describe them**
(`rate_class`, `idempotent`, `body_limit`) belong in group A, because retrofitting
a field onto every route descriptor later is the expensive part, not the limiter.

## Order of work

Phases 1–3 are **one atomic contract migration**, not a refactor followed by a
migration. I labelled phase 1 "behaviour-preserving"; that is false as written —
required `If-Match`, strict content-type, unknown-field rejection, body limits
and new error bodies all change observable behaviour, and clients and tests must
move with them. Pre-release that is fine, but the label mattered because it was
carrying a safety argument it could not support.

Precisely: steps 1–3 and 7–8 *are* behaviour-preserving and the existing suite is
a real oracle for them. Steps 4–6 and 9–10 change the wire contract and need
their tests rewritten in the same breath. Sequencing still matters for
debuggability, but "existing tests stay green throughout" holds only for the
first group.

**Phase 1 — foundations**

1. `policy` package: actions, capability table, `Authorize`, `Scope`,
   `ResourceScope` with unexported fields. Port `AuthorizeConversation` onto it,
   existing authz tests green.
2. Rewrite `agentSubscriptions` (`realtime/node.go:121`) to derive channels from
   `policy.Scope`. Add the REST/realtime parity test. **Before** any route moves,
   so the two surfaces cannot drift mid-refactor.
3. Paging framework: `store.Page[T]`, `Cursor` (versioned, bound),
   `gormstore.Paginate`, `store.SlicePage`, `apiutil.PageParams`, `httpx.Page`.
   Convert the queue and peer list, then the settings collections, then the
   archived-message path — `SlicePage` lands **with** `Paginate`, not after. If
   the hot path ships paginated while the archive still returns everything with
   `has_more:false`, the framework has an exception on day one and the
   archive-mid-pagination case is broken before anyone uses it.
4. Error contract: domain-error → HTTP mapping, `request_id`, `fields`, the code
   vocabulary. Request-ID middleware (`server.go` currently mounts only Recovery
   and CORS, contradicting `ARCHITECTURE.md:71`).
5. Body-limit middleware from the manifest (`http.MaxBytesReader`), covering raw
   upload and inbound email as well as JSON — together with the temp-file +
   atomic-rename rewrite of `localUploadPut`, without which the limit destroys
   existing objects. Request decoder: content-type, unknown-field rejection,
   per-principal writable fields.
6. ETag/`If-Match` on brands and the email channel — pre-release because
   required headers cannot be added later. Rate limits on password login, token
   issuance and unauthenticated ingress.

**Phase 2 — the resource surface**

7. `ConversationRepo.List` + `ConversationQuery`; port `ListQueue`/`ListPeers`/
   `ListForUser` tests onto it before deleting them.
8. `domain.ListConversations`; wire search through the scope, delete `PeerOnly`;
   snippet + `matched_fields`.
9. Route manifest + `Mount` generated from it; `GET /v1/conversations`; then the
   remaining route moves. The contract test enumerating every route lands here —
   it is what makes the moves safe.
10. Archival retention change: `PurgeHot` keeps `conversations` (marked
    `archived_at`) + `conversation_members`, purging only message bulk; migration
    adds `archived_at` and restores rows for already-archived conversations from
    tombstone snapshots. Switch the `ErrNotFound`-means-archived checks in
    `ClassifyAgentAccess` and the `api/shared.go` fallbacks to read `archived_at`.
11. `contacts` read model over `conversation_members JOIN conversations` — no new
    tables, scope applied before aggregation, aggregate-aware `Paginate` variant.

**Phase 3 — clients, atomically**

12. inbox, widget, Kotlin SDK, `webasset/widget.js`, `sild-dev`, e2e — migrated
    together. Per the pre-release assumption there is no interim state to
    support, so a partially migrated client set has no value and real risk.
    Includes the inbox reading scoped grants from `/v1/principal` in place of its
    own `peer_access` logic.
13. Docs: `ARCHITECTURE.md` (resource principle, read-model qualification,
    aggregate exemption, pagination, policy), `docs/chat-platform-spec.md` §4
    restructured resource-first, §5.4 `since`.

**Phase 4 — additive hardening** (can follow first release)

14. General rate limiting by `rate_class`; `Idempotency-Key`; audit records;
    per-action metrics. (ETag/`If-Match` and auth/ingress rate limits are **not**
    here — see phase 1.)

## Pre-release security prerequisite (not part of any phase)

Surfaced by the manifest work, fixed independently of it, and **required before
first release** — which is why it is not in phase 4:

- HMAC/expiry validation on local upload signed URLs. Today the handler checks
  only the path shape, so an issued URL is a permanent bearer token.
- Tenant-bound object keys, so one tenant cannot read or overwrite another's
  object by guessing a key.

The temp-file + atomic-rename write lands earlier, with the body limit, because
the limit is unsafe without it (phase 1 step 5).

Clients break across phases 1–3 as a unit — not only at phase 3, since the body
limits, error bodies and `If-Match` in phase 1 are already wire-visible.

## Capability changes, stated explicitly

This refactor is meant to move URLs, not permissions. Three bindings genuinely
change, each deliberate:

| change | from | to | why |
|---|---|---|---|
| `POST /v1/conversations` | API key | + user JWT, + admin session | absorbs the two support-request routes; peer creation still key-only |
| `POST /v1/conversations/:id/assignments` | API key | + admin session | an operator queueing an existing conversation has no route today |
| `GET /v1/brands/active` | user JWT (`/me/brand`) | any principal or none | merges the public and authenticated brand reads |

Plus one from the policy work: `GET /v1/principal` is readable by **any**
principal, where `/v1/admin/me` was operator-only. It discloses only what the
caller already knows about itself, plus its effective capabilities.

Everything else keeps its exact current principal set. Anything not in this table
that changes access is a bug in the implementation, not an intended outcome —
and the route-manifest contract test, not review, is what catches it.

## Verification

- `cd backend && make test` — `api/agent_scope_test.go`, `auth_rbac_test.go`,
  `scope_counts_test.go`, `peer_test.go`, `peer_fixes_test.go`,
  `invariants_test.go`, `domain/search_test.go` all exercise the paths being
  merged. Port rather than delete.
- **New test, non-negotiable:** an agent *without* `peer_access` gets zero peer
  rows from `GET /v1/conversations` for every combination of `kind=peer`,
  `q=<text matching a peer message body>`,
  `q=<text matching peer member metadata>`, and no filter at all. This is the
  regression guard for the boundary `buildFilters` currently owns.
- New test: user JWT cannot widen `participant=` to another user's id.
- New test: scope intersection returns an empty page (not 403) for a list, while
  `GET /v1/conversations/:id` on the same conversation still 403s.
- New test, **table-driven over every list endpoint** — conversations, search,
  contacts, messages, team, api-keys, webhooks, deliveries: seed more rows than
  `limit`, page to exhaustion, assert no duplicates, no gaps, correct total, and
  `has_more == (next_cursor != null)` on every page. One test body, one row per
  endpoint; a new list endpoint that forgets to use the framework fails to
  compile into the table.
- New test: a cursor minted under one sort key is rejected (400) when replayed
  against another.
- New test: `?limit=0`, `?limit=-1`, `?limit=99999` all clamp rather than error;
  `?cursor=garbage` returns 400.
- New test: **archived history pages** — archive a conversation with more than
  `limit` messages, page to exhaustion through the sink, assert parity with what
  the hot path returned before archival.
- New test: **archive mid-pagination** — take page 1 hot, archive the
  conversation, then present the hot cursor to the archived path and assert the
  client sees no duplicate, no gap, and no error.
- New test: **`since=` catch-up beyond one page** — seed `limit + 1` messages,
  assert a two-call catch-up (re-issuing `since=<last id>`) yields each message
  exactly once. Guards the 500-cap truncation described above.
- New test: every current inbox scope returns the **same row set** before and
  after — table-driven off the five-row scope table, seeded with the case that
  distinguishes the two state machines: an *open* conversation whose latest
  assignment is closed, and a *closed* conversation whose assignment is still
  assigned. A mapping that confuses the axes passes on ordinary data and fails
  only on those two rows.
- New test: a cursor minted under `order=desc` is rejected (400) when replayed
  with `order=asc`, and vice versa.
- New test: contact projection — one `external_user_id` with three memberships
  carrying different metadata returns exactly one row, with the metadata of the
  most recently joined; `last_activity` and `conversation_count` count only
  scope-visible conversations.
- New test: an operator without `peer_access` cannot see, via `GET /v1/contacts`,
  a contact who exists only in peer conversations — the contacts-shaped twin of
  the `buildFilters` regression guard.
- New test: `?q=` matching a term that appears **only in an older message**
  returns that message's snippet, not the last-message preview; a match on member
  metadata reports `matched_fields: ["member.metadata"]`.
- **The authorization table test** — every principal kind × role × `peer_access`
  × action × resource state (support/peer, open/closed, member/non-member,
  assigned/unassigned, own/cross tenant), with the expected outcome per row.
  Replaces the spot-checks in `auth_rbac_test.go` and `agent_scope_test.go`.
  Adding an action without adding rows must fail.
- **REST/realtime parity test:** for each principal in that matrix, publish a
  representative support message, assignment update, internal note and peer
  message; assert each is received **iff** policy permits that principal to read
  it. Includes live `peer_access` grant and revoke so `reconcilePeerSubscriptions`
  stays covered. Deliberately *not* set equality — the channel set mixes
  granularities (`agents:<tenant>`, `peer:<tenant>`, `conv:<id>`) and has no
  conversation-id set to compare against.
- **The route-manifest contract test:** enumerate every mounted route and assert
  unauthenticated behaviour, wrong credential type, insufficient capability,
  cross-tenant id, valid principal, and response-schema conformance. A route
  registered without a declared action or principal set fails to build.
- New test: a mutation DTO with an unknown field is rejected 400 naming the
  field; an API-key-only field (`sender_kind`, `internal_actor_id`) sent by a
  user or admin principal is rejected 400, **not** silently ignored.
- New test: a request body exceeding the route's `body_limit` is rejected **413
  `payload_too_large`, without consuming or persisting more than the allowed
  bytes** — including `PUT /v1/uploads/local/*` and `POST /v1/email/inbound`, the
  two routes that never touch the JSON decoder. (`MaxBytesReader` detects excess
  *while* reading, so "before the body is read" was the wrong assertion; what
  must hold is that nothing beyond the cap is buffered or written.)
- New test: a cursor minted for conversation A is rejected (400) on conversation
  B; a peer-list cursor minted with `role=driver` is rejected when replayed with
  `role=client`. Both are `f`-fingerprint cases the earlier hand-listed filter
  set would have missed.
- New test: a contact whose conversations have all been archived is still
  returned by `GET /v1/contacts`, with `conversation_count: 0`. Assert it against
  a tenant put through the real archival job, not a hand-built fixture.
- New test, **the contacts scope suite** — a contact with one support and one
  peer membership, where the *peer* membership is newer. An operator without
  `peer_access` must see: the support membership's metadata (not the peer one's),
  a `last_activity` from the support conversation only, and a list position
  computed without the peer activity. A page of such contacts must be full-length
  and skip nobody. These are the three leaks a post-aggregation filter cannot
  close, so they need asserting individually.
- New test: an agent's contact aggregates cover every support conversation
  carrying an assignment, **including ones assigned to other operators and ones
  still queued** — the regression guard for the `AssignedOnly` misreading.
- New test: same `Idempotency-Key` with an identical body replays the stored
  response; with a different body returns 409 `idempotency_key_reused`. The
  second assertion is the one a body-hash-in-key design cannot satisfy.
- New test: `PUT /v1/brands` without `If-Match` is **428**, with a malformed one
  **400**, with a stale one **412**; a concurrent two-operator edit cannot
  silently drop one side.
- New test: an oversized `PUT /v1/uploads/local/*` over an existing object is
  rejected **and the original object is byte-identical afterwards** — the
  regression guard for the truncate-before-copy path.
- New test: `GET /v1/principal` returns grants whose `conversations.list` scope
  includes `peer` for a `peer_access` agent and excludes it otherwise — the
  signal the inbox renders the peer surface from.
- New test: `GET /v1/brands/active` with an expired credential returns 401 and
  never the public payload; authenticated responses carry
  `Cache-Control: private, no-store` and `Vary: Authorization, Cookie`.
- New test: paging a list ordered by the mutable `last_activity` while a row is
  touched mid-scan produces a duplicate or gap but never an error — asserting the
  documented live-view semantics rather than a stability we do not have.
- New test: a user JWT sending `open_assignment:false` to `POST /v1/conversations`
  gets 403 and no conversation row is written; same for an admin session.
- New test: `PATCH /v1/assignments/:id` rejects `assignee_actor_id:"<other>"`,
  `assignee_actor_id:null`, `status:"queued"` and both-fields-at-once with 400;
  claim on a closed assignment returns 409; close on a closed assignment
  returns 200.
- `cd e2e && npx playwright test --workers=1` (matches CI; parallel SQLite
  failures are lock noise).
- Manual: `make dev`, then confirm in the inbox that the support queue, the peer
  list, contact history, search, infinite scroll on each, and the widget
  round-trip all still work — all now hitting one endpoint with one cursor.
