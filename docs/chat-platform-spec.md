# Sild — Chat Platform Technical Spec

*Sild ("bridge" in Estonian) — a multi-tenant chat platform that bridges client, driver, dispatcher,
support, and email into one conversation primitive.*

Multi-tenant chat backend with native SDKs and a web drop-in. One **untyped** conversation primitive
serves every case — dispatcher↔client, client↔driver, client↔support. Support is not a type; it's any
conversation carrying an **assignment**. Conversations are created server-side; clients connect with
short-lived JWTs.

---

## 1. Invariants

- **Postgres is canonical; the socket is an egress-only nudge.** Server→client push only — clients
  write via REST and never publish/subscribe over the socket. No delivery guarantees: reconnect =
  re-auth + REST catch-up (`messages?since=`).
- **Conversations are untyped and created server-side.** Multi-party conversations come from the host
  backend (API key); clients/agents may only open a **support request** (a conversation with self +
  an assignment). Clients never add arbitrary members.
- **Two credential types:** API keys (server↔server, never reach a client) and user JWTs (clients).
- **Multi-tenant from row zero.** `tenant_id` on every row, key, and token.
- **Two RBAC layers:** platform roles (owner/admin/agent) and conversation roles
  (dispatcher/client/driver/agent).
- **Tenant is never client-supplied.** For user-JWT and admin routes it is the verified `tid` claim;
  for API-key routes it is bound to the key. Never read tenant from a path, header, or body — that's a
  cross-tenant leak. Every query is scoped by the resolved `tenant_id`.
- **Create operations are atomic.** Conversation + members + assignment (and guest create-and-mint,
  and email create-and-assign) happen in one transaction or none — no orphaned conversations.
- **Conversation lifecycle:** a delete that would leave an **open** conversation with zero members is
  rejected; an open conversation is ended by **closing** it. A closed conversation may have zero members
  and is archival-eligible. `closed` is terminal — no reopen; a new need is a new conversation/assignment.

---

## 2. Auth

### 2.1 API keys (integration, server↔server)
- Single secret key `sild_live_<random>`; stored hashed (argon2id or SHA-256), never retrievable after
  issue. Tenant-scoped. Sent as `Authorization: Bearer sild_live_...`. **Server-side only** — never ships
  in a client.
- Used for: minting user tokens (authed *and* guest), creating conversations/members, server-side
  message ingress, remapping members, webhook management.

### 2.2 User JWT (clients: native SDK + web)
- **JWS, asymmetric (ES256 or EdDSA).** Verified by anyone via JWKS. No JWE — the claims are not secret
  from the client that owns them.
- Claims:
```json
{
  "iss": "https://chat.sild.io",
  "sub": "<host_user_id>",
  "tid": "<tenant_id>",
  "typ": "user",
  "iat": 1730000000,
  "exp": 1730003600
}
```
- Short TTL (15–60 min). Authorization (which conversations, what role) is resolved **live** from the
  membership table on each request/subscribe — not encoded in the token.
- Rotate signing keys without redeploying clients via JWKS.
- A **guest** is just a user token whose `sub` is a host-generated id (UUID or `guest_`-prefixed). No
  special token type — the host backend mints it the same way via the secret key (§4.5).

### 2.3 Minting flow (the rule that shapes everything)
```
host end-user → host backend (holds API key) → POST /v1/tokens → user JWT → client connects
```
API key stays server-side. Native apps and the web widget obtain tokens through a host-provided
`tokenProvider` callback; they never see the API key.

### 2.4 Admin auth
Google OIDC → admin session (cookie). Separate identity space from chat end-users. Used by the
inbox UI only.

### 2.5 JWKS
`GET /.well-known/jwks.json` → public keys for JWT verification.

---

## 3. Data model

