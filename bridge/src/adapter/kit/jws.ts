import { createPrivateKey, createPublicKey, generateKeyPairSync, sign, verify, type KeyObject } from 'node:crypto';
import { isRecord } from './schema.ts';
import type { DeliveryClaims } from './types.ts';

// Adapter → bridge: every delivery is a compact JWS, EdDSA over Ed25519.
const HEADER = { alg: 'EdDSA', typ: 'sild-delivery+jws' } as const;

export type KeyPair = { privateKey: string; publicKey: string };

// privateKey: base64url PKCS#8 DER. publicKey: base64url raw 32-byte key (JWK "x").
export function generateKeyPair(): KeyPair {
  const { privateKey, publicKey } = generateKeyPairSync('ed25519');
  return { privateKey: exportPrivateKey(privateKey), publicKey: exportPublicKey(publicKey) };
}

export function exportPrivateKey(key: KeyObject): string {
  return key.export({ format: 'der', type: 'pkcs8' }).toString('base64url');
}

export function exportPublicKey(key: KeyObject): string {
  const jwk = key.export({ format: 'jwk' });
  if (jwk.crv !== 'Ed25519' || typeof jwk.x !== 'string') throw new Error('not an Ed25519 key');
  return jwk.x;
}

export function loadPrivateKey(s: string): KeyObject {
  const key = s.trimStart().startsWith('-----BEGIN')
    ? createPrivateKey(s)
    : createPrivateKey({ key: Buffer.from(s, 'base64url'), format: 'der', type: 'pkcs8' });
  if (key.asymmetricKeyType !== 'ed25519') throw new Error('adapter key must be Ed25519');
  return key;
}

export function loadPublicKey(s: string): KeyObject {
  const key = s.trimStart().startsWith('-----BEGIN')
    ? createPublicKey(s)
    : createPublicKey({ key: { kty: 'OKP', crv: 'Ed25519', x: s.trim() }, format: 'jwk' });
  if (key.asymmetricKeyType !== 'ed25519') throw new Error('adapter key must be Ed25519');
  return key;
}

export function publicKeyOf(privateKey: KeyObject): string {
  return exportPublicKey(createPublicKey(privateKey));
}

export function signDelivery(key: KeyObject, claims: DeliveryClaims): string {
  const head = b64json(HEADER);
  const body = b64json(claims);
  const sig = sign(null, Buffer.from(`${head}.${body}`), key).toString('base64url');
  return `${head}.${body}.${sig}`;
}

export type Opened = { ok: true; claims: DeliveryClaims } | { ok: false; reason: string };

// A compact JWS taken apart; each opener checks its own algorithm, key and claims.
export type Compact = { header: Record<string, unknown>; payload: unknown; signed: Buffer; signature: Buffer };

export function splitCompact(jws: string): Compact | string {
  const parts = jws.split('.');
  if (parts.length !== 3) return 'not a compact JWS';
  const [head, body, sig] = parts as [string, string, string];
  const header = fromB64json(head);
  if (!isRecord(header)) return 'bad header';
  if (header.crit !== undefined) return 'unsupported crit header';
  return { header, payload: fromB64json(body), signed: Buffer.from(`${head}.${body}`), signature: Buffer.from(sig, 'base64url') };
}

// Checks the signature, the pinned algorithm and expiry. Binding to an
// outstanding request is the caller's job.
export function openDelivery(key: KeyObject, jws: string, now = Date.now()): Opened {
  const c = splitCompact(jws);
  if (typeof c === 'string') return { ok: false, reason: c };
  if (c.header.alg !== HEADER.alg) return { ok: false, reason: 'algorithm not allowed' };
  if (!verify(null, c.signed, key, c.signature)) return { ok: false, reason: 'bad signature' };
  const claims = c.payload;
  if (!isClaims(claims)) return { ok: false, reason: 'bad payload' };
  if (claims.exp * 1000 <= now) return { ok: false, reason: 'expired' };
  if (claims.delivery.id !== claims.request_id) return { ok: false, reason: 'delivery id does not match request_id' };
  return { ok: true, claims };
}

export function b64json(v: unknown): string {
  return Buffer.from(JSON.stringify(v)).toString('base64url');
}

function fromB64json(part: string): unknown {
  try {
    return JSON.parse(Buffer.from(part, 'base64url').toString('utf8'));
  } catch {
    return null;
  }
}

function isClaims(v: unknown): v is DeliveryClaims {
  if (!isRecord(v)) return false;
  const d = v.delivery;
  return (
    typeof v.request_id === 'string' &&
    typeof v.conversation_ref === 'string' &&
    typeof v.iat === 'number' &&
    typeof v.exp === 'number' &&
    isRecord(d) &&
    typeof d.id === 'string' &&
    (d.state === 'delivered' || d.state === 'declined' || d.state === 'expired') &&
    typeof d.summary === 'string' &&
    Array.isArray(d.files) &&
    d.files.every(isWireFile)
  );
}

function isWireFile(f: unknown): boolean {
  if (!isRecord(f)) return false;
  const hasInline = typeof f.inline === 'string';
  const hasUrl = typeof f.url === 'string';
  return (
    typeof f.name === 'string' &&
    typeof f.mime === 'string' &&
    Number.isSafeInteger(f.size) &&
    (f.size as number) >= 0 &&
    typeof f.sha256 === 'string' &&
    /^[0-9a-f]{64}$/.test(f.sha256) &&
    hasInline !== hasUrl
  );
}
