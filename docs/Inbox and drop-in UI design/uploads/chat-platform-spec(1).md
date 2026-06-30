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
  re-auth + REST catch-up (`messages?after=`).
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

Base: `/v1`. All responses JSON. Errors: `{ "error": { "code", "message" } }` with standard status.

### 4.0 Who can create what

Conversations are untyped. The distinction is *who* may create one and whether an **assignment**
(support) is opened with it.

| action                                              | API key | user JWT      | admin session |
|-----------------------------------------------------|:-------:|:-------------:|:-------------:|
| create conversation (arbitrary members)             | ✓       | ✗             | ✗             |
| open support request (conversation + assignment)    | ✓       | ✓ (self only) | ✓             |
| add an assignment to an existing conversation       | ✓       | ✗             | ✓             |

Arbitrary multi-party conversations (driver↔client, dispatcher↔client) are host-backend-only. A
support request is just a conversation that gets an assignment, opened three ways: host backend
(§4.1), authed client from the SDK/web (§4.2), or agent from the inbox (§4.3). A **guest** request is
the same host-backend path (§4.1) with a generated user id — no separate mechanism. A user may have
many concurrent support requests — no dedupe.

### 4.1 Integration (API-key auth)

**Mint user token**
```
POST /v1/tokens
Authorization: Bearer sild_live_...
{ "user_id": "u_123", "ttl_seconds": 1800 }
→ 200 { "token": "<jwt>", "expires_at": "..." }
```

**Create conversation** — untyped; members + optional `reference`/`metadata`.
```
POST /v1/conversations
Authorization: Bearer sild_live_...
{
  "reference": "trip_8842",
  "metadata": { "kind": "ride" },          // host-defined, opaque to platform
  "members": [
    { "user_id": "u_client_1", "conv_role": "client",
      "metadata": { "phone": "+3725...", "app_version": "2.3.1", "role": "client" } },
    { "user_id": "u_driver_9", "conv_role": "driver",
      "metadata": { "phone": "+3725...", "app_version": "2.3.0", "role": "driver" } }
  ],
  "open_assignment": false                  // true → also queue for an agent (support)
}
→ 201 { "id": "c_abc", "status": "open", "members": [...] }
```
The host distinguishes its own chats via `metadata`/`reference`; the platform never reads them.
Set `open_assignment: true` to create a host-originated support request — conversation, members, and
assignment commit in **one transaction** (§1), so a failed assignment can't orphan the conversation.

**Manage members**
```
POST   /v1/conversations/:id/members   { "user_id", "conv_role" }      → 201
DELETE /v1/conversations/:id/members/:user_id                          → 204
       -- rejected (409) if it would leave an OPEN conversation with 0 members; close it instead
```

**Open / manage assignment** (support queue)
```
POST /v1/conversations/:id/assignments    → 201 { assignment }   -- queue this conversation
```

**Issue an upload URL** (direct-to-bucket; see §12)
```
POST /v1/uploads
Authorization: Bearer sild_live_...   (or user JWT — see §4.2)
{ "mime_type": "image/jpeg", "size_bytes": 220184, "filename": "photo.jpg" }
→ 201 { "object_key": "...", "upload_url": "<signed PUT>", "expires_at": "..." }
```

**Server-side message ingress** (bot / CRM / external agent writes into a conversation)
```
POST /v1/conversations/:id/messages
Authorization: Bearer sild_live_...
{ "body": "...", "sender_kind": "agent", "internal_actor_id": "agent_42",
  "attachments": [ { "object_key": "...", "disposition": "attachment" } ] }
→ 201 { message }
```

**Fetch conversation**
```
GET /v1/conversations/:id → 200 { id, status, reference, metadata, members[], assignment? }
```

### 4.2 User (JWT auth — native SDK + web)

```
GET  /v1/me/conversations
     → 200 [ { id, last_message, unread_count, members[], assignment? } ]

POST /v1/me/support-requests       -- client opens a support request (SDK / web)
     { "metadata": { } }           -- creates a conversation (sub as client) + queued assignment
     → 201 { conversation }        -- NOT deduped; a user may have many open at once

GET  /v1/conversations/:id/messages?before=<msg_id>&limit=50
     → 200 { messages[], has_more }
GET  /v1/conversations/:id/messages?after=<msg_id>          -- reconnect catch-up
     → 200 { messages[] }

POST /v1/conversations/:id/messages
     { "body": "...", "client_msg_id": "uuid",                 -- idempotency
       "attachments": [ { "object_key": "...", "disposition": "inline" } ] }
     → 201 { message }
     -- visibility defaults to "participants". Only agents (admin session / ingress) may set
     --   "visibility": "internal"; a user/guest token requesting internal → 403.
     -- archived messages: GET filters out visibility=internal for non-agent callers.

POST /v1/uploads                   -- see §4.1; also accepts a user JWT (scoped to the caller)

POST /v1/conversations/:id/read   { "last_read_message_id": "m_999" }     → 204
POST /v1/conversations/:id/typing                                         → 204  -- fans out a typing event

POST   /v1/me/push-tokens  { "platform": "ios", "token": "<device_token>" }   → 201
DELETE /v1/me/push-tokens   { "token": "<device_token>" }                    → 204
       -- token in body; deletion filtered by sub (never a guessable path param)

GET  /v1/conversations/:id        → 200 { conversation + members }
```
All user endpoints authorize against `conversation_members` for `sub`. Non-member → 403.

