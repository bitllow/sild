import assert from 'node:assert/strict';
import { after, before, test } from 'node:test';
import { ApiError, createHttp, type Credential } from './http.ts';
import { json, serve } from './testing/server.ts';

let srv: Awaited<ReturnType<typeof serve>>;

before(async () => {
  srv = await serve((req, res) => {
    const path = req.url.split('?')[0];
    if (path === '/v1/echo') return json(res, 200, { auth: req.headers.authorization ?? null, url: req.url, body: req.body });
    if (path === '/v1/empty') return json(res, 204);
    if (path === '/v1/versioned') {
      if (req.headers['if-none-match'] === '"v1"') return json(res, 304, undefined, { etag: '"v1"' });
      return json(res, 200, { v: 1 }, { etag: '"v1"' });
    }
    if (path === '/v1/invalid') {
      return json(res, 422, {
        error: { code: 'validation_failed', message: 'label is required', request_id: 'req_1', fields: { label: 'required' } },
      });
    }
    if (path === '/v1/plain-error') {
      res.writeHead(502, { 'content-type': 'text/html' }).end('<html>bad gateway</html>');
      return;
    }
    if (path === '/v1/guarded') {
      if (req.headers.authorization === 'Bearer fresh') return json(res, 200, { ok: true });
      return json(res, 401, { error: { code: 'unauthorized', message: 'token expired' } });
    }
    json(res, 404, { error: { code: 'not_found', message: 'no route' } });
  });
});
after(() => srv.close());

test('a bearer token rides the Authorization header', async () => {
  const http = createHttp({ baseUrl: srv.url + '/', credential: { kind: 'bearer', token: 'k1' } });
  const r = await http.get<{ auth: string }>('/v1/echo');
  assert.equal(r.auth, 'Bearer k1');
});

test('a token provider is asked on every request', async () => {
  let n = 0;
  const http = createHttp({ baseUrl: srv.url, credential: { kind: 'bearer', getToken: async () => `t${++n}` } });
  assert.equal((await http.get<{ auth: string }>('/v1/echo')).auth, 'Bearer t1');
  assert.equal((await http.get<{ auth: string }>('/v1/echo')).auth, 'Bearer t2');
});

test('cookie and anonymous credentials send no Authorization header', async () => {
  for (const credential of [{ kind: 'cookie' }, { kind: 'anonymous' }] as Credential[]) {
    const http = createHttp({ baseUrl: srv.url, credential });
    assert.equal((await http.get<{ auth: null }>('/v1/echo')).auth, null);
  }
});

test('the credential decides the fetch credentials mode; anonymous overrides per call', async () => {
  const modes: (string | undefined)[] = [];
  const spy: typeof fetch = async (_url, init) => {
    modes.push(init?.credentials);
    return new Response('{}', { status: 200 });
  };
  await createHttp({ credential: { kind: 'cookie' }, fetch: spy }).get('/v1/x');
  await createHttp({ credential: { kind: 'cookie' }, fetch: spy }).get('/v1/x', { anonymous: true });
  await createHttp({ credential: { kind: 'bearer', token: 't' }, fetch: spy }).get('/v1/x');
  assert.deepEqual(modes, ['include', 'omit', 'omit']);
});

test('a credential getter is read per request', async () => {
  let cred: Credential = { kind: 'cookie' };
  const http = createHttp({ baseUrl: srv.url, credential: () => cred });
  assert.equal((await http.get<{ auth: null }>('/v1/echo')).auth, null);
  cred = { kind: 'bearer', token: 'embedded' };
  assert.equal((await http.get<{ auth: string }>('/v1/echo')).auth, 'Bearer embedded');
});

test('per-call anonymous drops the bearer', async () => {
  const http = createHttp({ baseUrl: srv.url, credential: { kind: 'bearer', token: 'k1' } });
  assert.equal((await http.get<{ auth: null }>('/v1/echo', { anonymous: true })).auth, null);
});

