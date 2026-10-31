import assert from 'node:assert/strict';
import { test } from 'node:test';
import { SildKeys, generateSildTestKey, requesterClaimsFor, signRequester, type RequesterClaims } from './requester.ts';
import type { InfoRequest } from './types.ts';

const key = generateSildTestKey('k1');
const trust = { tenant: 't_1', keys: [key.jwk] };
const keys = new SildKeys(trust);
const req: InfoRequest = {
  id: 'a3f1c9e2b7d84f0e', requester: { id: 'fleetline.7' }, conversation_ref: 'c_1',
  source: 'driver.devices', params: { driver_id: 'drv_2207' }, reason: 'no alerts',
};
const body = JSON.stringify(req);
const sign = (over: Partial<RequesterClaims> = {}, signer = key) =>
  signRequester(signer.privateKey, 'k1', { ...requesterClaimsFor('t_1', body), ...over });
const check = (token: string | undefined, b = body) => keys.check(token, Buffer.from(b), JSON.parse(b) as InfoRequest);

test('an assertion for exactly this request passes', async () => {
  assert.equal(await check(sign()), null);
});

test('a missing, forged or foreign assertion is refused', async () => {
  assert.equal(await check(undefined), 'missing requester assertion');
  assert.equal(await check(sign({}, generateSildTestKey('k1'))), 'requester assertion: bad signature');
  assert.equal(await check(signRequester(key.privateKey, 'k9', requesterClaimsFor('t_1', body))), 'requester assertion: unknown signing key');
  assert.equal(await check(sign({ tid: 't_2' })), 'requester assertion: other tenant');
  assert.equal(await check(sign({ typ: 'user' as 'requester' })), 'requester assertion: wrong type');
  assert.equal(await check(sign({ exp: Math.floor(Date.now() / 1000) - 120 })), 'requester assertion: expired');
});

test('an assertion binds the body, the requester, the conversation and the request id', async () => {
  const other = JSON.stringify({ ...req, params: { driver_id: 'drv_9999' } });
  assert.equal(await check(sign(), other), 'requester assertion: signed another body');
  assert.equal(await check(sign({ sub: 'fleetline.8' })), 'requester assertion: does not match the request');
  assert.equal(await check(sign({ conv: 'c_2' })), 'requester assertion: does not match the request');
  assert.equal(await check(sign({ rid: 'b0000000000000000' })), 'requester assertion: does not match the request');
});

test('checks that arrive while the keys are being fetched wait for them', async () => {
  const real = globalThis.fetch;
  let release!: () => void;
  globalThis.fetch = (() => new Promise((r) => (release = () => r(Response.json({ keys: [key.jwk] }))))) as typeof fetch;
  try {
    const fetched = new SildKeys({ tenant: 't_1', jwksUrl: 'https://sild.example/.well-known/jwks.json' });
    const token = sign();
    const first = fetched.check(token, Buffer.from(body), req);
    const second = fetched.check(token, Buffer.from(body), req);
    release();
    assert.deepEqual(await Promise.all([first, second]), [null, null]);
  } finally {
    globalThis.fetch = real;
  }
});

test('a pinned key must be EC P-256', () => {
  assert.throws(() => new SildKeys({ tenant: 't_1', keys: [{ kty: 'OKP', crv: 'Ed25519', x: 'AA', kid: 'x' }] }), /EC P-256/);
});
