// Conformance suite a provider runs against its adapter:
//   ADAPTER_URL=https://adapter.example.com/sild ADAPTER_SECRET=… ADAPTER_PUBLIC_KEY=… \
//     node --test 'adapter/conformance/*.test.ts'
// CONFORMANCE_CASES may name a JSON file overriding DEFAULT_CASES (which fit
// fake-adapter/). A case left out is skipped.
import { describe, test } from 'node:test';
import assert from 'node:assert/strict';
import { createHash, randomUUID } from 'node:crypto';
import { readFileSync } from 'node:fs';
import {
  LIMITS,
  SOURCE_NAME,
  checkSchema,
  generateKeyPair,
  instructionsError,
  loadPublicKey,
  openDelivery,
  signRequest,
  type DeliveryClaims,
  type Source,
} from '../../src/adapter/kit/index.ts';

type Case = { source: string; params: Record<string, unknown> };
type Cases = Partial<Record<'sync' | 'async' | 'url' | 'decline' | 'expire' | 'invalid_params', Case>> & { requester?: { id: string } };

const DEFAULT_CASES: Cases = {
  sync: { source: 'driver.lookup', params: { query: 'Andres Kuusk' } },
  async: { source: 'driver.devices', params: { driver_id: 'drv_2207' } },
  url: { source: 'orders.completed', params: { month: '2026-09' } },
  decline: { source: 'payments.cards', params: { driver_id: 'drv_2207' } },
  expire: { source: 'activity.errors', params: { service: 'dispatch', days: 90 } },
  invalid_params: { source: 'driver.devices', params: { driver_id: 'not-an-id' } },
};

const URL_ = process.env.ADAPTER_URL;
const SECRET = process.env.ADAPTER_SECRET ?? '';
const PUBLIC = process.env.ADAPTER_PUBLIC_KEY ?? '';
const configured = Boolean(URL_ && SECRET && PUBLIC);
const cases: Cases = process.env.CONFORMANCE_CASES ? JSON.parse(readFileSync(process.env.CONFORMANCE_CASES, 'utf8')) : DEFAULT_CASES;
const requester = cases.requester ?? { id: 'sild-conformance' };
const base = configured ? new URL(URL_!.endsWith('/') ? URL_! : `${URL_}/`) : new URL('http://unset/');
const key = configured ? loadPublicKey(PUBLIC) : null;

type Res = { status: number; body: unknown; raw: Buffer };

async function call(method: string, rel: string, body = '', headers?: Record<string, string>): Promise<Res> {
  const url = new URL(rel, base);
  const target = url.pathname + url.search;
  const h = headers ?? signRequest(SECRET, { method, target, body });
  const res = await fetch(url, {
    method,
    headers: { ...h, ...(body ? { 'content-type': 'application/json' } : {}) },
    body: body || undefined,
    redirect: 'manual',
  });
  const raw = Buffer.from(await res.arrayBuffer());
  let parsed: unknown = null;
  try {
    parsed = JSON.parse(raw.toString('utf8'));
  } catch {}
  return { status: res.status, body: parsed, raw };
}

const conv = () => `conf_${randomUUID().slice(0, 8)}`;
const newRequest = (c: Case, conversation_ref = conv()) => ({
  id: randomUUID().replaceAll('-', ''),
  requester,
  conversation_ref,
  source: c.source,
  params: c.params,
  reason: 'Sild adapter conformance suite',
});
const post = (r: unknown) => call('POST', 'requests', JSON.stringify(r));
const field = (b: unknown, k: string): unknown => (typeof b === 'object' && b !== null ? (b as Record<string, unknown>)[k] : undefined);

async function catalog(): Promise<Source[]> {
  const res = await call('GET', 'catalog');
  assert.equal(res.status, 200);
  return res.body as Source[];
}

async function maxWait(source: string): Promise<number> {
  return (await catalog()).find((s) => s.source === source)?.max_wait_seconds ?? 30;
}

// The cursor just past everything currently retained.
async function head(): Promise<string> {
  let cursor = '';
  for (;;) {
    const res = await call('GET', `deliveries?${new URLSearchParams({ cursor, wait: '0' })}`);
    assert.equal(res.status, 200);
    const items = field(res.body, 'items') as unknown[];
    cursor = String(field(res.body, 'next_cursor'));
    if (!items.length) return cursor;
  }
}

async function awaitDelivery(id: string, cursor: string, seconds: number): Promise<DeliveryClaims> {
  const deadline = Date.now() + seconds * 1000;
  while (Date.now() < deadline) {
    const wait = Math.max(1, Math.min(LIMITS.maxWaitSeconds, Math.ceil((deadline - Date.now()) / 1000)));
    const res = await call('GET', `deliveries?${new URLSearchParams({ cursor, wait: String(wait) })}`);
    assert.equal(res.status, 200);
    const items = field(res.body, 'items');
    const next = field(res.body, 'next_cursor');
    assert.ok(Array.isArray(items), 'items is a list');
    assert.equal(typeof next, 'string', 'next_cursor is a string');
    for (const jws of items as string[]) {
      const opened = openDelivery(key!, jws);
      assert.ok(opened.ok, `delivery verifies: ${opened.ok ? '' : opened.reason}`);
      if (opened.claims.request_id === id) return opened.claims;
    }
    cursor = next as string;
  }
  throw new Error(`no delivery for ${id} within ${seconds}s`);
}

