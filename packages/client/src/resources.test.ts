import assert from 'node:assert/strict';
import { after, before, test } from 'node:test';
import { createHttp, type Http } from './http.ts';
import * as sild from './resources.ts';
import { json, serve } from './testing/server.ts';
import { BEFORE_FIRST_MESSAGE } from './wire.ts';

let srv: Awaited<ReturnType<typeof serve>>;
let http: Http;
const log = Array.from({ length: 150 }, (_, i) => ({ id: `m_${String(i + 1).padStart(26, '0')}`, body: `b${i}` }));

before(async () => {
  srv = await serve((req, res) => {
    const u = new URL(req.url, 'http://x');
    if (u.pathname === '/v1/conversations/c%2F1/messages' && u.searchParams.has('since')) {
      const since = u.searchParams.get('since')!;
      const limit = Number(u.searchParams.get('limit'));
      const rest = log.filter((m) => m.id > since);
      return json(res, 200, { items: rest.slice(0, limit), next_cursor: null, has_more: rest.length > limit });
    }
    if (u.pathname === '/v1/translations/manifest') {
      if (req.headers['if-none-match'] === 'e1') return json(res, 304);
      return json(res, 200, { project: 'sild', fallback_locale: 'en', locales: { en: 3 } }, { etag: 'e1' });
    }
    json(res, 200, { ok: true });
  });
  http = createHttp({ baseUrl: srv.url, credential: { kind: 'bearer', token: 'k' } });
});
after(() => srv.close());

const last = () => srv.seen.at(-1)!;

test('messagesSince drains the sync read from BEFORE_FIRST_MESSAGE', async () => {
  const all = await sild.messagesSince(http, 'c/1', BEFORE_FIRST_MESSAGE);
  assert.equal(all.length, 150);
  assert.equal(new Set(all.map((m) => m.id)).size, 150);
});

test('list filters go out under their wire names', async () => {
  await sild.listConversations(http, { queue: 'triage', assignment_status: 'queued', sort: 'waiting_since', limit: 100, cursor: null });
  assert.equal(last().url, '/v1/conversations?queue=triage&assignment_status=queued&sort=waiting_since&limit=100');
});

test('assignment writes use the narrow PATCH bodies', async () => {
  await sild.claimAssignment(http, 'as_1');
  assert.deepEqual([last().method, last().url, JSON.parse(last().body)], ['PATCH', '/v1/assignments/as_1', { assignee_actor_id: 'me' }]);
  await sild.closeAssignment(http, 'as_1');
  assert.deepEqual(JSON.parse(last().body), { status: 'closed' });
});

test('the public brand read carries no credential', async () => {
  await sild.activeBrand(http, 't_1');
  assert.equal(last().url, '/v1/brands/active?app_id=t_1');
  assert.equal(last().headers.authorization, undefined);
  await sild.activeBrand(http);
  assert.equal(last().headers.authorization, 'Bearer k');
});

test('translationManifest returns null while the etag holds', async () => {
  const first = await sild.translationManifest(http, 'sild');
  assert.equal(first?.etag, 'e1');
  assert.equal(await sild.translationManifest(http, 'sild', 'e1'), null);
});

test('uploadUrl rebases a local-dev upload onto the client base', () => {
  const grant = { object_key: 'k', upload_url: 'http://public:8080/v1/uploads/local/k?sig=1', expires_at: '' };
  assert.deepEqual(sild.uploadUrl(http, grant), { url: `${srv.url}/v1/uploads/local/k?sig=1`, local: true });
  const cloud = { ...grant, upload_url: 'https://bucket.example/k?sig=1' };
  assert.deepEqual(sild.uploadUrl(http, cloud), { url: cloud.upload_url, local: false });
});