```
tenants            (id, name, searchable_metadata_keys, created_at)
                   -- searchable_metadata_keys: text[] of member-metadata keys to index + autocomplete
api_keys           (id, tenant_id, hash, label, revoked_at, created_at)
admin_users        (id, tenant_id, email, platform_role, created_at)   -- owner|admin|agent
webhook_endpoints  (id, tenant_id, url, secret, events[], active, created_at)

conversations      (id, tenant_id, reference, metadata, status, created_at)
                   -- NO type field. status: open|closed
                   -- reference: host object id (free-form). metadata: jsonb, host-defined
                   --   (host uses these to tell its own chats apart; platform stays agnostic)
conversation_members (conversation_id, member_kind, external_user_id, internal_actor_id,
                    conv_role, metadata, member_search_text, joined_at, left_at)
                   -- member_kind: user|agent|bot|email ; exactly one id column non-null
                   --   email = external party reachable by email; external_user_id holds the address
                   -- conv_role: dispatcher|client|driver|agent
                   -- metadata: jsonb, host-defined per-PARTICIPANT (phone, app_version, role,
                   --   guest:true …); a guest is just a user with a host-generated external_user_id
                   -- member_search_text: materialized concat of searchable_metadata_keys values,
                   --   refreshed on member write; GIN(gin_trgm_ops) on THIS (not live jsonb extraction)
assignments        (id, tenant_id, conversation_id, assignee_actor_id, status, created_at, closed_at)
                   -- status state machine: queued → assigned → closed ; assigned → queued (return to
                   --   queue) allowed. closed is TERMINAL (no reopen). assignee_actor_id null until claimed.
                   -- multiple assignments per user are fine (each is its own conversation)
messages           (id, tenant_id, conversation_id, sender_kind, visibility, channel,
                    external_user_id, internal_actor_id, body, created_at)
                   -- sender_kind: user|agent|bot|system ; exactly one id column non-null
                   -- visibility: participants | internal   (internal = agent-only note, never delivered out)
                   -- channel: app | email  (how it entered/left; app = WS/SDK, email = mail connector)
                   -- body search is PARTIAL/substring: GIN(body gin_trgm_ops) via pg_trgm
                   --   (trigram, not tsvector — matches infixes, no language/normalization config)
message_attachments (id, message_id, disposition, object_key, mime_type, size_bytes, filename)
                   -- disposition: inline|attachment  (inline = render in body, attachment = listed)
                   -- object_key points into the configured bucket (GCS or S3)
email_threads      (conversation_id, tenant_id, thread_token, last_address, last_message_id)
                   -- maps inbound replies back to a conversation via a token in subject/reply-to
read_receipts      (conversation_id, participant_kind, external_user_id, internal_actor_id,
                    last_read_message_id, updated_at)  -- upsert, 1 per participant
                   -- MONOTONIC: ignore a last_read_message_id older than the stored one (out-of-order
                   --   or duplicate POSTs are normal) — upsert with a GREATEST guard
push_tokens        (id, tenant_id, member_kind, external_user_id, internal_actor_id,
                    platform, token, updated_at)   -- platform: ios|android|web
                   -- deregistered on logout; one user may have many devices
conversation_archives (conversation_id, tenant_id, sink, sink_ref, message_count,
                    archived_at)   -- tombstone: hot rows gone. sink: bigquery|gcs_json|s3_json
                                   -- sink_ref: BigQuery row key / table coord, or bucket object key
```

`reference` ties a conversation to a host-side object (trip_id, order_id). Indexes:
`(tenant_id, conversation_id, id)` for history pagination; `assignments(tenant_id, status, assignee_actor_id)`
for the inbox queue.

**Identity namespaces are kept in separate columns and never collide.** `external_user_id` =
host's user namespace (clients, drivers, dispatchers, **and guests** — a guest is just a host-generated
id in this same space). `internal_actor_id` = our namespace (`agent` → `admin_users.id`; `bot`/`system`
→ reserved synthetic ids per tenant). Every member, message, and receipt sets exactly one of the two.
The host owns its id space and is responsible for keeping guest ids distinct from real ones.

**Metadata is two-layer, both host-defined and opaque to the platform.**
`conversations.metadata` = conversation-level facts. `conversation_members.metadata` = per-participant
facts the host attaches when adding the member (e.g. `phone`, `app_version`, `role: "driver"`). The
inbox renders member metadata in the agent's member panel; the platform never interprets either.

---

## 4. REST API

Base: `/v1`. All responses JSON. Errors:
`{ "error": { "code", "message", "request_id", "fields"? } }` with standard
status. `code` is API surface and SDKs branch on it; `message` is for humans and
may be reworded freely.

