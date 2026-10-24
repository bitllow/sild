# Embedded sign-in

A host app frames the Sild inbox and hands it the token its own user already
has. Sild does not interpret that token. It asks the integrator's **validator**,
an endpoint the integrator runs, who the token belongs to. Each integrator writes
its own validator, so all integrator-specific logic (signature checks, token
types, which roles may use support) lives on the integrator's side.

```
host page ──postMessage {token}──▶ iframe ──POST /v1/admin/auth/embedded {embed_key, token}──▶ Sild
                                                                                         │
                                              POST <validator_url> {token}, Bearer <API token>
                                                                                         ▼
                                                                                 integrator validator
```

## Configuration (per tenant, owner only)

`PUT /v1/identity-provider` replaces the whole config:

```json
{
  "validator_url": "https://app.example/sild/validate",
  "validator_token": "<API token Sild presents to the validator>",
  "allowed_origins": ["https://app.example"]
}
```

- `validator_url` must use `https`. Plain `http` is accepted only when Sild runs
  in `development` or `test`, and only to a loopback host (`localhost`,
  `127.0.0.1`, `::1`).
- `validator_token` is write-only. It is sealed at rest and never returned.
  Omit it to keep the stored token, but only while `validator_url` stays the
  same: a new URL requires the token again, so a stored token is never sent
  anywhere it was not given for.
- `allowed_origins` are the origins allowed to frame the inbox and post it a
  token.

`GET /v1/identity-provider` returns `embed_key`, `validator_url`,
`has_validator_token`, `allowed_origins` and `updated_at`. `DELETE` turns
embedded sign-in off and ends every embedded session of the tenant.

From the CLI: `sild-admin identity set --tenant ID --validator-url URL
--validator-token-file FILE --origins a,b`, plus `identity show` and
`identity remove`.

Only the host page's token comes from the request. The URL and the API token
come from this configuration alone.

## The validator contract

### Request (Sild → validator)

```
POST <validator_url>
Authorization: Bearer <validator API token>
Content-Type: application/json

{"token": "<integrator token>"}
```

Sild waits 5 seconds and does not follow redirects. The validator should reject
any call that does not carry its API token, with a non-2xx status.

### Response (validator → Sild)

The person may sign in:

```json
{"active": true, "sub": "fleetline.7", "exp": 1767225600, "name": "Marko Saar"}
```

- `sub` (required): the person's stable identity at the integrator, 1–255
  characters. Sild matches it to a team member's `external_sub`.
- `exp` (optional): unix seconds at which the token expires.
- `name` (optional): display name. Sild does not use it today.

The call worked, but this person may not sign in:

```json
{"active": false, "reason": "role_not_allowed", "sub": "fleetline.4411"}
```

- `reason`: a short code matching `[a-z0-9_.-]{1,64}`. It defaults to
  `inactive` when absent. Examples: `role_not_allowed` for an end-user or driver
  token, `token_expired`, `invalid_token`.
- `sub` (optional): shown to the refused person so an admin can link them.

Unknown fields are ignored. Any non-2xx status, a body that is not this JSON,
`active` missing, or an active answer without a `sub` counts as a **validator
failure**. Sild never treats one as a sign-in.

## What Sild does with the answer

`POST /v1/admin/auth/embedded {embed_key, token}` answers the iframe:

| outcome | status | body |
|---|---|---|
| active, linked member with roles | 200 | `{token: "sild_emb_…", expires_at, admin_id, external_sub}` |
| validator says inactive | 403 | `error.code = access_denied`, `error.fields = {reason, sub}` with the validator's reason |
| active, but `exp` already passed | 403 | `access_denied`, reason `token_expired` |
| active, no member has this `external_sub` | 403 | `access_denied`, reason `user_not_provisioned` |
| active, member holds no roles | 403 | `access_denied`, reason `no_grants` |
| validator failure (unreachable, timeout, non-2xx, malformed) | 502 | `error.code = validator_failed`; Sild logs the detail and does not return it |
| unknown `embed_key` | 401 | |

The session expires at the earlier of one hour and the validator's `exp`, so a
sign-out at the integrator reaches Sild within the token's own lifetime. The
host sends a fresh token before then. Members are linked by `external_sub` only,
never by email, and nobody is provisioned automatically. An admin sets the link
with `PUT /v1/team/:id/external-sub` or `sild-admin agent link`.

## Testing a validator

`tools/fake-integrator` contains a reference validator (`POST /validate`) and
`check-validator.ts`, which checks any validator URL against this contract.
