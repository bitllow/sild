# Test suite — spec traceability

How the test suite maps onto the spec (`docs/chat-platform-spec.md`). Default runs
on SQLite (zero infra); cross-dialect runs add Postgres/MySQL.

```
make test                                   # whole suite on SQLite
docker compose up -d postgres                # for the cross-dialect run
SILD_TEST_POSTGRES_DSN="host=localhost port=5432 user=sild password=sild dbname=sild sslmode=disable" \
  go test ./internal/store/gormstore/       # migration + pg_trgm/GIN on real Postgres
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
| §5.6 internal notes | published only to `conv:<id>:internal`; stripped from client history; user can't set internal | `realtime/…: TestPublishInternalChannelOnly`, `api/messages_test.go: TestInternalNoteIsolation`, `TestUserCannotPostInternal` |
| §5.5 push fan-out | only offline members, never the sender | `push/fanout_test.go` |
| §6.1 webhooks | HMAC `X-Signature`, stable `X-Sild-Event-Id`, delivery log, backoff retry on failure | `connector/webhook/relay_test.go` |
| §6.2 email | inbound creates/threads by token; agent reply emails out; signature gate | `domain/email_test.go` |
| §4.3 search | keyword (body + member metadata) + `status:`/`role:` filters, AND'd | `domain/search_test.go` |
| §7 platform RBAC | agent can't manage API keys; owner can | `api/auth_rbac_test.go: TestPlatformRoleGuardsAPIKeys` |
| §7 conversation RBAC | non-member → 403 | `…: TestNonMemberForbidden` |
| §11 uploads | size cap enforced; ownership record; attachments validated against completed uploads | exercised via `domain.IssueUpload` + `SendMessage` attachment resolution |
| §12 archival | write-then-delete (verified), tombstone + membership snapshot, sink rehydrate; OPEN never archived | `archive/job_test.go` |

## Conventions

- `internal/testutil` builds an in-process backend (real store + services + gin)
  over a temp SQLite file, with a capturing realtime publisher and mailer so
  egress (events, email) is assertable without infra.
- Each test gets an isolated database (temp file), so tests are independent.