**Paths name the data model, not the consumer.** The credential decides which
subset of a resource you see, never which URL you call. The one exception is
`/v1/admin/auth/*` — obtaining a credential genuinely differs per consumer.

### 4.0 The list contract

Every collection endpoint takes `?limit=` and `?cursor=` and returns:

```
{ "items": [ … ], "next_cursor": "<opaque>|null", "has_more": bool }
```

`next_cursor` is null exactly when `has_more` is false. The cursor is **opaque**:
round-trip it verbatim. It is bound to the resource, sort key, direction and
filter set, so replaying one against a different query is a 400, not a wrong
page. Limits clamp to [1,100] rather than erroring.

Ordering by `last_activity` is **live-view**, not snapshot: the key is mutable,
so rows can move between pages while you scroll. Deduplicate by id and refresh
page one on a realtime reorder.

### 4.1 Who can create what

Conversations are untyped. `kind` is derived from `open_assignment` at creation:
support when an assignment opens, peer otherwise. It is set once and never
re-derived.

| action | API key | user JWT | admin session |
|---|:---:|:---:|:---:|
| open a support conversation | ✓ | ✓ (self only) | ✓ (names one user) |
| create a **peer** conversation | ✓ | ✗ | ✗ |
| arbitrary members | ✓ | ✗ | ✗ |

Peer creation is server-to-server only: a peer conversation is visible to every
`peer_access` operator, so letting a user mint one would be a way to inject rows
into the operator peer inbox. A non-key principal sending
`open_assignment: false` gets **403**, never a silent upgrade.

### 4.2 `conversations`

```
GET /v1/conversations
    ?kind=support|peer                ?participant=me|<external_user_id>
    ?assignee=me|<actor_id>|none      ?status=open|closed
    ?assignment_status=queued|assigned|closed
    ?q=<mixed-token search bar string>    ?role=<conv_role>
    ?sort=last_activity|created|waiting_since   ?order=asc|desc
    ?limit=  ?cursor=
 → { items[], next_cursor, has_more, counts?: { open, you, unassigned, closed } }
```

This one endpoint backs the support queue, the peer inbox, a user's own
conversations, a contact's history and search. `counts` is emitted only for the
operator support queue.

**Two state machines, two filters.** `status` is the CONVERSATION lifecycle
(`open|closed`, terminal); `assignment_status` is the assignment's
(`queued → assigned → closed`, with `assigned → queued` legal). "Closed" in the
inbox has always meant the *conversation* is closed. `assignment_status` is only
meaningful alongside `kind=support`.

`sort=waiting_since` requires `kind=support` (400 otherwise): the key comes from
the assignment, and a keyset over a NULL-bearing key is undefined.

Rows returned for a `?q=` query carry a `snippet` — the matching fragment.
Without it a hit in an older message renders with an unrelated preview.

```
POST   /v1/conversations              -- create; see §4.1
GET    /v1/conversations/:id
GET    /v1/conversations/:id/messages ?cursor= (page) | ?since= (catch-up)
POST   /v1/conversations/:id/messages
POST   /v1/conversations/:id/read     { "last_read_message_id" }   → 204
POST   /v1/conversations/:id/typing                                → 204
POST   /v1/conversations/:id/close
POST   /v1/conversations/:id/assignments
POST   /v1/conversations/:id/members                  (API key)
DELETE /v1/conversations/:id/members/:user_id         (API key)
POST   /v1/conversations/:id/members/remap            (API key, guest claim)
```

An operator sending into a **peer** conversation implicitly joins it (adds the
agent participant + a system join-note). That fires on the shared send route
because it is a property of the data — operator + peer conversation — not of the
URL the client chose.

### 4.3 `assignments`

```
PATCH /v1/assignments/:id   { "assignee_actor_id": "me" } | { "status": "closed" }
```

Deliberately narrow. `assignee_actor_id` accepts only `"me"`: claiming for
another operator has no route, and accepting an arbitrary id would quietly
introduce reassignment. `status` accepts only `"closed"`. Both fields at once is
a 400.

### 4.4 `contacts`

```
GET /v1/contacts?q=&limit=&cursor=
GET /v1/contacts/:external_user_id
```

Contacts and conversations are both searchable, as separate resources — a contact
search returns people, a conversation search returns threads. A contact's history
is `GET /v1/conversations?participant=<id>`, so there is no
`/contacts/:id/conversations`.

