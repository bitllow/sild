import { describe, test } from 'node:test';
import assert from 'node:assert/strict';
import { createPrivateKey, sign } from 'node:crypto';
import { createServer } from 'node:http';
import type { AddressInfo } from 'node:net';
import { createAdapter } from './server.ts';
import { HEADER, NonceStore, checkSecret, signRequest, verifyRequest } from './hmac.ts';
import { generateKeyPair, loadPrivateKey, loadPublicKey, openDelivery, publicKeyOf, signDelivery } from './jws.ts';
import { checkSchema, validate } from './schema.ts';
import type { DeliveryClaims } from './types.ts';

const req = { method: 'POST', target: '/requests', body: '{"a":1}' };
const S = 's3cret-0123456789';

describe('request signing', () => {
  test('a signed request verifies once', () => {
    const nonces = new NonceStore();
    const h = signRequest(S, req);
    assert.deepEqual(verifyRequest(S, nonces, req, h), { ok: true });
    assert.deepEqual(verifyRequest(S, nonces, req, h), { ok: false, reason: 'replayed nonce' });
  });
  test('a different body, target, method or secret fails', () => {
    const h = signRequest(S, req);
    assert.equal(verifyRequest(S, new NonceStore(), { ...req, body: '{"a":2}' }, h).ok, false);
    assert.equal(verifyRequest(S, new NonceStore(), { ...req, target: '/requests?x=1' }, h).ok, false);
    assert.equal(verifyRequest(S, new NonceStore(), { ...req, method: 'PUT' }, h).ok, false);
    assert.equal(verifyRequest('other-secret-0123456789', new NonceStore(), req, h).ok, false);
  });
  test('a timestamp outside the window fails', () => {
    const h = signRequest(S, req, Date.now() - 301_000);
    assert.deepEqual(verifyRequest(S, new NonceStore(), req, h), { ok: false, reason: 'timestamp outside window' });
  });
  test('a forged request does not burn the nonce', () => {
    const nonces = new NonceStore();
    const h = signRequest(S, req);
    assert.equal(verifyRequest(S, nonces, req, { ...h, [HEADER.signature]: 'AAAA' }).ok, false);
    assert.equal(verifyRequest(S, nonces, req, h).ok, true);
  });
  test('missing headers fail', () => {
    assert.equal(verifyRequest(S, new NonceStore(), req, {}).ok, false);
  });
  test('a secret under 16 characters is refused', () => {
    const short = 'fifteen-chars-x';
    assert.deepEqual(verifyRequest(short, new NonceStore(), req, signRequest(short, req)), { ok: false, reason: 'adapter secret too short' });
    assert.throws(() => checkSecret(''), /at least 16/);
    assert.throws(() => checkSecret(short), /at least 16/);
    checkSecret('sixteen-chars-xx');
  });
});

const claims = (over: Partial<DeliveryClaims> = {}): DeliveryClaims => {
  const now = Math.floor(Date.now() / 1000);
  return {
    request_id: 'req_00000001',
    conversation_ref: 'conv1',
    iat: now,
    exp: now + 60,
    delivery: { id: 'req_00000001', state: 'delivered', summary: 'ok', files: [] },
    ...over,
  };
};

describe('delivery JWS', () => {
  const pair = generateKeyPair();
  const priv = loadPrivateKey(pair.privateKey);
  const pub = loadPublicKey(pair.publicKey);

  test('round-trips', () => {
    const opened = openDelivery(pub, signDelivery(priv, claims()));
    assert.equal(opened.ok, true);
    assert.equal(publicKeyOf(priv), pair.publicKey);
  });
  test('another key fails', () => {
    const other = loadPublicKey(generateKeyPair().publicKey);
    assert.deepEqual(openDelivery(other, signDelivery(priv, claims())), { ok: false, reason: 'bad signature' });
  });
  test('alg none fails', () => {
    const [, body] = signDelivery(priv, claims()).split('.');
    const head = Buffer.from(JSON.stringify({ alg: 'none' })).toString('base64url');
    assert.deepEqual(openDelivery(pub, `${head}.${body}.`), { ok: false, reason: 'algorithm not allowed' });
  });
  test('a tampered payload fails', () => {
    const [head, , sig] = signDelivery(priv, claims()).split('.');
    const body = Buffer.from(JSON.stringify(claims({ conversation_ref: 'conv2' }))).toString('base64url');
    assert.equal(openDelivery(pub, `${head}.${body}.${sig}`).ok, false);
  });
  test('an expired delivery fails', () => {
    assert.deepEqual(openDelivery(pub, signDelivery(priv, claims({ exp: 1 }))), { ok: false, reason: 'expired' });
  });
  test('a delivery id that differs from request_id fails', () => {
    const c = claims();
    c.delivery.id = 'req_99999999';
    assert.equal(openDelivery(pub, signDelivery(priv, c)).ok, false);
  });
  test('a file with both inline and url fails', () => {
    const c = claims();
    c.delivery.files = [{ name: 'a', mime: 'text/plain', size: 0, sha256: '0'.repeat(64), inline: '', url: 'https://x/' }];
    assert.deepEqual(openDelivery(pub, signDelivery(priv, c)), { ok: false, reason: 'bad payload' });
  });
  test('loads a PEM private key', () => {
    const pem = createPrivateKey({ key: Buffer.from(pair.privateKey, 'base64url'), format: 'der', type: 'pkcs8' }).export({ format: 'pem', type: 'pkcs8' });
    const key = loadPrivateKey(String(pem));
    const head = Buffer.from('x');
    assert.ok(sign(null, head, key).length === 64);
  });
});

