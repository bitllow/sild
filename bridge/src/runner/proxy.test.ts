import assert from 'node:assert/strict';
import { createServer, type IncomingHttpHeaders } from 'node:http';
import type { AddressInfo } from 'node:net';
import { after, before, test } from 'node:test';
import { startModelProxy, type ModelProxy } from './proxy.ts';

const seen: { url?: string; headers: IncomingHttpHeaders; body: string }[] = [];
const upstream = createServer((req, res) => {
  let body = '';
  req.on('data', (c) => (body += c));
  req.on('end', () => {
    seen.push({ url: req.url, headers: req.headers, body });
    res.writeHead(201, { 'content-type': 'application/json', 'x-upstream': '1' }).end('{"ok":true}');
  });
});
let upstreamUrl = '';
const TOKEN = 'per-start-token';
const proxies: ModelProxy[] = [];

before(async () => {
  await new Promise<void>((r) => upstream.listen(0, '127.0.0.1', r));
  upstreamUrl = `http://127.0.0.1:${(upstream.address() as AddressInfo).port}`;
});
after(async () => {
  for (const p of proxies) await p.close();
  upstream.close();
});

async function proxy(auth: Parameters<typeof startModelProxy>[0]['auth']) {
  const p = await startModelProxy({ auth, token: TOKEN, upstream: upstreamUrl });
  proxies.push(p);
  return `http://127.0.0.1:${p.port}`;
}

test('api key: replaces whatever credential the CLI sent', async () => {
  const base = await proxy({ kind: 'api_key', key: 'sk-real' });
  const res = await fetch(`${base}/v1/messages?beta=true`, {
    method: 'POST',
    headers: { 'x-api-key': 'placeholder', authorization: `Bearer ${TOKEN}`, 'content-type': 'application/json' },
    body: '{"model":"m"}',
  });
  assert.equal(res.status, 201);
  assert.equal(res.headers.get('x-upstream'), '1');
  assert.equal(await res.text(), '{"ok":true}');
  const got = seen.at(-1)!;
  assert.equal(got.url, '/v1/messages?beta=true');
  assert.equal(got.headers['x-api-key'], 'sk-real');
  assert.equal(got.headers.authorization, undefined);
  assert.equal(got.body, '{"model":"m"}');
});

test('oauth: bearer token plus the oauth beta, keeping the CLI betas', async () => {
  const base = await proxy({ kind: 'oauth', token: 'oat-real' });
  await fetch(`${base}/v1/messages/count_tokens`, {
    method: 'POST', headers: { 'anthropic-beta': 'tools-1', 'x-api-key': 'placeholder', authorization: `Bearer ${TOKEN}` }, body: '{}',
  });
  const got = seen.at(-1)!;
  assert.equal(got.headers.authorization, 'Bearer oat-real');
  assert.equal(got.headers['x-api-key'], undefined);
  assert.equal(got.headers['anthropic-beta'], 'tools-1,oauth-2025-04-20');
});

test('anything but the messages API is refused', async () => {
  const base = await proxy({ kind: 'api_key', key: 'sk-real' });
  const before = seen.length;
  assert.equal((await fetch(`${base}/v1/models`)).status, 404);
  assert.equal((await fetch(`${base}/v1/files`, { method: 'POST', body: '{}' })).status, 404);
  assert.equal((await fetch(`${base}/v1/messagesX`, { method: 'POST', body: '{}' })).status, 404);
  assert.equal(seen.length, before);
});

test('a caller without the proxy token is refused before anything goes upstream', async () => {
  const base = await proxy({ kind: 'api_key', key: 'sk-real' });
  const before = seen.length;
  const send = (headers: Record<string, string>) =>
    fetch(`${base}/v1/messages`, { method: 'POST', headers, body: '{}' }).then((r) => r.status);
  assert.equal(await send({}), 401);
  assert.equal(await send({ authorization: 'Bearer wrong' }), 401);
  assert.equal(await send({ authorization: `Bearer ${TOKEN}x` }), 401);
  assert.equal(await send({ 'x-api-key': TOKEN }), 401);
  assert.equal(seen.length, before);
  assert.equal(await send({ authorization: `Bearer ${TOKEN}` }), 201);
});