A contact is a **read model** over `conversation_members`, keyed
`(tenant_id, external_user_id)`. Projection rules:

- **Metadata**: the most recently joined *authorized* membership wins whole; keys
  are never unioned across memberships. Merging would fabricate a person who
  never existed and make the result order-dependent.
- **Existence**: any authorized membership, in any state (left, closed,
  archived).
- **`conversation_count`**: current memberships only.
- **`last_activity`**: all authorized memberships. So an archived-only contact
  stays findable with a count of zero.
- The scope is applied **before** aggregation. Two operators may legitimately see
  different counts for the same contact; that is the scope working.

### 4.5 `brands`

```
GET /v1/brands/active?app_id=   -- any principal, or none
GET /v1/brands                  -- owner/admin
PUT /v1/brands                  -- owner/admin
```

`/v1/brands` is a singleton configuration **aggregate**, not a collection: `PUT`
replaces the whole set atomically and `active_brand_id` is a property of the set,
so it is exempt from the list contract.

`GET /v1/brands/active` is public when keyed by `app_id` and tenant-scoped when a
credential is present. Credential first; an authenticated caller's `app_id` is
ignored. Without a credential `app_id` is **required** — a missing one is a 400,
never a sole-tenant guess, or the response would depend on how many tenants the
deployment happens to hold. A credential that is present but **invalid** is a 401 —
never a silent downgrade to the public form, which would turn an expired session
into a quiet tenant switch. Public responses are `Cache-Control: public, max-age=60`;
authenticated ones are `private, no-store`, both with
`Vary: Authorization, Cookie`.

### 4.6 Identity, settings, integration

```
GET    /v1/principal        -- who am I, for any credential (see below)
GET    /v1/realtime/token   -- admin session
POST   /v1/tokens           -- API key: mint a user JWT
POST   /v1/uploads          -- any principal: signed direct-to-bucket grant
POST   /v1/push-tokens      { "platform", "token" }   → 201   (user JWT)
DELETE /v1/push-tokens      { "token" }               → 204   (user JWT)

GET/POST/DELETE /v1/api-keys[/:id]          owner/admin
GET/POST/PATCH/DELETE /v1/webhooks[/:id]    owner/admin
GET    /v1/webhooks/:id/deliveries          owner/admin
GET/PATCH /v1/channels/email                owner/admin
GET/POST /v1/team, PATCH /v1/team/:id, POST /v1/team/:id/password
```

`GET /v1/principal` returns a discriminated identity plus **effective grants**:

```json
{ "kind": "admin", "tenant_id": "t_…",
  "subject": { "id": "adm_…", "role": "agent" },
  "grants": [ { "action": "conversations.list",
                "scope": { "kinds": ["support"], "requires_assignment": true } } ] }
```

Grants carry the **scope** each action is held over, not just its name: an agent
with `peer_access` and one without both hold `conversations.list`, and only the
scope tells them apart. Frontends render affordances from this rather than
re-deriving policy from role flags.

### 4.7 Public

```
GET /.well-known/jwks.json
GET /widget.js
POST /v1/email/inbound          -- signature is the gate
PUT/GET /v1/uploads/local/*     -- signed capability (HMAC + expiry + object key)
GET /v1/admin/auth/{google,google/callback,password,logout}
```

Local upload URLs are **signed capabilities**, not addresses: the signature binds
the verb, the object key and an expiry, and the tenant is the key's first
segment. Without it the URL would be a permanent bearer token for any guessable
key.

### 4.8 Guest support (no special keys)

A guest is just a user token whose `sub` is a host-generated id, minted by the
host backend with the secret key — there is no anonymous platform endpoint.
Claim on login remaps the generated id to the real user, preserving history:

```
POST /v1/conversations/:id/members/remap        (API key)
{ "from_user_id": "guest_7f3a", "to_user_id": "u_123" }   → 200
```

---

## 5. Realtime (egress-only)

Built on the **Centrifuge** library embedded in the backend. The socket is **server→client only**:
clients never publish or subscribe over it. All writes go over REST (§4); this layer only pushes events.

Connect: `wss://chat.sild.io/v1/ws?token=<jwt>`. (SSE is available for the web widget; native uses WS.)