describe('params schema', () => {
  const schema = {
    type: 'object',
    properties: {
      driver_id: { type: 'string', pattern: '^drv_[0-9]+$' },
      days: { type: 'integer', minimum: 1, maximum: 90 },
      service: { type: 'string', enum: ['a', 'b'] },
      flag: { type: 'boolean' },
      ratio: { type: 'number' },
    },
    required: ['driver_id'],
    additionalProperties: false,
  };

  test('valid params pass', () => {
    assert.deepEqual(validate(schema, { driver_id: 'drv_1', days: 3, service: 'a', flag: true, ratio: 0.5 }), []);
  });
  for (const [name, params] of [
    ['missing required', { days: 3 }],
    ['pattern', { driver_id: 'x' }],
    ['integer', { driver_id: 'drv_1', days: 1.5 }],
    ['minimum', { driver_id: 'drv_1', days: 0 }],
    ['maximum', { driver_id: 'drv_1', days: 91 }],
    ['enum', { driver_id: 'drv_1', service: 'c' }],
    ['boolean', { driver_id: 'drv_1', flag: 'yes' }],
    ['number', { driver_id: 'drv_1', ratio: 'NaN' }],
    ['extra key', { driver_id: 'drv_1', other: 1 }],
  ] as const) {
    test(`rejects ${name}`, () => assert.notDeepEqual(validate(schema, params), []));
  }
  test('rejects a non-object', () => assert.notDeepEqual(validate(schema, 'drv_1'), []));
  test('accepts the subset', () => assert.deepEqual(checkSchema(schema), []));
  test('refuses keywords outside the subset', () => {
    assert.match(checkSchema({ type: 'object', properties: { a: { type: 'string', maxLength: 3 } } }).join(), /maxLength/);
    assert.match(checkSchema({ type: 'array' }).join(), /unsupported type/);
  });
});

describe('adapter', () => {
  const opts = { secret: S, privateKey: loadPrivateKey(generateKeyPair().privateKey), catalog: [], handle: () => ({ type: 'expire' as const }) };

  test('refuses a secret under 16 characters at construction', () => {
    assert.throws(() => createAdapter({ ...opts, secret: '' }), /at least 16/);
    assert.throws(() => createAdapter({ ...opts, secret: 'ci-secret' }), /at least 16/);
  });

  const serve = async (adapter: ReturnType<typeof createAdapter>) => {
    const server = createServer(adapter.handler);
    await new Promise<void>((r) => server.listen(0, '127.0.0.1', r));
    const origin = `http://127.0.0.1:${(server.address() as AddressInfo).port}`;
    const call = (method: string, target: string, body = '') =>
      fetch(origin + target, { method, body: body || undefined, headers: signRequest(S, { method, target, body }) });
    return { call, close: () => (adapter.close(), server.close()) };
  };

  test('deliveries published after a restart sort after every cursor issued before it', async () => {
    const catalog = [{ source: 'x.y', description: 'x', params: { type: 'object' }, max_wait_seconds: 30 }];
    const run = async () => {
      const adapter = createAdapter({ ...opts, catalog, handle: () => ({ type: 'accepted' }) });
      const http = await serve(adapter);
      const publish = async (id: string) => {
        const body = JSON.stringify({ id, requester: { id: 'u' }, conversation_ref: 'c', source: 'x.y', params: {}, reason: 'r' });
        assert.equal((await http.call('POST', '/requests', body)).status, 202);
        adapter.publish(id, { type: 'expire' });
      };
      const page = async (cursor: string) =>
        (await (await http.call('GET', `/deliveries?cursor=${cursor}`)).json()) as { items: string[]; next_cursor: string };
      return { publish, page, close: http.close };
    };
    const first = await run();
    await first.publish('req_00000001');
    const { next_cursor: cursor } = await first.page('');
    first.close();

    const second = await run();
    await second.publish('req_00000002');
    await second.publish('req_00000003');
    assert.equal((await second.page(cursor)).items.length, 2);
    second.close();
  });

  test('serves instructions given as text or a function, and 404 without them', async () => {
    const body = async (adapter: ReturnType<typeof createAdapter>) => {
      const { call, close } = await serve(adapter);
      try {
        const res = await call('GET', '/instructions');
        return { status: res.status, json: await res.json() };
      } finally {
        close();
      }
    };
    assert.deepEqual(await body(createAdapter({ ...opts, instructions: 'Acme is a CRM.' })), { status: 200, json: { text: 'Acme is a CRM.' } });
    assert.deepEqual(await body(createAdapter({ ...opts, instructions: async () => ({ text: 'Acme', version: 'v2' }) })), { status: 200, json: { text: 'Acme', version: 'v2' } });
    assert.equal((await body(createAdapter(opts))).status, 404);
    assert.equal((await body(createAdapter({ ...opts, instructions: () => null }))).status, 404);
  });

  test('refuses instructions over 8 KiB', async () => {
    const big = 'é'.repeat(4097);
    assert.throws(() => createAdapter({ ...opts, instructions: big }), /over 8192 bytes/);
    const { call, close } = await serve(createAdapter({ ...opts, instructions: () => ({ text: big }) }));
    try {
      assert.equal((await call('GET', '/instructions')).status, 500);
    } finally {
      close();
    }
  });

  test('serves only paths under basePath', async () => {
    const { call, close } = await serve(createAdapter({ ...opts, basePath: '/sild' }));
    const get = async (target: string) => (await call('GET', target)).status;
    try {
      assert.equal(await get('/sild/catalog'), 200);
      assert.equal(await get('/abcd/catalog'), 404);
      assert.equal(await get('/sildx/catalog'), 404);
      assert.equal(await get('/sil/catalog'), 404);
      assert.equal(await get('/catalog'), 404);
    } finally {
      close();
    }
  });
});
