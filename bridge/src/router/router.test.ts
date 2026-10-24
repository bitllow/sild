import { after, before, describe, test } from 'node:test';
import assert from 'node:assert/strict';
import { createServer, type IncomingHttpHeaders, type Server } from 'node:http';
import type { AddressInfo } from 'node:net';
import { CATALOG } from '../../fake-adapter/scenarios.ts';
import { AnthropicRouter, proposals } from './anthropic.ts';
import { FakeRouter } from './fake.ts';
import { redact } from './redact.ts';

describe('redact', () => {
  test('replaces phones and emails with numbered placeholders', () => {
    const r = redact('Call +372 5123 4567 or mail andres.kuusk@example.ee, again +372 5123 4567.');
    assert.equal(r.text, 'Call [phone 1] or mail [email 1], again [phone 1].');
    assert.deepEqual(r.found.map((f) => f.value), ['andres.kuusk@example.ee', '+372 5123 4567']);
  });
  for (const keep of [
    'Adding a new van fails, VIN WV1ZZZSYZP9012345',
    'Trip TR-88412 and driver drv_2207, task 123456',
    'Completed 2026-09-01 to 2026-09-30',
    'The September export has 212 orders, report shows 219.',
    'Order 20260930123 is missing',
    'App 5.12.0 since 07:42',
  ]) {
    test(`keeps ${JSON.stringify(keep)}`, () => assert.equal(redact(keep).text, keep));
  }
  test('catches a local number with separators', () => assert.equal(redact('his number is 5123 4567').text, 'his number is [phone 1]'));
});

const route = (text: string, delivered: string[] = [], max = 3) => new FakeRouter().route({ text, catalog: CATALOG, delivered, max });

describe('FakeRouter on the design scenarios', () => {
  test('driver not receiving trips', async () => {
    const r = await route("Andres Kuusk hasn't received any new trips since this morning. Other drivers are fine.");
    assert.deepEqual(r.requests.map((p) => [p.source, p.params]), [['driver.lookup', { query: 'Andres Kuusk' }]]);
  });
  test('the session asks for his devices', async () => {
    const r = await route('push token state of drv_2207 devices', [], 1);
    assert.deepEqual(r.requests[0], { source: 'driver.devices', params: { driver_id: 'drv_2207' }, label: 'Checked his phone registration' });
  });
  test('ERP webhook 401', async () => {
    const r = await route('Our ERP stopped receiving order webhooks around 08:00. Their logs show 401.');
    assert.deepEqual(r.requests.map((p) => [p.source, p.params]), [['webhook.deliveries', { webhook: 'ERP' }]]);
  });
  test('VIN rejected', async () => {
    const r = await route("Adding a new van fails with 'VIN invalid', but the VIN is correct: WV1ZZZSYZP9012345");
    assert.deepEqual(r.requests.map((p) => p.source), ['vehicle.validate', 'vehicle.registry']);
  });
  test('September export', async () => {
    const r = await route('The September invoice export has 212 orders, but our dispatch report shows 219.');
    assert.deepEqual(r.requests.map((p) => [p.source, p.params]), [['invoices.export', { month: '2026-09' }], ['orders.completed', { month: '2026-09' }]]);
  });
  test('a phone goes into params', async () => {
    const r = await route('Driver with phone +372 5123 4567 gets no trips');
    assert.deepEqual(r.requests[0]!.params, { query: '+372 5123 4567' });
  });
  test('skips delivered sources and respects max', async () => {
    const r = await route("VIN WV1ZZZSYZP9012345 rejected", ['vehicle.validate'], 3);
    assert.deepEqual(r.requests.map((p) => p.source), ['vehicle.registry']);
    assert.equal((await route('VIN WV1ZZZSYZP9012345', [], 1)).requests.length, 1);
  });
});

describe('proposals', () => {
  test('keeps well-shaped entries only', () => {
    const got = proposals({ requests: [{ source: 'a', params: {}, label: 'x' }, { source: 'b', params: [], label: 'x' }, { source: 1 }, null] });
    assert.deepEqual(got, [{ source: 'a', params: {}, label: 'x' }]);
  });
  test('tolerates garbage', () => assert.deepEqual(proposals('nope'), []));
});

// A local stand-in for the bridge's model proxy; no model is called.
describe('AnthropicRouter', () => {
  let server: Server;
  let base: string;
  let seen: { headers: IncomingHttpHeaders; body: Record<string, unknown> } | null = null;
  let reply: unknown;

  before(async () => {
    server = createServer((req, res) => {
      const chunks: Buffer[] = [];
      req.on('data', (c: Buffer) => chunks.push(c));
      req.on('end', () => {
        seen = { headers: req.headers, body: JSON.parse(Buffer.concat(chunks).toString()) };
        res.writeHead(200, { 'content-type': 'application/json' }).end(JSON.stringify(reply));
      });
    });
    await new Promise<void>((r) => server.listen(0, '127.0.0.1', r));
    base = `http://127.0.0.1:${(server.address() as AddressInfo).port}`;
  });
  after(() => new Promise<void>((r) => server.close(() => r())));

  const message = (input: unknown) => ({
    id: 'msg_1', type: 'message', role: 'assistant', model: 'claude-haiku-5-5', stop_reason: 'tool_use', stop_sequence: null,
    usage: { input_tokens: 1, output_tokens: 1 },
    content: [{ type: 'tool_use', id: 'tu_1', name: 'route', input }],
  });

  test('sends the catalog and requester text, returns the tool call', async () => {
    reply = message({ requests: [{ source: 'driver.lookup', params: { query: '+372 5123 4567' }, label: 'Looked up the driver' }] });
    const r = await new AnthropicRouter({ baseURL: base }).route({ text: 'driver +372 5123 4567 gets no trips', catalog: CATALOG, delivered: ['service.status'], max: 2 });
    assert.deepEqual(r.requests, [{ source: 'driver.lookup', params: { query: '+372 5123 4567' }, label: 'Looked up the driver' }]);
    assert.equal(seen!.body.model, 'claude-haiku-5-5');
    assert.match(JSON.stringify(seen!.body.messages), /driver.lookup.*service.status.*gets no trips/s);
    assert.equal(seen!.headers['x-api-key'], 'injected-by-proxy');
  });

  test('uses a configured model and auth token', async () => {
    reply = message({ requests: [] });
    await new AnthropicRouter({ baseURL: base, authToken: 'tok', model: 'claude-sonnet-5-5' }).route({ text: 'x', catalog: CATALOG, delivered: [], max: 1 });
    assert.equal(seen!.body.model, 'claude-sonnet-5-5');
    assert.equal(seen!.headers.authorization, 'Bearer tok');
  });

  test('caps the result at max', async () => {
    const p = { source: 'service.status', params: {}, label: 'x' };
    reply = message({ requests: [p, p, p] });
    const r = await new AnthropicRouter({ baseURL: base }).route({ text: 'x', catalog: CATALOG, delivered: [], max: 2 });
    assert.equal(r.requests.length, 2);
  });
});
