# Integrating with Sild tech-support triage

Your staff ask Sild's triage assistant about problems in your product. The
assistant reads your source code and asks your systems for data. To take part
you build two small services. Both run on your side, and Sild only calls them:

| you build | what it does | reference |
|---|---|---|
| **Validator** | tells Sild who a sign-in token belongs to | [embedded-sign-in.md](embedded-sign-in.md) |
| **Adapter** | answers the assistant's data requests from your systems | [adapter contract](../bridge/adapter/openapi.yaml) |

This page shows how the two fit together. The references define the wire
formats.

```
your app ── token ─▶ Sild inbox ── POST validator {token} ─▶ your validator ─▶ {active, sub}
                                                                                   │
                          sub is linked to a Sild team member (external_sub) ◀─────┘

requester's message or the assistant's request_info
   ▼
Sild bridge ── "who asks this exact request?" ─▶ Sild backend ── signed requester assertion
   │                                                                      │
   └── POST /requests {requester.id = sub}, HMAC + X-Sild-Requester ◀─────┘ ─▶ your adapter
   ◀── signed delivery (JWS) ───────────────────────────────────────────────────────┘
```

## One identity, both services

The `sub` your validator returns is the person's stable id in your system, for
example `fleetline.7`. An admin links it once to a Sild team member, with
`PUT /v1/team/:id/external-sub` or `sild-admin agent link --sub`. Sild never
links by email and never creates members on its own.

Every data request carries that same value as `requester.id`, together with
a **requester assertion**: a short-lived token signed by Sild, not by the
bridge, that vouches for exactly this request. Sild issues one only for a
conversation the bridge currently holds, and only for that conversation's
linked requester. A leaked bridge secret therefore cannot ask about anyone.
Requesters with no link get no data at all.

This holds for every request, whether it was fetched automatically from the
requester's message, asked by the assistant mid-investigation, or retried
after an approval. Each one gets its own assertion, HMAC signature and nonce,
and nothing is reused.

**Your adapter decides, for each request, whether this person may see this
data.** That check is the main access control on your data.

## The validator

See [embedded-sign-in.md](embedded-sign-in.md). In short:

- `POST {token}` with your API token as Bearer.
- Answer `{active: true, sub}` for staff who may use support. Answer
  `{active: false, reason}` for everyone else, for example drivers or end users
  with `role_not_allowed`.
- Sild keeps the session until the earlier of one hour and your `exp`.
- Check it with `tools/fake-integrator/check-validator.ts`.

## The adapter

The reference is [openapi.yaml](../bridge/adapter/openapi.yaml). For Node there
is a kit, `bridge/src/adapter/kit` (`createAdapter`), which does the signing,
verification, long-polling and file serving for you. You write the catalog and
a handler. `bridge/fake-adapter/` is a complete example.

### Endpoints

- `GET /catalog`: the sources you offer.
- `GET /instructions`: optional text for the assistant about your product.
- `POST /requests`: one data request. Answer it at once (201), decline it (403)
  or accept it and deliver later (202).
- `GET /deliveries`: the bridge long-polls this for later answers.
- `GET /files/{token}`: large files referenced by a delivery.

### Keys

| key | who holds it | used for |
|---|---|---|
| shared secret, at least 16 characters | you and the bridge | the bridge signs every request with HMAC; you verify the signature and refuse replays |
| Ed25519 key pair | you hold the private key, the bridge pins the public key | you sign every delivery as a JWS |
| Sild's signing keys | published by Sild at `<sild>/.well-known/jwks.json` | you verify each requester assertion |

`generateKeyPair()` in the kit makes the pair. Give Sild the public key (the raw
key, base64url) and the secret, and keep both out of your repository.

With the kit, pass `sild: { tenant, jwksUrl, issuer }` to `createAdapter`.
`issuer` is optional and pins Sild's issuer. `keys` pins JWKs (EC P-256) in
addition to or instead of the URL, for tests. The kit fetches and caches Sild's
keys. It refuses a request unless the assertion's type, tenant, expiry, body
hash, requester, conversation and request id all match it. Without the kit,
follow the checks in the contract's "Requester assertion" section.

### The catalog is what the assistant sees

The assistant reads every catalog entry when a conversation starts: the name,
the description, each param's type, whether it is required, its allowed values
and its description, and `max_wait_seconds`. It asks only for what is listed.
The bridge's router then turns the assistant's request into a call to one
source with concrete params. Write entries for that reader:

- One source per question someone actually asks, for example
  `driver.devices`: "A driver's registered phones: app version and
  push-notification token state."
- Put real examples in param descriptions ("Driver id, e.g. drv_2207") and use
  `enum` for closed lists.
- Give an honest `max_wait_seconds`. After that time the bridge reports the data
  as not available.
- Leave out sources support staff must never see. Declining a request at run
  time also works, and the requester sees your reason.

### Limits the bridge enforces

- 3 requests per assistant turn and 12 per conversation.
- At most 4 different ids per conversation (`MAX_REQUEST_IDS`). An id is the
  value of any free-text string param, so enums and numbers do not count. This
  caps how far one conversation can browse your data.
- The file and size limits are listed in the contract.

These limits are a backstop. Your own check on each request still applies.
Sild also caps the assertions it signs per conversation, and records each
one.

