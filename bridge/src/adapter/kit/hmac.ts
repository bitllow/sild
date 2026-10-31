import { createHash, createHmac, randomBytes, timingSafeEqual } from 'node:crypto';
import { LIMITS } from './types.ts';

// Bridge → adapter request signing. The adapter checks every request.
export const HEADER = {
  timestamp: 'x-sild-timestamp',
  nonce: 'x-sild-nonce',
  signature: 'x-sild-signature',
} as const;

const NONCE = /^[A-Za-z0-9_-]{16,64}$/;
export const MIN_SECRET_LENGTH = 16;

export function checkSecret(secret: string): void {
  if (secret.length < MIN_SECRET_LENGTH) throw new Error(`adapter secret must be at least ${MIN_SECRET_LENGTH} characters`);
}

export type SignedRequest = {
  method: string;
  target: string; // path + query exactly as sent
  body: string | Buffer;
};

export function canonical(req: SignedRequest, timestamp: string, nonce: string): string {
  const bodyHash = createHash('sha256').update(req.body).digest('hex');
  return ['SILD-HMAC-SHA256', req.method.toUpperCase(), req.target, bodyHash, timestamp, nonce].join('\n');
}

export function signRequest(secret: string, req: SignedRequest, now = Date.now()): Record<string, string> {
  const timestamp = String(Math.floor(now / 1000));
  const nonce = randomBytes(18).toString('base64url');
  const signature = createHmac('sha256', secret).update(canonical(req, timestamp, nonce)).digest('base64url');
  return { [HEADER.timestamp]: timestamp, [HEADER.nonce]: nonce, [HEADER.signature]: signature };
}

export type Verdict = { ok: true } | { ok: false; reason: string };

export class NonceStore {
  #seen = new Map<string, number>();
  #ttlMs: number;

  constructor(ttlMs = LIMITS.clockSkewSeconds * 2 * 1000) {
    this.#ttlMs = ttlMs;
  }

  // True the first time a nonce is seen within the TTL.
  claim(nonce: string, now = Date.now()): boolean {
    for (const [n, exp] of this.#seen) {
      if (exp > now) break; // insertion order = expiry order
      this.#seen.delete(n);
    }
    if (this.#seen.has(nonce)) return false;
    this.#seen.set(nonce, now + this.#ttlMs);
    return true;
  }
}

type Headers = Record<string, string | string[] | undefined>;

export function verifyRequest(
  secret: string,
  nonces: NonceStore,
  req: SignedRequest,
  headers: Headers,
  now = Date.now(),
): Verdict {
  if (secret.length < MIN_SECRET_LENGTH) return { ok: false, reason: 'adapter secret too short' };
  const ts = one(headers[HEADER.timestamp]);
  const nonce = one(headers[HEADER.nonce]);
  const sig = one(headers[HEADER.signature]);
  if (!ts || !nonce || !sig) return { ok: false, reason: 'missing signature headers' };
  if (!/^\d{1,12}$/.test(ts)) return { ok: false, reason: 'bad timestamp' };
  if (Math.abs(now / 1000 - Number(ts)) > LIMITS.clockSkewSeconds) return { ok: false, reason: 'timestamp outside window' };
  if (!NONCE.test(nonce)) return { ok: false, reason: 'bad nonce' };

  const want = createHmac('sha256', secret).update(canonical(req, ts, nonce)).digest();
  const got = Buffer.from(sig, 'base64url');
  if (got.length !== want.length || !timingSafeEqual(got, want)) return { ok: false, reason: 'bad signature' };
  // Only a correctly signed request may consume a nonce, so forgeries cannot fill the store.
  if (!nonces.claim(nonce, now)) return { ok: false, reason: 'replayed nonce' };
  return { ok: true };
}

// A header sent once; a repeated header counts as absent.
export function one(v: string | string[] | undefined): string | undefined {
  return Array.isArray(v) ? undefined : v;
}