// Checks every rule the bridge applies to a delivered payload.
async function checkDelivered(claims: DeliveryClaims, req: { id: string; conversation_ref: string }) {
  assert.equal(claims.request_id, req.id);
  assert.equal(claims.conversation_ref, req.conversation_ref);
  assert.equal(claims.delivery.id, req.id);
  assert.equal(claims.delivery.state, 'delivered');
  assert.ok(Buffer.byteLength(claims.delivery.summary) <= LIMITS.summaryBytes, 'summary ≤ 1 KiB');
  assert.doesNotMatch(claims.delivery.summary, /[\u0000-\u001f]/, 'summary is one line');
  for (const f of claims.delivery.files) {
    assert.doesNotMatch(f.name, /[/\\\u0000]/, `${f.name}: no separators`);
    let data: Buffer;
    if (f.inline !== undefined) {
      data = Buffer.from(f.inline, 'base64');
      assert.ok(data.length <= LIMITS.inlineFileBytes, `${f.name}: inline ≤ 1 MiB`);
    } else {
      const url = new URL(f.url!);
      assert.equal(url.host, base.host, `${f.name}: url on the adapter's host`);
      assert.equal(url.protocol, base.protocol, `${f.name}: url uses the adapter's scheme`);
      const res = await call('GET', url.href);
      assert.equal(res.status, 200, `${f.name}: url answers 200`);
      data = res.raw;
    }
    assert.equal(data.length, f.size, `${f.name}: size matches`);
    assert.equal(createHash('sha256').update(data).digest('hex'), f.sha256, `${f.name}: sha256 matches`);
  }
}

// Posts a case and returns its delivery, sync or async.
async function deliver(c: Case): Promise<{ claims: DeliveryClaims; req: ReturnType<typeof newRequest>; status: number }> {
  const cursor = await head();
  const req = newRequest(c);
  const res = await post(req);
  if (res.status === 201) {
    const opened = openDelivery(key!, String(field(res.body, 'delivery')));
    assert.ok(opened.ok, `delivery verifies: ${opened.ok ? '' : opened.reason}`);
    return { claims: opened.claims, req, status: 201 };
  }
  assert.equal(res.status, 202, `POST /requests answers 201 or 202, got ${res.status}`);
  assert.equal(field(res.body, 'id'), req.id);
  return { claims: await awaitDelivery(req.id, cursor, (await maxWait(c.source)) + 10), req, status: 202 };
}

const offline = configured ? false : 'set ADAPTER_URL, ADAPTER_SECRET, ADAPTER_PUBLIC_KEY';
const skip = (c?: Case) => offline || (c ? false : 'no case configured');

describe('catalog', { skip: offline }, () => {
  test('is a list of well-formed sources', async () => {
    const list = await catalog();
    assert.ok(Array.isArray(list) && list.length > 0);
    const names = new Set<string>();
    for (const s of list) {
      assert.match(s.source, SOURCE_NAME);
      assert.ok(!names.has(s.source), `${s.source} listed once`);
      names.add(s.source);
      assert.equal(typeof s.description, 'string');
      assert.ok(s.description.length > 0, `${s.source}: has a description`);
      assert.ok(typeof s.max_wait_seconds === 'number' && s.max_wait_seconds > 0, `${s.source}: max_wait_seconds > 0`);
      assert.deepEqual(checkSchema(s.params), [], `${s.source}: params inside the schema subset`);
      assert.equal((s.params as { type?: unknown }).type, 'object', `${s.source}: params is an object schema`);
    }
  });
  test('names every configured case', async () => {
    const names = new Set((await catalog()).map((s) => s.source));
    for (const [k, c] of Object.entries(cases)) if (c && 'source' in c) assert.ok(names.has(c.source), `case ${k}: ${c.source} in catalog`);
  });
});

describe('instructions', { skip: offline }, () => {
  test('are absent (404) or a text within the limit', async () => {
    const res = await call('GET', 'instructions');
    if (res.status === 404) return;
    assert.equal(res.status, 200, `GET /instructions answers 200 or 404, got ${res.status}`);
    assert.equal(instructionsError(res.body), null, `body: ${instructionsError(res.body)}`);
  });
  test('an unsigned request is refused', async () => {
    assert.equal((await call('GET', 'instructions', '', {})).status, 401);
  });
});

