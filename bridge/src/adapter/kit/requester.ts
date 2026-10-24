import { createHash, createPublicKey, generateKeyPairSync, sign, verify, type JsonWebKey, type KeyObject } from 'node:crypto';
import { b64json, splitCompact } from './jws.ts';
import { isRecord } from './schema.ts';
import type { InfoRequest } from './types.ts';

// Sild's signed statement of who asks a request, sent by the bridge next to the HMAC signature.
export const REQUESTER_HEADER = 'x-sild-requester';

const SKEW_SECONDS = 30;
const REFRESH_MS = 10 * 60_000;
// After a failed or missing-key fetch, Sild's JWKS is asked again at most this often.
const RETRY_MS = 60_000;

// How an adapter knows Sild: the tenant it serves, and Sild's signing keys from
// its JWKS URL (`<sild>/.well-known/jwks.json`), pinned JWKs, or both.
export type SildTrust = { tenant: string; jwksUrl?: string; keys?: JsonWebKey[]; issuer?: string };

export type RequesterClaims = {
  iss: string;
  sub: string;
  tid: string;
  typ: 'requester';
  conv: string;
  rid: string;
  bsh: string;
  iat: number;
  exp: number;
};

export class SildKeys {
  #trust: SildTrust;
  #pinned = new Map<string, KeyObject>();
  #fetched = new Map<string, KeyObject>();
  #fetchedAt = 0;
  #triedAt = 0;
  #inflight: Promise<void> | null = null;

  constructor(trust: SildTrust) {
    if (!trust.jwksUrl && !trust.keys?.length) throw new Error('Sild trust needs a jwksUrl or pinned keys');
    this.#trust = trust;
    for (const jwk of trust.keys ?? []) {
      if (jwk.kty !== 'EC' || jwk.crv !== 'P-256') throw new Error('a pinned Sild key must be an EC P-256 JWK');
      this.#pinned.set(String(jwk.kid ?? ''), createPublicKey({ key: jwk, format: 'jwk' }));
    }
  }

  // Null when the assertion vouches for exactly this request, otherwise why not.
  async check(token: string | undefined, body: Buffer, req: InfoRequest, now = Date.now()): Promise<string | null> {
    if (!token) return 'missing requester assertion';
    const c = splitCompact(token);
    if (typeof c === 'string') return `requester assertion: ${c}`;
    if (c.header.alg !== 'ES256' || typeof c.header.kid !== 'string') return 'requester assertion: algorithm not allowed';
    const key = await this.#key(c.header.kid, now);
    if (!key) return 'requester assertion: unknown signing key';
    if (!verify('sha256', c.signed, { key, dsaEncoding: 'ieee-p1363' }, c.signature)) return 'requester assertion: bad signature';
    const p = c.payload;
    if (!isRecord(p) || p.typ !== 'requester') return 'requester assertion: wrong type';
    if (p.tid !== this.#trust.tenant) return 'requester assertion: other tenant';
    if (this.#trust.issuer && p.iss !== this.#trust.issuer) return 'requester assertion: other issuer';
    const t = now / 1000;
    if (typeof p.exp !== 'number' || p.exp < t - SKEW_SECONDS) return 'requester assertion: expired';
    if (typeof p.iat !== 'number' || p.iat > t + SKEW_SECONDS) return 'requester assertion: issued in the future';
    if (p.bsh !== createHash('sha256').update(body).digest('base64url')) return 'requester assertion: signed another body';
    if (p.sub !== req.requester.id || p.conv !== req.conversation_ref || p.rid !== req.id) return 'requester assertion: does not match the request';
    return null;
  }

  // A known key is served at once and refreshed behind the request; only an unknown one waits for Sild.
  async #key(kid: string, now: number): Promise<KeyObject | undefined> {
    const known = this.#pinned.get(kid) ?? this.#fetched.get(kid);
    if (this.#trust.jwksUrl) {
      const stale = now - this.#fetchedAt > REFRESH_MS && now - this.#triedAt > RETRY_MS;
      if (known && stale) void this.#refresh(now);
      if (!known) await (this.#inflight ?? (now - this.#triedAt > RETRY_MS ? this.#refresh(now) : null));
    }
    return this.#pinned.get(kid) ?? this.#fetched.get(kid);
  }

  #refresh(now: number): Promise<void> {
    this.#triedAt = now;
    this.#inflight ??= (async () => {
      try {
        const res = await fetch(this.#trust.jwksUrl!, { redirect: 'error', signal: AbortSignal.timeout(5_000) });
        if (!res.ok) return;
        const body: unknown = await res.json();
        const keys = isRecord(body) && Array.isArray(body.keys) ? body.keys : [];
        const next = new Map<string, KeyObject>();
        for (const jwk of keys) {
          if (!isRecord(jwk) || typeof jwk.kid !== 'string' || jwk.kty !== 'EC' || jwk.crv !== 'P-256') continue;
          next.set(jwk.kid, createPublicKey({ key: jwk as JsonWebKey, format: 'jwk' }));
        }
        this.#fetched = next;
        this.#fetchedAt = now;
      } catch {
        // Keep the last good keys; an unknown kid is refused.
      } finally {
        this.#inflight = null;
      }
    })();
    return this.#inflight;
  }
}

// For tests and conformance runs: an adapter pins `jwk`, the caller signs with `privateKey`.
export function generateSildTestKey(kid = 'test'): { privateKey: KeyObject; jwk: JsonWebKey } {
  const { privateKey, publicKey } = generateKeyPairSync('ec', { namedCurve: 'P-256' });
  return { privateKey, jwk: { ...publicKey.export({ format: 'jwk' }), kid } };
}

export function signRequester(privateKey: KeyObject, kid: string, claims: RequesterClaims): string {
  const signed = `${b64json({ alg: 'ES256', typ: 'JWT', kid })}.${b64json(claims)}`;
  return `${signed}.${sign('sha256', Buffer.from(signed), { key: privateKey, dsaEncoding: 'ieee-p1363' }).toString('base64url')}`;
}

// The claims Sild would sign for this exact request body.
export function requesterClaimsFor(tenant: string, body: string, now = Date.now()): RequesterClaims {
  const req = JSON.parse(body) as InfoRequest;
  const iat = Math.floor(now / 1000);
  return {
    iss: 'test', sub: req.requester.id, tid: tenant, typ: 'requester', conv: req.conversation_ref, rid: req.id,
    bsh: createHash('sha256').update(body).digest('base64url'), iat, exp: iat + 300,
  };
}
