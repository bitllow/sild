# Test suite — spec traceability

How the test suite maps onto the spec (`docs/chat-platform-spec.md`). Default runs
on SQLite (zero infra); cross-dialect runs add Postgres/MySQL.

```
make test                                   # whole suite on SQLite
docker compose up -d postgres               # for the cross-dialect run
docker compose --profile test up -d mysql   # row locks the SQLite path cannot exercise
SILD_TEST_POSTGRES_DSN="host=localhost port=5433 user=sild password=sild dbname=sild sslmode=disable" \
SILD_TEST_MYSQL_DSN="sild:sild@tcp(127.0.0.1:3307)/sild?charset=utf8mb4&parseTime=True&loc=Local" \
  go test ./internal/store/gormstore/       # migration, pg_trgm/GIN, SKIP LOCKED
```

| Spec area | What's asserted | Test |
|---|---|---|
| §1 atomic create | conversation + members + assignment commit together | `api/invariants_test.go: TestCreateConversationAtomicWithAssignment` |
| §1 last-member rule | removing the last member of an OPEN conv → 409 | `…: TestRemoveLastMemberRejected` |
| §1 closed terminal | close is idempotent; closed conv may go empty | `…: TestCloseThenRemoveAllowed` |
| §1 tenant isolation | tenant B can't read tenant A's conversation | `…: TestCrossTenantIsolation` |
| §1/§3 tenant_id everywhere | `conversation_members`/`message_attachments`/`read_receipts` carry tenant_id; portable + idempotent migrate | `store/gormstore/migrate_test.go` |
| §2.1 API keys | invalid key rejected; SHA-256 prefix verify round-trip | `api/auth_rbac_test.go: TestInvalidAPIKeyRejected`, `auth/auth_test.go` |
| §2.2 user JWT | ES256 mint/verify; tenant from `tid` | `auth/*`, end-to-end in `api/flow_test.go` |
| §2.5 JWKS | public keys served | `api/auth_rbac_test.go: TestJWKSEndpoint` |
| §4.0 who-creates-what | user JWT cannot create arbitrary conversations | `…: TestUserCannotCreateConversation` |
| §4.2 support request | client opens self request (+ queued assignment) | `api/flow_test.go` |
| §4.2 idempotency | repeat `client_msg_id` returns same message | `api/flow_test.go` |
| §4.2 pagination | `before=`/`limit` with `has_more` | `api/messages_test.go: TestPaginationBeforeHasMore` |
| §5.4 catch-up | `after=` returns messages missed while offline | `api/flow_test.go` |
| §5.1 channel split | participants → conv + user channels | `realtime/centrifuge_test.go: TestPublishParticipantsChannels` |
| §5.1 operator fan-out | an operator's subscription set is fixed (never per-conversation); a support conversation reaches the agents channel, a peer one only the peer channel | `realtime/parity_test.go: TestChannelSetDoesNotGrowWithTheQueue`, `domain/agent_fanout_test.go` |
| §5.6 internal notes | published only to agent tenant channels, never a conversation or user channel, and nowhere at all when no operator may observe the conversation; stripped from client history; user can't set internal | `realtime/…: TestPublishInternalChannelOnly`, `TestPublishInternalWithoutObserversGoesNowhere`, `api/messages_test.go: TestInternalNoteIsolation`, `TestUserCannotPostInternal` |
| §5.5 push fan-out | only offline members, never the sender | `push/fanout_test.go` |
| §6.1 webhooks | HMAC `X-Signature`, stable `X-Sild-Event-Id`, delivery log, backoff retry on failure | `connector/webhook/relay_test.go` |
| §6.2 email | inbound creates/threads by token; agent reply emails out; signature gate | `domain/email_test.go` |
| §4.3 search | keyword (body + member metadata) + `status:`/`role:` filters, AND'd | `domain/search_test.go` |
| §7 platform RBAC | agent can't manage API keys; owner can | `api/auth_rbac_test.go: TestPlatformRoleGuardsAPIKeys` |
| §7 conversation RBAC | non-member → 403 | `…: TestNonMemberForbidden` |
| §11 uploads | size cap enforced; ownership record; attachments validated against completed uploads | exercised via `domain.IssueUpload` + `SendMessage` attachment resolution |
| §12 archival | write-then-delete (verified), tombstone + membership snapshot, sink rehydrate; OPEN never archived | `archive/job_test.go` |