### 4.3 Admin (Google session)

```
GET  /v1/admin/auth/google            → redirect
GET  /v1/admin/auth/google/callback   → set session

GET  /v1/admin/assignments?status=queued&assignee=<id>     -- the inbox queue
POST /v1/admin/support-requests           { "external_user_id": "u_123", "metadata": { } }
     -- agent opens a support request with a user (own conversation + assignment)
POST /v1/admin/assignments/:id/claim      -- assign to calling agent
POST /v1/admin/assignments/:id/close
     -- agent sends via 4.2 POST messages (sender_kind=agent, internal_actor_id=<agent>)
     -- internal note: same call with "visibility": "internal" (never delivered to client; see §5.5)

GET  /v1/admin/search?q=<raw bar string>&before=&limit=
     -- ONE search bar, mixed tokens (like GitHub/Linear/Gmail). The backend tokenizes q:
     --   field:value tokens → structured filters (exact):
     --       status:open|closed   assignee:me|<id>   role:driver|client|…   channel:app|email
     --       phone:5512   app_version:2.3   meta.<key>:<value>  (any host metadata key)
     --   bare keywords (everything else) → PARTIAL trigram match, OR'd across BOTH
     --       messages.body AND member-metadata text (so typing just "5512" or "refund" works)
     --   all tokens AND together. unknown field: prefix → treated as a literal keyword, never errors.
     -- returns conversations + matching message snippets, ranked by trigram similarity.
     -- HOT data only; archived included only via the explicit deep-search mode (§12).
     -- (programmatic callers may also pass pre-split filters instead of packing them into q.)
```

**Search model.** Fixed fields (`status`, `assignee`, `role`, `channel`) map to columns. Member-metadata
search runs against the materialized `member_search_text` (GIN-trigram), built from each tenant's
**`searchable_metadata_keys`**; those keys get the index + UI autocomplete. Generic `meta.<key>:<value>`
always works as a slower live-jsonb fallback. A conversation matches if **any** member's metadata
matches (you're finding "the conversation with this phone number"). `me` in `assignee:me` resolves to
the calling agent.

```
POST   /v1/admin/api-keys   { "label" } → 201 { key }   -- shown once, secret
GET    /v1/admin/api-keys               → list (no secrets)
DELETE /v1/admin/api-keys/:id           → revoke

POST   /v1/admin/webhooks   { "url", "events": [...] } → 201 { id, secret }
GET    /v1/admin/webhooks
DELETE /v1/admin/webhooks/:id
```

### 4.4 Public
```
GET /.well-known/jwks.json
```

### 4.5 Guest support (no special keys)

A guest is just a user token whose `sub` is a host-generated id, minted by the host backend with the
secret key — there is no anonymous platform endpoint. The host backend generates an id, creates the
conversation + assignment, and mints a token (the §4.1 calls), then hands the token to the widget via
the normal `tokenProvider`. Refresh and reload-persistence are the host's job (it remembers the id in
its own session and re-mints); gating/abuse control lives at the host's token endpoint.

**Claim on login** — remap the generated id to the real user, preserving history:
```
POST /v1/conversations/:id/members/remap        (secret key)
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
| `conv:<conv_id>:internal` | agents only             | internal notes (§5.6) |

### 5.2 Subscriptions are membership-derived
On connect the backend validates the JWT, reads the user's memberships from Postgres, and attaches the
channel set server-side (Centrifuge server-side subscriptions): `user:<id>` plus `conv:<id>` for each
membership, plus `conv:<id>:internal` if the connection is an agent. The client declares nothing.
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
(re)connect: re-auth (fresh JWT via `tokenProvider`) → `GET /conversations/:id/messages?after=<last_seen>`
per conversation. Multi-node fan-out is handled by Centrifuge's Redis broker.

### 5.5 Push (offline delivery)
- SDK registers the device token on connect + OS rotation (`POST /me/push-tokens`), deregisters on
  logout (so a signed-out device can't receive the next user's messages).
- Fan-out only to members with **no live connection** (Centrifuge presence) — connected clients already
  got the event; no double-notify.
- Payload is a nudge: `{ conversation_id, message_id, preview?, unread_count }`. Body inclusion is a
  per-tenant flag. SDK `onPush` builds/suppresses the notification; tap → open → `after=` catch-up.
- Transport: FCM (Android/web) + APNs (iOS).

### 5.6 Internal notes — enforced by the channel split
A `visibility=internal` message is published to `conv:<id>:internal` only. Clients are never subscribed
to that channel, so an internal note **physically cannot reach them** — the privacy boundary is a
subscription fact, not UI logic. Internal notes are also never pushed, never emailed, and stripped from
history/search for non-agent callers.

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
// on tap → client.conversation(id).open() runs the after= catch-up automatically

// events (delegate / listener / flow)
onMessage(Message)              // Message.attachments[] = { objectKey, disposition, mimeType, url }
onRead(conversationId, userId, lastReadMessageId)
onMemberChange(conversationId, change)
onAssignmentUpdated(conversationId, status)
onTyping(conversationId, userId)
onConnectionStateChange(state)
```

Reconnect/catch-up is handled inside the SDK (subscriptions are server-side; the SDK just re-auths and
runs the `after=` fetch). Idempotency via
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
`/admin/search` (BigQuery latency/cost isn't inbox-interactive); it's a separate deep-search call. JSON
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