### 5.1 Channels
| channel | subscribers | carries |
|---------|-------------|---------|
| `user:<user_id>`          | that user's connections | user-targeted events (added to conversation, assignment updates) |
| `conv:<conv_id>`          | all members             | messages, receipts, typing, member changes |
| `agents:<tenant_id>`      | every operator          | the same events for every support conversation an assignment makes readable, plus queue changes |
| `peer:<tenant_id>`        | peer_access operators   | the same events for every peer conversation |

End users subscribe per conversation, which is bounded by their own membership.
Operators subscribe per **tenant**: an inbox watching one channel per assigned
conversation would hold thousands of subscriptions and rebuild them on every
reconnect. The two tenant channels carry what per-conversation channels carry, so
an operator's subscription set is fixed and a conversation created after connect
needs no re-subscription.

Which tenant channel a conversation's events reach is decided by the same
classification REST authorizes with (`ClassifyAgentAccess`): peer conversations go
to `peer:<tenant_id>`, support conversations to `agents:<tenant_id>`. Both tenant
channels reach every operator holding them, so a conversation no operator may read
reaches neither — publishing it would hand out what REST refuses.

### 5.2 Subscriptions are membership-derived
On connect the backend validates the JWT and attaches the channel set server-side (Centrifuge
server-side subscriptions). The client declares nothing. A user connection gets `user:<id>` plus
`conv:<id>` for each membership, read from Postgres. An operator connection gets `user:<admin_id>`,
`agents:<tenant>`, and `peer:<tenant>` when `policy.Scope` admits peer conversations — a fixed set that
does not depend on the queue.
Membership changes mid-connection call `node.Subscribe/Unsubscribe(user, channel)` — cluster-wide via
the Redis broker.

### 5.3 Events (published by REST handlers after the Postgres write)
Envelope: `{ "type": "...", "conversation_id": "c_abc", "data": { }, "ts": 1730000000 }`
| type | data |
|------|------|
| `message.created`     | full message object (incl. `attachments[]`)    |
| `message.read`        | `{ user_id, last_read_message_id }`            |
| `member.added` / `member.removed` | `{ user_id, conv_role? }`          |
| `assignment.updated`  | `{ assignment_id, status, assignee_actor_id }` |
| `conversation.closed` | `{}`                                           |
| `typing`              | `{ user_id }`  — server-throttled to 1 per user per conversation per ~3s |

### 5.4 Reconnect & catch-up (the only correctness mechanism)
The socket guarantees nothing — a missed event is invisible until reconnect. The SDK MUST, on every
(re)connect, in this order: re-auth (fresh JWT via `tokenProvider`) → re-fetch
`GET /v1/conversations` (to pick up conversations added while offline) →
`GET /v1/conversations/:id/messages?since=<last_seen>` for **each conversation whose messages the
SDK is holding**. Multi-node fan-out is handled by Centrifuge's Redis broker.

Catch-up repairs client state; it is not a history fetch. A conversation the SDK holds no messages
for has no `<last_seen>` to resume from, and its refreshed list row already carries the current
preview, activity and unread count — so nothing is dropped by skipping it. Opening such a
conversation later fetches its thread the normal way, which is bounded by `limit` exactly as it
would be with no outage at all: reading further back is pagination (`?cursor=`), a separate
mechanism from catch-up.

**`?since=` is a sync read, not a page.** It returns everything after a message id,
oldest-first, bounded by `limit`, in the standard envelope with `next_cursor: null`.
While `has_more` is true the client MUST re-issue with **the id of the last item it
received** — the message id is the position, and re-sending the original `since`
loops on the same page forever. This is normative: the read is bounded, so a client
that ignores `has_more` silently loses history, which is exactly what the old
unbounded-looking `after=` did at its hidden 500-row cap.

### 5.5 Push (offline delivery)
- SDK registers the device token on connect + OS rotation (`POST /v1/push-tokens`), deregisters on
  logout (so a signed-out device can't receive the next user's messages).
- Fan-out only to members with **no live connection** (Centrifuge presence) — connected clients already
  got the event; no double-notify.
- Payload is a nudge: `{ conversation_id, message_id, preview?, unread_count }`. Body inclusion is a
  per-tenant flag. SDK `onPush` builds/suppresses the notification; tap → open → `since=` catch-up.
