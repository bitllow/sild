import { after, before, describe, test } from 'node:test';
import assert from 'node:assert/strict';
import { createHash, randomUUID } from 'node:crypto';
import { mkdtempSync, readFileSync, rmSync } from 'node:fs';
import { createServer, type Server } from 'node:http';
import type { AddressInfo } from 'node:net';
import { tmpdir } from 'node:os';
import path from 'node:path';
import { startFakeAdapter, type FakeAdapter } from '../../fake-adapter/server.ts';
import { AdapterClient, type Received } from './client.ts';
import { DEFAULT_FILE_LIMITS, FileStore } from './files.ts';
import { generateKeyPair, loadPrivateKey, signDelivery } from './kit/jws.ts';
import { createAdapter } from './kit/server.ts';
import type { DeliveryClaims, WireFile } from './kit/types.ts';

process.env.FAKE_DELAY_SCALE = '0';
const SECRET = 'test-secret-0123456789';

let fake: FakeAdapter;
let root: string;
before(async () => {
  fake = await startFakeAdapter({ secret: SECRET });
  root = mkdtempSync(path.join(tmpdir(), 'client-'));
});
after(async () => {
  await fake.close();
  rmSync(root, { recursive: true, force: true });
});

const id = () => randomUUID().replaceAll('-', '');
const client = (over: Partial<ConstructorParameters<typeof AdapterClient>[0]> = {}) =>
  new AdapterClient({ url: fake.url, secret: SECRET, publicKey: fake.publicKey, files: new FileStore(root, DEFAULT_FILE_LIMITS), allowInsecureLocal: true, ...over });
const ask = (source: string, params: Record<string, unknown>) => ({ id: id(), requester: { id: 'u1' }, source, params, reason: 'test' });
const next = (c: AdapterClient) => new Promise<Received>((resolve) => { const off = c.onReceived((r) => { off(); resolve(r); }); });

describe('against the fake adapter', () => {
  test('fetches the catalog', async () => {
    const c = client();
    const sources = (await c.catalog()).map((s) => s.source);
    assert.ok(sources.includes('driver.lookup'));
    assert.ok(sources.includes('telematics.gps'));
  });

  test('fetches the instructions, labelled with the adapter host', async () => {
    const got = await client().instructions();
    assert.equal(got?.from, new URL(fake.url).host);
    assert.equal(got?.version, '2026-10-01');
    assert.match(got?.text ?? '', /Fleetline/);
  });

  test('a sync source delivers in the response and lands on disk', async () => {
    const c = client();
    const got = next(c);
    assert.deepEqual(await c.request('conv_sync', ask('driver.lookup', { query: 'Andres Kuusk' })), { type: 'pending' });
    const r = await got;
    assert.equal(r.state, 'delivered');
    if (r.state !== 'delivered') return;
    assert.equal(r.summary, 'drv_2207 · active · shift 07:00–15:00');
    assert.equal(r.files[0]!.name, 'drivers.json');
    assert.equal(path.basename(r.files[0]!.path), '1.json');
    assert.match(readFileSync(r.files[0]!.path, 'utf8'), /drv_2207/);
  });

  test('an async source arrives through the deliveries stream', async () => {
    const c = client();
    const got = next(c);
    const req = ask('driver.devices', { driver_id: 'drv_2207' });
    assert.deepEqual(await c.request('conv_async', req), { type: 'pending' });
    const r = await got;
    c.close();
    assert.equal(r.requestId, req.id);
    assert.equal(r.state, 'delivered');
  });

  test('a file delivered by URL is fetched and checked', async () => {
    const c = client();
    const got = next(c);
    await c.request('conv_url', ask('orders.completed', { month: '2026-09' }));
    const r = await got;
    c.close();
    assert.equal(r.state, 'delivered');
    if (r.state !== 'delivered') return;
    const csv = r.files.find((f) => f.name === 'orders.csv')!;
    assert.match(readFileSync(csv.path, 'utf8'), /^order,completed,invoiced/);
    assert.equal(path.basename(csv.path), '1.csv');
  });

  test('a decline comes back as declined', async () => {
    const c = client();
    const got = next(c);
    assert.deepEqual(await c.request('conv_decl', ask('payments.cards', { driver_id: 'drv_2207' })), { type: 'pending' });
    assert.equal((await got).state, 'declined');
  });

  test('an unknown source fails', async () => {
    const sent = await client().request('conv_x', ask('nope.nothing', {}));
    assert.equal(sent.type, 'failed');
  });

  test('a wrong secret fails', async () => {
    const sent = await client({ secret: 'wrong' }).request('conv_x', ask('service.status', {}));
    assert.deepEqual(sent.type, 'failed');
    assert.match((sent as { reason: string }).reason, /401/);
  });

  test('a delivery signed by another key is refused', async () => {
    const sent = await client({ publicKey: generateKeyPair().publicKey }).request('conv_x', ask('service.status', {}));
    assert.equal(sent.type, 'failed');
  });

  test('http to a non-local host is refused even with allowInsecureLocal', async () => {
    const sent = await client({ url: 'http://adapter.example.com' }).request('conv_x', ask('service.status', {}));
    assert.equal(sent.type, 'failed');
  });
});