### What to send back

- `summary` is one line, and it is shown to the requester. Files reach only the
  assistant's sandbox.
- You redact. Send what support needs to diagnose the problem, and leave out
  data it should not have, such as card numbers, passwords or full addresses.

### Instructions

`GET /instructions` returns `{text, version}`, up to 8 KiB. Describe your
product, its terms, the language and tone to use, and where to send people for
what the assistant cannot do, for example "invoice corrections go to
billing@…". The text cannot override Sild's own rules for the assistant.

## Network

- **Adapter:** must use `https` on an address that resolves to a public IP. The
  bridge refuses private, loopback and link-local addresses and follows no
  redirects. A private network (VPN or private link) is not supported yet.
  Firewall the adapter to the bridge's egress IPs; the HMAC signature is the
  second layer.
- **Validator:** any `https` URL. Restrict it to Sild's egress IPs.

## Testing

- Adapter: run the conformance suite against a staging deployment that trusts
  a test key in place of Sild's own keys.

  ```sh
  cd bridge
  node fake-adapter/test-key.ts   # SILD_TEST_KEY for the suite, SILD_KEYS for your adapter
  ADAPTER_URL=https://adapter-staging.example.com/sild ADAPTER_SECRET=… ADAPTER_PUBLIC_KEY=… \
    SILD_TENANT=t_… SILD_TEST_KEY='…' CONFORMANCE_CASES=cases.json npm run conformance
  ```

  `cases.json` names a source and params of yours for each case. A case you
  leave out is skipped. See the top of
  `bridge/adapter/conformance/conformance.test.ts`.
- Validator: run `tools/fake-integrator/check-validator.ts`.
- End to end: `scripts/triage-local.sh` runs Sild, the bridge and the fake
  adapter and integrator on one machine. It shows what your services stand in
  for.

## Step-up: approval before data

You can require a person to approve data access, for example with a passkey,
before your adapter answers:

1. Your adapter declines with `code: step_up_required` while there is no
   approval for this requester in this conversation (the assertion's `sub` and
   `conv`). With the kit, return
   `{ type: 'decline', code: STEP_UP_REQUIRED, reason }`.
2. The bridge parks the request. The requester sees what the assistant wants
   to check, with an **Approve data access** button. A parked request waits
   until they approve or the conversation closes.
3. The embedded inbox posts `{ type: "sild:step-up", conversation, reason }`
   to your host page, at your exact origin.
4. Your page runs the passkey against your own backend, which records the
   approval for `sub` + `conversation` with a fixed time-to-live (not
   extended on use).
5. Your page answers `{ type: "sild:step-up-done", conversation, ok }` to the
   inbox's origin. Sild notes the approval in the conversation and retries
   the parked requests, and your adapter now finds the approval.

One approval covers every request in that conversation until it expires. Each
request is still asserted and signed on its own. After expiry, or in another
conversation, the requester is asked again. To approve per subject instead,
store the approved ids with the approval and check `params` against them.

Require user verification (biometric or PIN) and check its flag on your server.
Give the adapter read-only access to the approval store, and fail closed when
the store is unreachable. Clear approvals on logout and when someone is
disabled. Turn on `embedded_only` (`sild-admin identity set … --embedded-only`),
since only your own page can run the step-up. With it on, linked members can
sign in only through your page, and sessions they already had end at once.
`tools/fake-integrator` and `STEP_UP=1 scripts/triage-local.sh` show the whole
flow.

## Hardening on your side

Sild's checks narrow who can ask; these keep a legitimate requester to what
they should see:

- **Scope, not just role.** Check what this person may see, for example only
  their own company's drivers and trips, not merely that their role may use
  support.
- **Re-check the person on every request.** Someone off-boarded an hour ago
  must be refused now, whatever happened at sign-in.
- **Rate limits** per requester and per source, and your own cap on how many
  different subjects one person looks up.
- **Network:** accept only Sild's egress IPs.
- **A read-only identity** for the adapter: its own database user or API scope,
  with no write paths.
- **Your own audit log and alerts:** who asked, what, and allowed or declined.
- **Minimal answers:** field allowlists per source, and redaction.
- **Key hygiene:** keys in a secret manager, with a rotation plan, and
  short-lived file URLs.
- **A kill switch:** one flag that makes the adapter decline everything.

## Go-live checklist

- [ ] The validator refuses calls without its API token, and refuses non-staff
      tokens with a reason.
- [ ] Support staff are linked (`external_sub`) to Sild team members.
- [ ] The adapter checks `requester.id` on every request.
- [ ] The adapter verifies the HMAC signature and refuses stale or replayed
      requests.
- [ ] The adapter verifies the requester assertion against Sild's JWKS, for
      your tenant, on every request.
- [ ] Deliveries are signed with a private key that is not in any repository.
- [ ] Catalog descriptions are written for the assistant, and sensitive sources
      are left out or declined.
- [ ] Summaries and files are redacted.
- [ ] Both services are on `https`, and firewalled to Sild's egress IPs.
- [ ] The conformance suite passes against the production adapter.
- [ ] With step-up: the passkey requires user verification, approvals have a
      fixed TTL and are cleared on logout, the adapter fails closed when the
      approval store is unreachable, and `embedded_only` is on.