- Transport: FCM (Android/web) + APNs (iOS).

### 5.6 Internal notes — enforced by the channel split
A `visibility=internal` message is published only to the tenant channels operators watch, and to no
client channel at all. Clients are never subscribed there, so an internal note **physically cannot
reach them** — the privacy boundary is a subscription fact, not UI logic. In a conversation no
operator may observe yet it reaches nobody live; history is the catch-up path (§5.4). Internal notes
are also never pushed, never emailed, and stripped from history/search for non-agent callers.

---

## 6. Connectors

### 6.1 Webhooks (outbound)
POST to registered `url`. Headers `X-Signature: sha256=<hmac(secret, raw_body)>` and
`X-Sild-Event-Id: <uuid>` (stable across retries — consumers dedupe on it).
Retries with exponential backoff (e.g. 1m, 5m, 30m, 2h, 6h), delivery log per attempt.

Events: `conversation.created`, `message.created`, `member.added`, `member.removed`,
`assignment.created`, `assignment.updated`, `conversation.closed`.
```json
{ "event": "message.created", "tenant_id": "t_1", "ts": 1730000000,
  "data": { "conversation_id": "c_abc", "message": { ... } } }
```

### 6.2 Email channel (in + out)
Email is just another way a message enters/leaves a conversation. The conversation primitive is
unchanged; the email party is an `email` member (address = `external_user_id`), and agents answer in
the inbox without thinking about email.

**Inbound** (provider inbound-parse webhook — SendGrid / Postmark / Mailgun):
```
POST /v1/email/inbound        -- provider posts the parsed email here
  → VERIFY provider signature (required gate). Reject unsigned/invalid.
  → recipient domain must be in the tenant's allowlist; per-tenant rate limit. Else drop.
  → resolve thread: extract thread_token from subject / reply-to (email_threads.thread_token)
     - token found  → append message (channel=email, sender_kind=user, visibility=participants)
     - no token     → create conversation + email member + queued assignment (one transaction)
  → attachments uploaded to the bucket, linked as message_attachments
```

**Outbound:** when an agent posts a `visibility=participants` message to a conversation with an `email`
member, the mail worker sends it to that address with `thread_token` in the subject and `Reply-To`.
Attachments are forwarded (re-fetched from the bucket) up to the provider cap (~10–25 MB total); over
the cap, a signed download link is sent instead. `visibility=internal` is never emailed (§5.6).

**Config** (per tenant): inbound address/domain, provider + signing secret, from-name/from-address.
The platform stores no mailboxes — it relies on the provider for transport.

---

## 7. RBAC

**Platform roles** (admin_users): guard API/inbox.
| role  | api keys | webhooks | all conversations | support inbox |
|-------|:--------:|:--------:|:-----------------:|:-------------:|
| owner | ✓        | ✓        | ✓                 | ✓             |
| admin | ✓        | ✓        | ✓                 | ✓             |
| agent | –        | –        | –                 | ✓             |

**Conversation roles** (conversation_members): guard send/read within a conversation. Membership is
the authorization check for every user endpoint and every realtime channel (subscriptions are resolved
server-side from membership, §5.2).

---

## 8. Admin / Support inbox (Phase 2, React web)

The inbox is the **assignment queue**, not a conversation type. It lists conversations that have an
assignment.

| screen              | contents                                                                 |
|---------------------|--------------------------------------------------------------------------|
| Login               | Google OIDC                                                               |
| Inbox list          | assignments filtered by status (queued/assigned/closed) + assignee; unread badges; **open new support request with a user** |
| Search              | one bar, mixed tokens: `status:`/`assignee:`/`role:`/`channel:`/`meta.*:` qualifiers + free keywords (partial match on body & party metadata); hot data only |
| Conversation view   | transcript (live via WS as agent), composer + attachments, **internal-note toggle** (agent-only), member panel showing per-member metadata, claim + close; email threads render inline |
| Settings → API keys | issue (shown once), list, revoke                                         |
| Settings → Webhooks | add (url + events), list, delete, delivery log                           |
| Settings → Team     | invite agents, set platform role                                         |

Defer: routing rules, SLA, canned responses, analytics.

---

## 9. Web drop-in (Phase 3 — ships first, for fastest validation)