// A hand-rolled adapter that answers with whatever delivery the test builds.
describe('against a misbehaving adapter', () => {
  const keys = generateKeyPair();
  const priv = loadPrivateKey(keys.privateKey);
  let server: Server;
  let url: string;
  let build: (body: { id: string; conversation_ref: string }) => DeliveryClaims;
  const bodies = new Map<string, Buffer>();

  before(async () => {
    server = createServer((req, res) => {
      const chunks: Buffer[] = [];
      req.on('data', (c: Buffer) => chunks.push(c));
      req.on('end', () => {
        const file = bodies.get(req.url ?? '');
        if (file) return void res.writeHead(200).end(file);
        if (req.url?.endsWith('/redirect')) return void res.writeHead(302, { location: 'http://169.254.169.254/' }).end();
        const body = JSON.parse(Buffer.concat(chunks).toString());
        res.writeHead(201, { 'content-type': 'application/json' }).end(JSON.stringify({ delivery: signDelivery(priv, build(body)) }));
      });
    });
    await new Promise<void>((r) => server.listen(0, '127.0.0.1', r));
    url = `http://127.0.0.1:${(server.address() as AddressInfo).port}`;
  });
  after(() => new Promise<void>((r) => server.close(() => r())));

  const c = () => new AdapterClient({ url, secret: SECRET, publicKey: keys.publicKey, files: new FileStore(root, { ...DEFAULT_FILE_LIMITS, filesPerDelivery: 2, bytesPerDelivery: 64 }), allowInsecureLocal: true });
  const claims = (body: { id: string; conversation_ref: string }, files: WireFile[], over: Partial<DeliveryClaims> = {}): DeliveryClaims => ({
    request_id: body.id,
    conversation_ref: body.conversation_ref,
    iat: Math.floor(Date.now() / 1000),
    exp: Math.floor(Date.now() / 1000) + 60,
    delivery: { id: body.id, state: 'delivered', summary: 'ok', files },
    ...over,
  });
  const inline = (name: string, text: string): WireFile => ({
    name, mime: 'text/plain', size: Buffer.byteLength(text), sha256: createHash('sha256').update(text).digest('hex'), inline: Buffer.from(text).toString('base64'),
  });
  const result = async (conv = 'conv_bad') => {
    const client = c();
    const got = next(client);
    const sent = await client.request(conv, ask('x.y', {}));
    return sent.type === 'failed' ? sent : got;
  };

  test('a delivery bound to another conversation is refused', async () => {
    build = (b) => claims({ ...b, conversation_ref: 'someone_else' }, []);
    assert.match(JSON.stringify(await result()), /"type":"failed"/);
  });
  test('a delivery for another request id is refused', async () => {
    build = (b) => claims({ ...b, id: 'req_notours1' }, []);
    assert.match(JSON.stringify(await result()), /"type":"failed"/);
  });
  test('a file name with a separator invalidates the delivery', async () => {
    build = (b) => claims(b, [inline('../evil.txt', 'x')]);
    assert.match(JSON.stringify(await result()), /"state":"invalid".*separator/);
  });
  test('a sha256 mismatch invalidates the delivery', async () => {
    build = (b) => claims(b, [{ ...inline('a.txt', 'x'), sha256: '0'.repeat(64) }]);
    assert.match(JSON.stringify(await result()), /sha256 mismatch/);
  });
  test('too many files invalidates the delivery', async () => {
    build = (b) => claims(b, [inline('a', 'x'), inline('b', 'x'), inline('c', 'x')]);
    assert.match(JSON.stringify(await result()), /too many files/);
  });
  test('too many bytes invalidates the delivery', async () => {
    build = (b) => claims(b, [inline('a', 'x'.repeat(65))]);
    assert.match(JSON.stringify(await result()), /too large/);
  });
  test('a summary over 1 KiB invalidates the delivery', async () => {
    build = (b) => ({ ...claims(b, []), delivery: { id: b.id, state: 'delivered', summary: 'x'.repeat(1025), files: [] } });
    assert.match(JSON.stringify(await result()), /1 KiB/);
  });
  test('a decline reason is clipped to one line of 1 KiB', async () => {
    build = (b) => ({ ...claims(b, []), delivery: { id: b.id, state: 'declined', summary: 'no\n'.repeat(2000), files: [] } });
    const r = (await result()) as Received & { reason: string };
    assert.equal(r.state, 'declined');
    assert.ok(Buffer.byteLength(r.reason) <= 1024);
    assert.doesNotMatch(r.reason, /\n/);
  });
  test('a file URL on another host is refused', async () => {
    build = (b) => claims(b, [{ ...inline('a', 'x'), inline: undefined, url: 'http://localhost:1/f' }]);
    assert.match(JSON.stringify(await result()), /not on the adapter's host/);
  });
  test('a redirect from the file URL is not followed', async () => {
    build = (b) => claims(b, [{ ...inline('a', 'x'), inline: undefined, url: `${url}/redirect` }]);
    assert.match(JSON.stringify(await result()), /HTTP 302/);
  });
  test('a file URL larger than declared is cut off', async () => {
    bodies.set('/big', Buffer.alloc(50, 'x'));
    build = (b) => claims(b, [{ ...inline('a', 'x'), inline: undefined, url: `${url}/big` }]);
    assert.match(JSON.stringify(await result()), /too large/);
  });
  test('a good file URL is stored', async () => {
    bodies.set('/ok', Buffer.from('hello'));
    build = (b) => claims(b, [{ ...inline('a.txt', 'hello'), inline: undefined, url: `${url}/ok` }]);
    const r = await result('conv_okurl');
    assert.equal((r as Received).state, 'delivered');
  });
});

describe('across an adapter restart', () => {
  const keys = generateKeyPair();
  const privateKey = loadPrivateKey(keys.privateKey);
  const catalog = [{ source: 'x.y', description: 'x', params: { type: 'object' }, max_wait_seconds: 30 }];
  const start = async (port: number, now: () => number) => {
    const adapter = createAdapter({ secret: SECRET, privateKey, catalog, handle: () => ({ type: 'accepted' }), now });
    const server = createServer(adapter.handler);
    await new Promise<void>((r) => server.listen(port, '127.0.0.1', r));
    const close = () => new Promise<void>((r) => {
      adapter.close();
      server.closeAllConnections();
      server.close(() => r());
    });
    return { adapter, port: (server.address() as AddressInfo).port, close };
  };

  for (const [clock, skew] of [['moved on', 0], ['went back', 60_000]] as const) {
    test(`new deliveries reach a client holding the old cursor when the clock ${clock}`, { timeout: 10_000 }, async () => {
      const first = await start(0, () => Date.now() + skew);
      const c = new AdapterClient({ url: `http://127.0.0.1:${first.port}`, secret: SECRET, publicKey: keys.publicKey, files: new FileStore(root, DEFAULT_FILE_LIMITS), allowInsecureLocal: true });
      const r1 = ask('x.y', {});
      const got1 = next(c);
      assert.deepEqual(await c.request('conv_restart', r1), { type: 'pending' });
      first.adapter.publish(r1.id, { type: 'deliver', summary: 'one', files: [] });
      assert.equal((await got1).requestId, r1.id);
      await first.close();

      const second = await start(first.port, Date.now);
      const later = [ask('x.y', {}), ask('x.y', {})];
      const got = new Set<string>();
      const both = new Promise<void>((resolve) => c.onReceived((r) => got.add(r.requestId).size === 2 && resolve()));
      for (const r of later) assert.deepEqual(await c.request('conv_restart', r), { type: 'pending' });
      for (const r of later) second.adapter.publish(r.id, { type: 'deliver', summary: 'later', files: [] });
      await both;
      assert.deepEqual([...got].sort(), later.map((r) => r.id).sort());
      c.close();
      await second.close();
    });
  }
});

describe('instructions', () => {
  let server: Server;
  let url: string;
  let answer: { status: number; body?: unknown } = { status: 404 };
  let hits = 0;

  before(async () => {
    server = createServer((_req, res) => {
      hits++;
      res.writeHead(answer.status, { 'content-type': 'application/json' }).end(JSON.stringify(answer.body ?? { error: 'x' }));
    });
    await new Promise<void>((r) => server.listen(0, '127.0.0.1', r));
    url = `http://127.0.0.1:${(server.address() as AddressInfo).port}`;
  });
  after(() => new Promise<void>((r) => server.close(() => r())));

  const c = (instructionsTtlMs?: number) =>
    new AdapterClient({ url, secret: SECRET, publicKey: generateKeyPair().publicKey, files: new FileStore(root, DEFAULT_FILE_LIMITS), allowInsecureLocal: true, instructionsTtlMs });
  const good = { status: 200, body: { text: 'Acme is a CRM.', version: 'v1' } };

  test('404 is none', async () => {
    answer = { status: 404 };
    assert.equal(await c().instructions(), null);
  });

  test('are cached within the TTL, one fetch for concurrent callers', async () => {
    answer = good;
    const client = c();
    hits = 0;
    const [a, b] = await Promise.all([client.instructions(), client.instructions()]);
    assert.deepEqual(a, { text: 'Acme is a CRM.', version: 'v1', from: new URL(url).host });
    assert.deepEqual(b, a);
    answer = { status: 404 };
    assert.deepEqual(await client.instructions(), a);
    assert.equal(hits, 1);
  });

  test('a failed fetch keeps the last good value', async () => {
    answer = good;
    const client = c(0);
    const first = await client.instructions();
    answer = { status: 503 };
    assert.deepEqual(await client.instructions(), first);
  });

  test('a text over 8 KiB or a malformed body is none', async () => {
    const client = c(0);
    for (const body of [{ text: 'x'.repeat(8 * 1024 + 1) }, { text: '' }, { text: 'ok', version: 2 }, ['x']]) {
      answer = good;
      assert.ok(await client.instructions());
      answer = { status: 200, body };
      assert.equal(await client.instructions(), null, JSON.stringify(body).slice(0, 40));
    }
  });
});
