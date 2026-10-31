// Real WebAuthn for the dev integrator: a passkey per person, checked before a step-up.
// Registration skips attestation ("none") and reads the key from getPublicKey().

import { createHash, createPublicKey, randomBytes, verify, type KeyObject } from "node:crypto";

type Passkey = { id: string; key: KeyObject; counter: number };
// The relying party as the browser saw it: the page origin and its hostname.
export type Ceremony = { origin: string; rpId: string };

const CHALLENGE_TTL_MS = 60_000;
const passkeys = new Map<string, Passkey[]>();
const challenges = new Map<string, { value: string; expires: number }>();

const bytes = (v: unknown) => (typeof v === "string" ? Buffer.from(v, "base64url") : Buffer.alloc(0));
const sha256 = (v: Buffer | string) => createHash("sha256").update(v).digest();

/** A single-use challenge for one ceremony. */
export function challenge(purpose: string): string {
  const value = randomBytes(32).toString("base64url");
  challenges.set(purpose, { value, expires: Date.now() + CHALLENGE_TTL_MS });
  return value;
}

export function credentialIds(sub: string): string[] {
  return (passkeys.get(sub) ?? []).map((p) => p.id);
}

/** Null once a new passkey for sub is stored, otherwise why not. */
export function register(sub: string, c: Ceremony, body: Record<string, unknown>): string | null {
  const authData = bytes(body.authenticatorData);
  const why = ceremony("webauthn.create", `reg:${sub}`, c, bytes(body.clientDataJSON), authData);
  if (why) return why;
  if (body.alg !== -7 || typeof body.id !== "string") return "only ES256 passkeys";
  let key: KeyObject;
  try {
    key = createPublicKey({ key: bytes(body.publicKey), format: "der", type: "spki" });
  } catch {
    return "bad public key";
  }
  passkeys.set(sub, [...(passkeys.get(sub) ?? []), { id: body.id, key, counter: authData.readUInt32BE(33) }]);
  return null;
}

/** Null when the assertion proves sub, present and verified, answered this challenge. */
export function check(sub: string, purpose: string, c: Ceremony, body: Record<string, unknown>): string | null {
  const p = (passkeys.get(sub) ?? []).find((k) => k.id === body.id);
  if (!p) return "unknown passkey";
  const authData = bytes(body.authenticatorData);
  const clientData = bytes(body.clientDataJSON);
  const why = ceremony("webauthn.get", purpose, c, clientData, authData);
  if (why) return why;
  if (!verify("sha256", Buffer.concat([authData, sha256(clientData)]), p.key, bytes(body.signature))) return "bad signature";
  // Synced passkeys report 0 throughout; a device-bound one must move forward.
  const counter = authData.readUInt32BE(33);
  if ((counter || p.counter) && counter <= p.counter) return "signature counter went back";
  p.counter = counter;
  return null;
}

function ceremony(type: string, purpose: string, c: Ceremony, clientData: Buffer, authData: Buffer): string | null {
  let cd: { type?: unknown; challenge?: unknown; origin?: unknown };
  try {
    cd = JSON.parse(clientData.toString("utf8"));
  } catch {
    return "bad client data";
  }
  const issued = challenges.get(purpose);
  challenges.delete(purpose);
  if (cd.type !== type) return "wrong ceremony";
  if (!issued || issued.expires < Date.now() || cd.challenge !== issued.value) return "unknown or expired challenge";
  if (cd.origin !== c.origin) return "wrong origin";
  if (authData.length < 37 || !authData.subarray(0, 32).equals(sha256(c.rpId))) return "wrong relying party";
  const flags = authData[32]!;
  if (!(flags & 0x01) || !(flags & 0x04)) return "user presence and verification are required";
  return null;
}
