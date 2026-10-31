# Fleetline — fake integrator (dev only)

A host app that embeds the Sild inbox the way an integrator would. It signs
tokens shaped like the reference integrator's — ES256, claims `i` (account id),
`n` (namespace), `r` (roles), no `sub`/`iss`/`aud` — posts them to the iframe
with an exact target origin, and runs the validator Sild calls to learn who a
token belongs to ([contract](../../docs/embedded-sign-in.md)).

The signing key is generated at every start, so tokens from a previous run stop
validating.

## Run it (Node 24, no dependencies)

```sh
# 1. backend: seeds the triage demo AND this integrator's validator config
cd backend
SILD_DEV_TRIAGE=1 go run ./cmd/sild-dev

# 2. inbox
cd inbox && npm run dev                      # http://localhost:3000

# 3. this host
cd tools/fake-integrator && node server.ts   # http://localhost:4400
```

Open http://localhost:4400 and switch users in the top bar:

| user            | token                           | result                          |
|-----------------|---------------------------------|---------------------------------|
| Admin           | `fleetline.2`, taxiCompanyAdmin | admin@sild.local (owner)        |
| Support agent   | `fleetline.1`, admin            | liis@sild.local (agent)         |
| Standard user   | `fleetline.7`, dispatcher       | marko@sild.local (requester)    |
| Not provisioned | `fleetline.40`, dispatcher      | 403 `user_not_provisioned`      |
| Driver          | `fleetline.4411`, driver        | 403 `role_not_allowed`          |

The seed configures: validator `http://localhost:4400/validate` with the dev
validator token, origin `http://localhost:4400`, embed key `emb_dev_fleetline`,
and links the three members above.

## Endpoints

- `GET /` — the host page (iframe → `$INBOX_URL/embed?key=$EMBED_KEY`)
- `GET /token?user=admin|agent|user|none|driver` — a fresh token and its claims
- `POST /validate` — the validator. Needs `Authorization: Bearer $FAKE_VALIDATOR_TOKEN`;
  verifies the signature and `exp`; refuses namespaces `driver`/`client` and
  tokens without a staff role (admin, taxiCompanyAdmin, dispatcher) with
  `role_not_allowed`; otherwise answers `active` with `sub` = `{n}.{i}`.

## Step-up with a passkey

With `STEP_UP=1 scripts/triage-local.sh`, data waits for the requester's
approval. Approve in the Fleetline page and it asks for a real passkey (Touch
ID, a password manager, or a phone via QR). The first time, it creates one for
the persona. It records the approval for 15 minutes (`STEP_UP_TTL`), and the
fake adapter checks it at `GET /elevation`. Passkeys need a secure context: on
`http://localhost:4400` they work. On a LAN address over plain http the page
falls back to a labelled simulated prompt, since a passkey also cannot belong to
a raw IP address. Passkeys live in memory, so restart and create one again.

## Environment

| var                    | default                         |                                         |
|------------------------|---------------------------------|-----------------------------------------|
| `PORT`                 | `4400`                          | host origin is `http://localhost:$PORT` |
| `INBOX_URL`            | port 3000 of the page's host    | where the inbox runs                    |
| `EMBED_KEY`            | `emb_dev_fleetline`             | the tenant's embed key                  |
| `TOKEN_TTL`            | `3600`                          | seconds; lower it to watch re-exchange  |
| `FAKE_VALIDATOR_TOKEN` | `fleetline-dev-validator-token` | the API token Sild must present         |

A host on another port or address also needs its origin in sild-dev's
`SILD_DEV_EMBED_ORIGIN`, a comma-separated list whose first entry also gives the
validator URL (`scripts/triage-local.sh` adds this machine's LAN address). A
different validator token needs `SILD_DEV_EMBED_VALIDATOR_TOKEN`.

## Configuring a tenant by hand

`sild-admin` seals the validator token, so it needs the server's
`SILD_SECRETS_KEY` (sild-dev uses a fixed dev key when none is set; see
`devSecretsKey` in `backend/cmd/sild-dev/main.go`).

```sh
cd backend
go run ./cmd/sild-admin identity set --tenant <TENANT_ID> \
  --validator-url http://localhost:4400/validate \
  --validator-token-file <(printf %s fleetline-dev-validator-token) \
  --origins http://localhost:4400
go run ./cmd/sild-admin agent link --tenant <TENANT_ID> --email marko@sild.local --sub fleetline.7
EMBED_KEY=<printed embed_key> node ../tools/fake-integrator/server.ts
```

## Checking a validator

`check-validator.ts` runs a validator endpoint, this one or a real integrator's,
against the contract:

```sh
VALIDATOR_TOKEN=fleetline-dev-validator-token node check-validator.ts http://localhost:4400/validate \
  "$(curl -s 'localhost:4400/token?user=user' | jq -r .token)" \
  "$(curl -s 'localhost:4400/token?user=driver' | jq -r .token)"
```

## The postMessage contract

- iframe → host: `{type: "sild:ready"}` once it knows its allowed origins, and
  `{type: "sild:token-request"}` before its session expires or after a 401.
- host → iframe: `{type: "sild:token", token, host?: "Fleetline"}`, sent to the
  inbox origin exactly (never `*`). The iframe accepts it only from an allowed
  origin and only from its parent window.