describe('request signing', { skip: offline }, () => {
  const body = () => JSON.stringify(newRequest(cases.sync ?? { source: 'x', params: {} }));
  const target = () => new URL('requests', base).pathname;

  test('an unsigned request is refused', async () => {
    assert.equal((await call('GET', 'catalog', '', {})).status, 401);
  });
  test('a request signed with another secret is refused', async () => {
    const h = signRequest('not-the-secret', { method: 'GET', target: new URL('catalog', base).pathname, body: '' });
    assert.equal((await call('GET', 'catalog', '', h)).status, 401);
  });
  test('a stale timestamp is refused', async () => {
    const h = signRequest(SECRET, { method: 'GET', target: new URL('catalog', base).pathname, body: '' }, Date.now() - 301_000);
    assert.equal((await call('GET', 'catalog', '', h)).status, 401);
  });
  test('a body other than the signed one is refused', async () => {
    const h = signRequest(SECRET, { method: 'POST', target: target(), body: body() });
    assert.equal((await call('POST', 'requests', body(), h)).status, 401);
  });
  test('a query other than the signed one is refused', async () => {
    const h = signRequest(SECRET, { method: 'GET', target: `${new URL('deliveries', base).pathname}?wait=0`, body: '' });
    assert.equal((await call('GET', 'deliveries?wait=0&cursor=', '', h)).status, 401);
  });
  test('a replayed request is refused', async () => {
    const h = signRequest(SECRET, { method: 'GET', target: new URL('catalog', base).pathname, body: '' });
    assert.equal((await call('GET', 'catalog', '', h)).status, 200);
    assert.equal((await call('GET', 'catalog', '', h)).status, 401);
  });
});

describe('requests', { skip: offline }, () => {
  test('an unknown source is refused with 404', async () => {
    assert.equal((await post(newRequest({ source: 'sild.conformance.unknown', params: {} }))).status, 404);
  });
  test('params outside the schema are refused with 400', { skip: skip(cases.invalid_params) }, async () => {
    assert.equal((await post(newRequest(cases.invalid_params!))).status, 400);
  });
  test('a malformed body is refused with 400', async () => {
    assert.equal((await call('POST', 'requests', '{"id":1}')).status, 400);
  });
  test('a body over 64 KiB is refused with 413', async () => {
    const big = newRequest(cases.sync ?? { source: 'x', params: {} });
    big.reason = 'x'.repeat(65 * 1024);
    assert.equal((await post(big)).status, 413);
  });
  test('a repeated request id is refused with 409', { skip: skip(cases.sync) }, async () => {
    const req = newRequest(cases.sync!);
    assert.equal((await post(req)).status, 201);
    assert.equal((await post(req)).status, 409);
  });
});

describe('deliveries', { skip: offline }, () => {
  test('sync delivery', { skip: skip(cases.sync) }, async () => {
    const { claims, req, status } = await deliver(cases.sync!);
    assert.equal(status, 201);
    await checkDelivered(claims, req);
  });
  test('async delivery through /deliveries', { skip: skip(cases.async) }, async () => {
    const { claims, req, status } = await deliver(cases.async!);
    assert.equal(status, 202);
    await checkDelivered(claims, req);
  });
  test('file delivered by URL', { skip: skip(cases.url) }, async () => {
    const { claims, req } = await deliver(cases.url!);
    await checkDelivered(claims, req);
    assert.ok(claims.delivery.files.some((f) => f.url), 'at least one file by url');
  });
  test('decline', { skip: skip(cases.decline) }, async () => {
    const cursor = await head();
    const req = newRequest(cases.decline!);
    const res = await post(req);
    if (res.status === 403) {
      assert.equal(field(res.body, 'declined'), true);
      assert.equal(typeof field(res.body, 'reason'), 'string');
      return;
    }
    assert.equal(res.status, 202, `decline is 403 or a declined delivery, got ${res.status}`);
    const claims = await awaitDelivery(req.id, cursor, (await maxWait(req.source)) + 10);
    assert.equal(claims.delivery.state, 'declined');
  });
  test('expiry', { skip: skip(cases.expire) }, async () => {
    const { claims } = await deliver(cases.expire!);
    assert.equal(claims.delivery.state, 'expired');
    assert.deepEqual(claims.delivery.files, []);
  });
  test('a delivery does not verify under another key', { skip: skip(cases.sync) }, async () => {
    const res = await post(newRequest(cases.sync!));
    const other = loadPublicKey(generateKeyPair().publicKey);
    assert.equal(openDelivery(other, String(field(res.body, 'delivery'))).ok, false);
  });
  test('delivery expiry is short', { skip: skip(cases.sync) }, async () => {
    const res = await post(newRequest(cases.sync!));
    const opened = openDelivery(key!, String(field(res.body, 'delivery')));
    assert.ok(opened.ok);
    assert.ok(opened.claims.exp - Date.now() / 1000 <= 3600, 'exp within an hour');
  });
  test('wait is honoured and capped', async () => {
    const cursor = await head();
    const t = Date.now();
    const res = await call('GET', `deliveries?${new URLSearchParams({ cursor, wait: '1' })}`);
    assert.equal(res.status, 200);
    assert.ok(Date.now() - t < 10_000);
  });
});