test('keepalive reaches fetch only when asked for', async () => {
  const seen: (boolean | undefined)[] = [];
  const spy: typeof fetch = async (_url, init) => {
    seen.push(init?.keepalive);
    return new Response(null, { status: 204 });
  };
  const http = createHttp({ credential: { kind: 'cookie' }, fetch: spy });
  await http.post('/v1/x', {});
  await http.post('/v1/x', {}, { keepalive: true });
  assert.deepEqual(seen, [undefined, true]);
});

test('JSON bodies and queries are encoded; empty query values are dropped', async () => {
  const http = createHttp({ baseUrl: srv.url, credential: { kind: 'anonymous' } });
  const r = await http.post<{ url: string; body: string }>('/v1/echo', { a: 1 }, { query: { q: 'a b', limit: 5, cursor: null, x: '' } });
  assert.equal(r.url, '/v1/echo?q=a+b&limit=5');
  assert.deepEqual(JSON.parse(r.body), { a: 1 });
});

test('204 resolves undefined; 304 is not an error and keeps the etag', async () => {
  const http = createHttp({ baseUrl: srv.url, credential: { kind: 'anonymous' } });
  assert.equal(await http.post('/v1/empty'), undefined);
  const first = await http.exchange<{ v: number }>('GET', '/v1/versioned');
  assert.deepEqual([first.status, first.data, first.etag], [200, { v: 1 }, '"v1"']);
  const again = await http.exchange('GET', '/v1/versioned', { headers: { 'if-none-match': '"v1"' } });
  assert.deepEqual([again.status, again.data], [304, undefined]);
});

test('the error envelope becomes an ApiError', async () => {
  const http = createHttp({ baseUrl: srv.url, credential: { kind: 'anonymous' } });
  await assert.rejects(http.put('/v1/invalid?x=1', {}), (e: unknown) => {
    assert.ok(e instanceof ApiError);
    assert.equal(e.status, 422);
    assert.equal(e.code, 'validation_failed');
    assert.equal(e.message, 'label is required');
    assert.equal(e.requestId, 'req_1');
    assert.deepEqual(e.fields, { label: 'required' });
    assert.equal(e.method, 'PUT');
    assert.equal(e.path, '/v1/invalid');
    return true;
  });
});

test('a non-JSON error body still yields status and a fallback code', async () => {
  const http = createHttp({ baseUrl: srv.url, credential: { kind: 'anonymous' } });
  await assert.rejects(http.get('/v1/plain-error'), (e: unknown) =>
    e instanceof ApiError && e.status === 502 && e.code === 'http_error' && e.message.length > 0);
});

test('a transport failure is an ApiError with status 0', async () => {
  const http = createHttp({ baseUrl: 'http://127.0.0.1:1', credential: { kind: 'anonymous' } });
  await assert.rejects(http.get('/v1/echo'), (e: unknown) =>
    e instanceof ApiError && e.status === 0 && e.code === 'network_error');
});

test('onUnauthorized can refresh the credential and retry once', async () => {
  let token = 'stale';
  let calls = 0;
  const http = createHttp({
    baseUrl: srv.url,
    credential: { kind: 'bearer', getToken: () => token },
    onUnauthorized: () => {
      calls++;
      token = 'fresh';
      return true;
    },
  });
  assert.deepEqual(await http.get('/v1/guarded'), { ok: true });
  assert.equal(calls, 1);
});

test('onUnauthorized that declines surfaces the 401 with its message', async () => {
  let seen: ApiError | undefined;
  const http = createHttp({
    baseUrl: srv.url,
    credential: { kind: 'bearer', token: 'stale' },
    onUnauthorized: (e) => {
      seen = e;
    },
  });
  await assert.rejects(http.get('/v1/guarded'), (e: unknown) =>
    e instanceof ApiError && e.isUnauthorized && e.message === 'token expired');
  assert.equal(seen?.code, 'unauthorized');
});

test('anonymous calls never trigger onUnauthorized', async () => {
  let calls = 0;
  const http = createHttp({ baseUrl: srv.url, credential: { kind: 'bearer', token: 'x' }, onUnauthorized: () => void calls++ });
  await assert.rejects(http.get('/v1/guarded', { anonymous: true }));
  assert.equal(calls, 0);
});
