import assert from 'node:assert/strict';
import { test } from 'node:test';
import { ApiError } from './http.ts';
import { connect, parseEnvelope, sseUrl, wsUrl, type CentrifugeOptions } from './realtime.ts';
import type { Envelope } from './wire.ts';

class Unauthorized extends Error {}

// Records how it was built and lets a test fire centrifuge events by hand.
class FakeCentrifuge {
  static UnauthorizedError = Unauthorized;
  static last: FakeCentrifuge;
  readonly handlers = new Map<string, (ctx: any) => void>();
  calls: string[] = [];
  readonly endpoint: unknown;
  readonly options: CentrifugeOptions;
  constructor(endpoint: unknown, options: CentrifugeOptions = {}) {
    this.endpoint = endpoint;
    this.options = options;
    FakeCentrifuge.last = this;
  }
  on(event: string, cb: (ctx: any) => void) {
    this.handlers.set(event, cb);
    return this;
  }
  connect() {
    this.calls.push('connect');
  }
  disconnect() {
    this.calls.push('disconnect');
  }
  fire(event: string, ctx: unknown = {}) {
    this.handlers.get(event)?.(ctx);
  }
}

const base = { Centrifuge: FakeCentrifuge, getToken: async () => 'tok', onEvent: () => {} };

test('websocket transport passes the url; sse wraps it as a transport endpoint', () => {
  connect({ ...base, url: 'ws://h/v1/ws' });
  assert.equal(FakeCentrifuge.last.endpoint, 'ws://h/v1/ws');
  connect({ ...base, url: 'https://h/v1/ws/sse', transport: 'sse', options: { emulationEndpoint: '/x' } });
  assert.deepEqual(FakeCentrifuge.last.endpoint, [{ transport: 'sse', endpoint: 'https://h/v1/ws/sse' }]);
  assert.equal(FakeCentrifuge.last.options.emulationEndpoint, '/x');
  assert.deepEqual(FakeCentrifuge.last.calls, ['connect']);
});

test('onConnected fires on every connect and marks reconnects', () => {
  const seen: boolean[] = [];
  const states: string[] = [];
  const rt = connect({ ...base, url: 'ws://h', onConnected: (i) => seen.push(i.reconnect), onState: (s) => states.push(s) });
  const c = FakeCentrifuge.last;
  c.fire('connecting');
  c.fire('connected');
  c.fire('disconnected', { code: 3000, reason: 'x' });
  rt.reconnect();
  c.fire('connected');
  assert.deepEqual(seen, [false, true]);
  assert.deepEqual(states, ['connecting', 'connected', 'disconnected', 'connected']);
  assert.deepEqual(c.calls, ['connect', 'disconnect', 'connect']);
  rt.close();
  assert.equal(c.calls.at(-1), 'disconnect');
});

test('publications reach onEvent as envelopes; malformed ones are dropped', () => {
  const got: [string, Envelope][] = [];
  connect({ ...base, url: 'ws://h', onEvent: (ch, env) => got.push([ch, env]) });
  const c = FakeCentrifuge.last;
  c.fire('publication', { channel: 'conv:c1', data: { type: 'message.created', conversation_id: 'c1', data: { id: 'm1' }, ts: 7 } });
  c.fire('publication', { channel: 'conv:c1', data: 'junk' });
  c.fire('publication', { channel: 'conv:c1', data: { data: {} } });
  assert.deepEqual(got, [['conv:c1', { type: 'message.created', conversation_id: 'c1', data: { id: 'm1' }, ts: 7 }]]);
});

test('a 401 or 403 from getToken stops reconnecting; other failures retry', async () => {
  let fail: unknown = new ApiError(403, 'forbidden', 'key revoked');
  connect({ ...base, url: 'ws://h', getToken: async () => { throw fail; } });
  const getToken = FakeCentrifuge.last.options.getToken!;
  await assert.rejects(getToken({}), (e: unknown) => e instanceof Unauthorized && e.message === 'key revoked');
  fail = new ApiError(0, 'network_error', 'down');
  await assert.rejects(getToken({}), (e: unknown) => e instanceof ApiError && e.status === 0);
});

test('getToken hands back the caller token', async () => {
  connect({ ...base, url: 'ws://h' });
  assert.equal(await FakeCentrifuge.last.options.getToken!({}), 'tok');
});

test('parseEnvelope keeps unknown event types', () => {
  assert.deepEqual(parseEnvelope({ type: 'future.event', data: null, ts: 1 }), { type: 'future.event', conversation_id: undefined, data: null, ts: 1 });
  assert.equal(parseEnvelope(null), null);
});

test('socket urls derive from the API base', () => {
  assert.equal(wsUrl('https://chat.sild.io'), 'wss://chat.sild.io/v1/ws');
  assert.equal(wsUrl('http://localhost:8080/'), 'ws://localhost:8080/v1/ws');
  assert.equal(sseUrl('https://chat.sild.io/'), 'https://chat.sild.io/v1/ws/sse');
  assert.equal(sseUrl(''), '/v1/ws/sse');
});