Built before native so the full loop (open request → agent answers → attachments) can be validated in
a browser without app-store cycles. Same capabilities as native, identical auth model, two
distribution modes.

**Script tag (WordPress, arbitrary sites)** — self-contained bundle, shadow DOM for style isolation:
```html
<script src="https://chat.sild.io/widget.js"></script>
<script>
  Sild.init({
    // REQUIRED: the tenant's app id, for the unauthenticated brand load. Surfaced in
    // the inbox under Settings → Installation; grants no access on its own.
    appId: 'app_...',
    // tokenProvider hits the host's endpoint, which mints a user token via the secret key.
    // For a guest, that endpoint mints a token for a host-generated id — same call, no special key.
    tokenProvider: () => fetch('/wp-json/sild/token').then(r => r.json()).then(d => d.token),
    conversationId: 'c_abc'   // optional; omit to show conversation list / open a support request
  });
</script>
```

**React (SPA)**:
```jsx
import { SildChat } from '@sild/react';
<SildChat tokenProvider={getToken} conversationId="c_abc" />
```

**WordPress plugin**: stores the API key server-side in WP, exposes `/wp-json/sild/token`
that mints a user JWT for the logged-in WP user, enqueues `widget.js`. Browser never sees the API key.

**Guest UX:** a guest token is scoped to its own conversation(s) and must open **directly to the thread**
— omitting `conversationId` to show the conversation-list view is an **authed-only** affordance. The
widget requires a resolved conversation for guest tokens.

---

## 10. Native SDK surface (Phase 4 — Swift / Kotlin)

Language-neutral signatures; mirror idiomatically per platform. Same API/WS contract the web widget
already proved out.

```
// init — tokenProvider mints the user JWT via the host backend (secret key).
// A guest is the same: the host endpoint mints a token for a host-generated id. No guest-specific API.
SildClient(config: {
  baseUrl: String,
  tokenProvider: async () -> String
})

client.connect()
client.disconnect()
client.connectionState            // disconnected | connecting | connected

// conversations
client.conversations.list() -> [ConversationSummary]
client.openSupportRequest(metadata: Map?) -> Conversation   // user-initiated; many allowed, no dedupe
let conv = client.conversation(id)

// uploads + messages
client.upload(data: Bytes, mimeType: String, filename: String) -> ObjectKey  // signed PUT to bucket
conv.messages(before: msgId?, limit: Int) -> [Message]
conv.send(text: String,
          attachments: [{ objectKey, disposition }]?,   // disposition: inline | attachment
          clientMsgId: String) -> Message
conv.markRead(messageId: String)
conv.sendTyping()

// push (SDK registers + handles)
client.registerPush(token: String, platform: ios|android)   // call on connect + token rotation
client.deregisterPush(token: String)                         // call on logout
client.onPush(handler: (PushPayload) -> Notification?)       // build/customize notification; nil = suppress
// PushPayload = { conversationId, messageId, preview?, unreadCount }
// on tap → client.conversation(id).open() runs the since= catch-up automatically

// events (delegate / listener / flow)
onMessage(Message)              // Message.attachments[] = { objectKey, disposition, mimeType, url }
onRead(conversationId, userId, lastReadMessageId)
onMemberChange(conversationId, change)
onAssignmentUpdated(conversationId, status)
onTyping(conversationId, userId)
onConnectionStateChange(state)
```

Reconnect/catch-up is handled inside the SDK (subscriptions are server-side; the SDK just re-auths and
runs the `since=` fetch). Idempotency via
`clientMsgId`.

**Rollout:** the SDK's first target is the **support client** (open request → agent answers), mirroring
the web widget. Generalizing to driver↔client and dispatcher↔client adds no transport — those are just
conversations the host backend created the SDK connects into.

---

## 11. Storage (attachments)

Bucket backend is chosen at deploy time via config — **GCS or S3** — behind one interface.
```
STORAGE_BACKEND = gcs | s3
STORAGE_BUCKET  = <name>
STORAGE_REGION  = <region>        # s3
# credentials via workload identity (GCS) / IAM role (S3); no static keys in app
```
- Clients **upload direct to the bucket** via a signed PUT URL (`POST /v1/uploads`) — bytes never
  transit the chat backend. The response `object_key` is what goes into `message.attachments[]`.