## Horizontal scalability

Guards that only matter with more than one replica, and the test that proves each
still holds. Every concurrency case runs on SQLite for speed and again on real
row contention when `SILD_TEST_POSTGRES_DSN` / `SILD_TEST_MYSQL_DSN` are set. CI
runs all three.

| Guard | What's asserted | Test |
|---|---|---|
| Assignment claim is not a lost update | two agents claim one assignment → exactly one wins; the loser gets 409 `assignment_already_taken`; a closed one gets 409 `assignment_already_closed`; close stays idempotent | `store/gormstore/assignment_guard_test.go`, `api/assignment_claim_test.go` |
| `client_msg_id` survives a concurrent retry | parallel sends with one key all return the same message, one row | `domain/idempotency_race_test.go: TestConcurrentSendsWithOneClientMsgID` |
| Outbox events are claimed, not just read | a second relay claims nothing; concurrent relays never overlap; a lapsed lock frees the row for the next pass; a renewed one does not; renewal reports a claim another relay took; reschedule releases. The claim lives in `claim_token`/`locked_until`, not a new `status`, so a relay from the previous release still sees the rows (ARCHITECTURE §4) | `store/gormstore/outbox_claim_test.go`, `connector/webhook/relay_race_test.go` |
| Concurrent relays each get a page | four relays claiming at once every one comes back with a full page instead of losing its candidates to a rival. Needs `FOR UPDATE SKIP LOCKED`, so postgres/mysql only — set the DSN or the case is skipped | `store/gormstore/outbox_claim_test.go: TestConcurrentRelaysEachGetAPage` |
| Job leases are exclusive | one holder at a time; expiry allows takeover; release is owner-scoped; `RunExclusive` bodies never overlap | `store/gormstore/lease_test.go` |
| A leased run keeps its lease, or stops | the heartbeat carries a run past the TTL; a second worker skips rather than queues; losing the lease cancels the work and reports `ErrLeaseLost`; a database that stops answering still gives the lease up on schedule instead of running on unleased | `store/gormstore/lease_test.go`, `store/lease_hang_test.go` |
| The archive sweep has one runner | a worker skips the tick while another holds the lease, and releases it after | `archive/sweep_test.go` |
| Signing-key bootstrap mints one key | concurrent cold start across replicas → one key; a waiter still runs the work when the lease holder failed; a broken key lookup is an error, not "no key" | `auth/bootstrap_test.go` |
| Inbound email is deduped | redelivery of one Message-ID does not open a second conversation; a released claim lets a retry through; a claim abandoned by a crashed process goes stale and the retry still lands; an in-flight claim is reported as in-flight (never as done) so the sender is asked to retry; a superseded attempt cannot complete or drop its successor's claim | `domain/email_dedupe_test.go` |
| Production refuses single-node config | memory broker / SQLite / unsigned local storage rejected at boot; development unaffected | `config/config_test.go` |

Not covered by a test, by decision: the per-IP rate limiter
(`middleware/ratelimit.go`) is in-process, so N replicas allow N× the configured
rate. It is a brute-force blunter, not a distributed quota — see
`tasks/HORIZONTAL-SCALABILITY-PLAN.md`.

## Conventions

- `internal/testutil` builds an in-process backend (real store + services + gin)
  over a temp SQLite file, with a capturing realtime publisher and mailer so
  egress (events, email) is assertable without infra.
- Each test gets an isolated database (temp file), so tests are independent.
