import { after, before, describe, test } from 'node:test';
import assert from 'node:assert/strict';
import { existsSync, mkdtempSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import path from 'node:path';
import type { BrokerEvent } from '../contracts.ts';
import { startFakeAdapter, type FakeAdapter } from '../../fake-adapter/server.ts';
import { testAssert, testSild } from '../../fake-adapter/sild.ts';
import { AdapterClient } from '../adapter/client.ts';
import { DEFAULT_FILE_LIMITS, FileStore } from '../adapter/files.ts';
import { FakeRouter } from '../router/fake.ts';
import type { Router } from '../router/router.ts';
import { createBroker, type Broker, type BrokerLimits } from './broker.ts';
import { createServer } from 'node:http';
import type { AddressInfo } from 'node:net';
import { CATALOG } from '../../fake-adapter/scenarios.ts';
import { createAdapter, generateKeyPair, loadPrivateKey } from '../adapter/kit/index.ts';
import { JsonStore } from '../state.ts';

process.env.FAKE_DELAY_SCALE = '0';
const SECRET = 'broker-secret-0123456789';

let fake: FakeAdapter;
let root: string;
const open: Broker[] = [];

before(async () => {
  fake = await startFakeAdapter({ secret: SECRET, sild: testSild });
  root = mkdtempSync(path.join(tmpdir(), 'broker-'));
});
after(async () => {
  for (const b of open) b.close();
  await fake.close();
  rmSync(root, { recursive: true, force: true });
});

function setup(limits: Partial<BrokerLimits> = {}, router: Router = new FakeRouter()) {
  const adapter = new AdapterClient({ url: fake.url, secret: SECRET, publicKey: fake.publicKey, files: new FileStore(root, DEFAULT_FILE_LIMITS), allowInsecureLocal: true, assert: testAssert });
  const broker = createBroker({ adapter, router, limits });
  open.push(broker);
  const events: BrokerEvent[] = [];
  broker.subscribe((e) => events.push(e));
  // Resolves once `n` requests have settled (delivered or not_available).
  const settled = (n: number, conv?: string) =>
    new Promise<BrokerEvent[]>((resolve) => {
      const check = () => {
        const done = events.filter((e) => e.type !== 'requested' && (!conv || e.conv === conv));
        if (done.length >= n) {
          off();
          resolve(done);
        }
      };
      const off = broker.subscribe(check);
      check();
    });
  return { broker, events, settled };
}

describe('requester text', () => {
  test('routes the opening message and delivers', async () => {
    const { broker, events, settled } = setup();
    const out = await broker.onRequesterText('c_trips', 'u1', "Andres Kuusk hasn't received any new trips since this morning.");
    assert.equal(out.text, "Andres Kuusk hasn't received any new trips since this morning.");
    const [done] = await settled(1);
    const requested = events.find((e) => e.type === 'requested');
    assert.deepEqual(requested && { label: requested.label, source: requested.source, params: requested.params }, {
      label: 'Looked up driver Andres Kuusk', source: 'driver.lookup', params: { query: 'Andres Kuusk' },
    });
    assert.equal(done!.type, 'delivered');
    if (done!.type !== 'delivered') return;
    assert.equal(done.summary, 'drv_2207 · active · shift 07:00–15:00');
    assert.ok(done.files[0]!.path.startsWith(path.join(root, 'c_trips', 'data')));
  });

  test('redacts phones and emails, keeping them in params', async () => {
    const { broker, events, settled } = setup();
    const out = await broker.onRequesterText('c_phone', 'u1', 'Driver andres.kuusk@example.ee gets no trips');
    assert.equal(out.text, 'Driver [email 1] gets no trips');
    await settled(1);
    const r = events.find((e) => e.type === 'requested');
    assert.deepEqual(r?.type === 'requested' && r.params, { query: 'andres.kuusk@example.ee' });
    assert.equal(r?.type === 'requested' && r.label.includes('@'), false);
  });

  test('redacts even when the router fails', async () => {
    const broken: Router = { route: async () => { throw new Error('down'); } };
    const { broker, events } = setup({}, broken);
    const out = await broker.onRequesterText('c_down', 'u1', 'call me on +372 5123 4567');
    assert.equal(out.text, 'call me on [phone 1]');
    assert.equal(events.length, 0);
  });

  test('does not request a source twice in a conversation', async () => {
    const { broker, events, settled } = setup();
    await broker.onRequesterText('c_dup', 'u1', 'VIN WV1ZZZSYZP9012345 rejected');
    await settled(2, 'c_dup');
    await broker.onRequesterText('c_dup', 'u1', 'still failing: VIN WV1ZZZSYZP9012345');
    assert.equal(events.filter((e) => e.type === 'requested').length, 2);
  });

  test('caps requests per message and per conversation', async () => {
    const { broker, events, settled } = setup({ perMessage: 1, perConversation: 2 });
    await broker.onRequesterText('c_cap', 'u1', 'The September invoice export has 212 orders, but our dispatch report shows 219.');
    assert.equal(events.filter((e) => e.type === 'requested').length, 1);
    await settled(1, 'c_cap');
    await broker.onRequesterText('c_cap', 'u1', 'VIN WV1ZZZSYZP9012345');
    assert.equal(events.filter((e) => e.type === 'requested').length, 2);
    assert.deepEqual(await broker.request('c_cap', 'u1', 'service status', 'why'), { notAvailable: 'request limit for this conversation reached' });
  });

  test('drops proposals whose params do not fit the schema', async () => {
    const router: Router = { route: async () => ({ requests: [{ source: 'driver.devices', params: { driver_id: 'nope' }, label: 'x' }] }) };
    const { broker, events } = setup({}, router);
    await broker.onRequesterText('c_schema', 'u1', 'anything');
    assert.equal(events.length, 0);
  });
});

describe('request_info', () => {
  test('an async source arrives later', async () => {
    const { broker, settled } = setup();
    const r = await broker.request('c_dev', 'u1', 'push token state of drv_2207 devices', 'trips are not reaching him');
    assert.ok('requestId' in r);
    const [done] = await settled(1, 'c_dev');
    assert.equal(done!.type, 'delivered');
    assert.equal(done!.requestId, r.requestId);
  });

  test('a conversation asks about a capped number of different ids; repeats and enums are free', async () => {
    const { broker } = setup({ distinctIds: 2 });
    assert.ok('requestId' in await broker.request('c_ids', 'u1', 'push token state of drv_1 devices', 'x'));
    assert.ok('requestId' in await broker.request('c_ids', 'u1', 'push token state of drv_2 devices', 'x'));
    assert.deepEqual(await broker.request('c_ids', 'u1', 'push token state of drv_3 devices', 'x'),
      { notAvailable: 'this conversation may ask about at most 2 different ids' });
    assert.ok('requestId' in await broker.request('c_ids', 'u1', 'pending trips of drv_1', 'x'));
    assert.ok('requestId' in await broker.request('c_ids', 'u1', 'service status', 'x'));
  });

  test('a need nothing matches is not available', async () => {
    const { broker } = setup();
    assert.deepEqual(await broker.request('c_none', 'u1', 'the weather in Tartu', 'curious'), { notAvailable: 'no data source offers this' });
  });

  test('the same need while in flight returns the same request', async () => {
    const { broker } = setup();
    const a = await broker.request('c_same', 'u1', 'GPS for 123 ABC', 'x');
    const b = await broker.request('c_same', 'u1', 'where is 123 ABC', 'x');
    assert.deepEqual(a, b);
  });

  test('the same need once delivered points at the data', async () => {
    const { broker, settled } = setup();
    const a = await broker.request('c_again', 'u1', 'service status', 'x');
    await settled(1, 'c_again');
    const b = await broker.request('c_again', 'u1', 'service status', 'x');
    assert.ok('requestId' in a);
    assert.match('notAvailable' in b ? b.notAvailable : '', /^already delivered in data\/[0-9a-f]{32}\/$/);
  });

  test('a decline becomes not_available', async () => {
    const { broker, settled } = setup();
    await broker.request('c_decl', 'u1', 'payment cards of drv_2207', 'x');
    const [done] = await settled(1, 'c_decl');
    assert.deepEqual(done!.type === 'not_available' && done.reason, 'declined: Payout card data is not available to support staff');
  });

  test('a source that never answers times out', async () => {
    const { broker, settled } = setup({ maxWaitSeconds: 0.2, graceSeconds: 0 });
    await broker.request('c_never', 'u1', 'GPS for 123 ABC', 'x');
    const [done] = await settled(1, 'c_never');
    assert.equal(done!.type, 'not_available');
    assert.match(done!.type === 'not_available' ? done.reason : '', /no answer/);
  });

  test('an adapter-side expiry becomes not_available', async () => {
    const router: Router = { route: async () => ({ requests: [{ source: 'activity.errors', params: { service: 'dispatch', days: 90 }, label: 'x' }] }) };
    const { broker, settled } = setup({ graceSeconds: 30 }, router);
    await broker.request('c_exp', 'u1', 'errors', 'x');
    const [done] = await settled(1, 'c_exp');
    assert.deepEqual(done!.type === 'not_available' && done.reason, 'expired: the provider gave up');
  });
});

describe('forget', () => {
  test('cancels outstanding requests and deletes files', async () => {
    const { broker, events, settled } = setup();
    await broker.request('c_forget', 'u1', 'service status', 'x');
    await settled(1, 'c_forget');
    await broker.request('c_forget', 'u1', 'GPS for 123 ABC', 'x');
    await broker.forget('c_forget');
    assert.equal(existsSync(path.join(root, 'c_forget', 'data')), false);
    assert.equal(events.filter((e) => e.type !== 'requested').length, 1);
  });
});

describe('a requester the provider cannot identify', () => {
  test('gets no data from requester text, and the text is still redacted', async () => {
    const { broker } = setup();
    const out = await broker.onRequesterText('c_unlinked', '', 'Andres Kuusk gets no trips, call +372 5123 4567');
    assert.ok(!out.text.includes('5123'));
    await new Promise((r) => setTimeout(r, 50));
    assert.equal((await broker.request('c_unlinked', '', 'pending trips for drv_2207', 'x') as { notAvailable?: string }).notAvailable,
      'the requester is not linked to the data provider');
  });
});

describe('step-up', () => {
  test('a request the requester has not approved is parked, and retried once they do', async () => {
    let approved = false;
    const gated = await startFakeAdapter({ secret: SECRET, sild: testSild, elevated: async () => (approved ? Math.floor(Date.now() / 1000) + 60 : null) });
    try {
      const adapter = new AdapterClient({
        url: gated.url, secret: SECRET, publicKey: gated.publicKey, files: new FileStore(root, DEFAULT_FILE_LIMITS),
        allowInsecureLocal: true, assert: testAssert,
      });
      const broker = createBroker({ adapter, router: new FakeRouter() });
      open.push(broker);
      const events: BrokerEvent[] = [];
      broker.subscribe((e) => events.push(e));
      const until = async (pred: () => boolean) => {
        for (let i = 0; i < 200 && !pred(); i++) await new Promise((r) => setTimeout(r, 25));
      };

      const r = await broker.request('c_step', 'u1', 'push token state of drv_2207 devices', 'no alerts');
      assert.ok('requestId' in r);
      await until(() => events.some((e) => e.type === 'parked'));
      assert.ok(events.some((e) => e.type === 'parked' && e.requestId === r.requestId));
      assert.ok(!events.some((e) => e.type === 'not_available'));

      assert.deepEqual(await broker.request('c_step', 'u1', 'push token state of drv_2207 devices', 'again'),
        { notAvailable: 'already asked; it waits for the requester to approve data access and is retried then' });

      approved = true;
      await broker.approved('c_step');
      await until(() => events.some((e) => e.type === 'delivered'));
      const requested = events.filter((e) => e.type === 'requested').map((e) => e.requestId);
      assert.deepEqual(requested, [r.requestId, r.requestId], 'the retry keeps the request id');
      const delivered = events.find((e) => e.type === 'delivered' && e.requestId === r.requestId);
      assert.ok(delivered?.type === 'delivered' && delivered.approvedUntil, 'the delivery says it came under the approval');
    } finally {
      await gated.close();
    }
  });
});

describe('restart', () => {
  type Rig = { broker: Broker; events: BrokerEvent[]; until: (pred: () => boolean) => Promise<void> };
  const boot = (url: string, publicKey: string, dir: string, limits: Partial<BrokerLimits> = {}): Rig => {
    const adapter = new AdapterClient({
      url, secret: SECRET, publicKey, files: new FileStore(root, DEFAULT_FILE_LIMITS), allowInsecureLocal: true,
      assert: testAssert, state: new JsonStore(path.join(dir, 'adapter')),
    });
    const broker = createBroker({ adapter, router: new FakeRouter(), limits, state: new JsonStore(path.join(dir, 'broker')) });
    open.push(broker);
    const events: BrokerEvent[] = [];
    broker.subscribe((e) => events.push(e));
    const until = async (pred: () => boolean) => {
      for (let i = 0; i < 200 && !pred(); i++) await new Promise((r) => setTimeout(r, 25));
    };
    return { broker, events, until };
  };

  // An adapter that accepts every request and answers only when the test says so.
  const holdingAdapter = async () => {
    const seen: string[] = [];
    const { privateKey, publicKey } = generateKeyPair();
    const adapter = createAdapter({
      secret: SECRET, privateKey: loadPrivateKey(privateKey), catalog: CATALOG, sild: testSild,
      handle: (req) => (seen.push(req.id), { type: 'accepted' }),
    });
    const server = createServer(adapter.handler);
    await new Promise<void>((r) => server.listen(0, '127.0.0.1', r));
    const url = `http://127.0.0.1:${(server.address() as AddressInfo).port}`;
    return { url, publicKey, seen, adapter, close: () => (adapter.close(), server.closeAllConnections(), server.close()) };
  };

  test('a request in flight is still answered after the bridge restarts', async () => {
    const held = await holdingAdapter();
    const dir = mkdtempSync(path.join(tmpdir(), 'broker-state-'));
    try {
      const first = boot(held.url, held.publicKey, dir);
      const r = await first.broker.request('c_restart', 'u1', 'push token state of drv_2207 devices', 'x');
      assert.ok('requestId' in r);
      await first.until(() => held.seen.length === 1);
      first.broker.close();

      const second = boot(held.url, held.publicKey, dir);
      held.adapter.publish(held.seen[0]!, { type: 'deliver', summary: 'token expired', files: [] });
      await second.until(() => second.events.some((e) => e.type === 'delivered'));
      assert.ok(second.events.some((e) => e.type === 'delivered' && e.requestId === r.requestId), 'delivered under the same request id');
    } finally {
      held.close();
      rmSync(dir, { recursive: true, force: true });
    }
  });

  test('a request whose wait ran out during the restart is reported not available', async () => {
    const held = await holdingAdapter();
    const dir = mkdtempSync(path.join(tmpdir(), 'broker-state-'));
    try {
      const first = boot(held.url, held.publicKey, dir, { maxWaitSeconds: 0.2, graceSeconds: 0 });
      const r = await first.broker.request('c_late', 'u1', 'push token state of drv_2207 devices', 'x');
      await first.until(() => held.seen.length === 1);
      first.broker.close();
      await new Promise((res) => setTimeout(res, 300));

      const second = boot(held.url, held.publicKey, dir, { maxWaitSeconds: 0.2, graceSeconds: 0 });
      await second.until(() => second.events.some((e) => e.type === 'not_available'));
      assert.ok('requestId' in r && second.events.some((e) => e.type === 'not_available' && e.requestId === r.requestId));
    } finally {
      held.close();
      rmSync(dir, { recursive: true, force: true });
    }
  });

  test('a request parked for approval is retried after a restart', async () => {
    let approved = false;
    const gated = await startFakeAdapter({ secret: SECRET, sild: testSild, elevated: async () => (approved ? Math.floor(Date.now() / 1000) + 60 : null) });
    const dir = mkdtempSync(path.join(tmpdir(), 'broker-state-'));
    try {
      const first = boot(gated.url, gated.publicKey, dir);
      const r = await first.broker.request('c_parked', 'u1', 'push token state of drv_2207 devices', 'x');
      await first.until(() => first.events.some((e) => e.type === 'parked'));
      first.broker.close();

      const second = boot(gated.url, gated.publicKey, dir);
      approved = true;
      assert.equal(await second.broker.approved('c_parked'), 1);
      await second.until(() => second.events.some((e) => e.type === 'delivered'));
      assert.ok('requestId' in r && second.events.some((e) => e.type === 'delivered' && e.requestId === r.requestId));
    } finally {
      await gated.close();
      rmSync(dir, { recursive: true, force: true });
    }
  });
});