- Download via signed GET URLs minted per request (or CDN for public-read tenants), resolved when a
  message is read. `message_attachments.object_key` is the only stored reference.
- `disposition` is a render hint, not storage: `inline` → show in the message body; `attachment` →
  list below it. Both point at the same object.
- Enforce `mime_type`/`size_bytes` limits at upload-URL issuance. Default **10 MB/file**, per-tenant
  override (`max_attachment_bytes`).

---

## 12. Hot/cold archival (pluggable sink)

Keep only **active** data in Postgres. Closed, idle conversations move to an **archive sink**, chosen
per deployment. Archival is a *channel* with one interface so destinations drop in later:
```
Sink {
  write(conversation) -> sink_ref      // durably persist; return a locator
  read(sink_ref)       -> conversation // rehydrate for fallback reads / restore
}
ARCHIVE_SINK = bigquery | gcs_json | s3_json     # first impl: bigquery
ARCHIVE_IDLE_DAYS = 30
```
`sink_ref` shape per backend: **bigquery** → `dataset.table` + partition + conversation_id;
**gcs_json / s3_json** → bucket object key.

**Eligibility:** `status = closed` (assignment closed or host-closed) AND idle past `ARCHIVE_IDLE_DAYS`.

**Job (background, batched):**
1. Serialize the whole conversation — row + members + messages + attachment manifest (object_keys,
   not bytes).
2. Hand it to the configured sink's `write()`:
   - `bigquery` → insert flat rows into partitioned tables (`conversations`, `messages`, `members`),
     returns the table coordinate / row key as `sink_ref`. **Queryable.**
   - `gcs_json` / `s3_json` → `PUT archive/{tenant}/{conversation}.json`, returns the object key. Not
     queryable.
3. In one transaction, after the sink confirms the write: delete `messages`, `message_attachments`,
   `read_receipts`, `assignments`, `conversation_members`, the `conversation` row; insert the
   `conversation_archives` tombstone (`sink`, `sink_ref`). **Write-then-delete, verified — never the reverse.**
4. Attachment objects stay in the bucket (referenced by the archive). Optionally move to a colder class.

**Read fallback:** `GET /v1/conversations/:id/messages` checks hot rows first; on miss, reads from the
sink via the tombstone (BigQuery query, or stream the JSON). Optional `?restore=true` rehydrates to hot.
Rare, since the inbox only touches open conversations.

**Search-all-history:** with the **bigquery** sink the archive is queryable — an optional "include
archived" mode can union hot trigram results with a BigQuery query. Kept **off** the default
`GET /v1/conversations?q=` (BigQuery latency/cost isn't inbox-interactive); it's a separate deep-search call. JSON
sinks leave archived data unsearchable — the accepted tradeoff for the cheaper destination.

---

## 13. Phase map

| phase | deliverable                              | depends on            |
|-------|------------------------------------------|-----------------------|
| 1     | Backend: auth, REST, realtime (Centrifuge, egress-only), push fan-out, webhooks, RBAC, JWKS, uploads, search, internal notes | — |
| 2     | Admin/support inbox = assignment queue + search + internal notes (React) | §4.3, §5, §8    |
| 3     | **Web drop-in** (script + React + WP) — validation target | §4.2, §5, §9 |
| 4     | Native SDKs (Swift, Kotlin) — push register/handle | §10 (web contract proven) |
| 5     | Email connector (inbound parse → thread, outbound reply) | §6.2 |
| 6     | Archival job → pluggable sink (BigQuery first) | §12; can land anytime after §1 |

**Locked before phase-1 code:** token model (JWS + single secret API key, asymmetric, JWKS); tenancy
in schema; untyped conversations + assignment-as-support; two identity namespaces (external/internal,
exactly one set; guests are host-generated external ids; email party in external space); two-layer
metadata (conversation + per-member); message `visibility` (participants/internal) enforced by the
realtime channel split (egress-only Centrifuge: `user:` / `conv:` / `conv::internal`); `channel`
(app/email); **partial trigram search** (pg_trgm on body + member-metadata text, no normalization);
pluggable archive sink behind one contract; the §4/§5 contract (all clients consume it); two-layer
RBAC; read receipts from message one; storage backend abstraction; push fan-out keyed on connection
presence.
