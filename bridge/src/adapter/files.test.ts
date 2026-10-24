import { after, before, describe, test } from 'node:test';
import assert from 'node:assert/strict';
import { mkdtempSync, readFileSync, readdirSync, rmSync, existsSync, statSync } from 'node:fs';
import { tmpdir } from 'node:os';
import path from 'node:path';
import { FileStore, nameError } from './files.ts';

let root: string;
before(() => (root = mkdtempSync(path.join(tmpdir(), 'files-'))));
after(() => rmSync(root, { recursive: true, force: true }));

const file = (name: string, size: number, mime = 'application/json') => ({ name, mime, data: Buffer.alloc(size, 'a') });

describe('FileStore', () => {
  test('writes generated names and a manifest', async () => {
    const store = new FileStore(root, { filesPerDelivery: 5, bytesPerDelivery: 100, bytesPerConversation: 1000 });
    const out = await store.write('c1', 'req_00000001', { source: 's', summary: 'sum' }, [file('../../etc/x.json', 3), file('b.csv', 2, 'text/csv')]);
    const dir = path.join(root, 'c1', 'data', 'req_00000001');
    assert.deepEqual(readdirSync(dir).sort(), ['1.json', '2.csv', 'manifest.json']);
    assert.equal(out[0]!.path, path.join(dir, '1.json'));
    assert.equal(out[0]!.name, '../../etc/x.json');
    const manifest = JSON.parse(readFileSync(path.join(dir, 'manifest.json'), 'utf8'));
    assert.equal(manifest.files[1].name, 'b.csv');
    assert.equal(manifest.summary, 'sum');
    assert.equal(statSync(out[0]!.path).mode & 0o777, 0o644);
  });
  test('keeps an extension the session can open: by mime, else from the name', async () => {
    const store = new FileStore(root, { filesPerDelivery: 5, bytesPerDelivery: 100, bytesPerConversation: 1000 });
    const out = await store.write('c-ext', 'req_00000003', { source: 's', summary: '' }, [
      file('a.pdf', 1, 'application/pdf'), file('b.LOG', 1, 'application/octet-stream'), file('c', 1, 'application/octet-stream'),
    ]);
    assert.deepEqual(out.map((f) => path.basename(f.path)), ['1.pdf', '2.log', '3.bin']);
  });
  test('a repeated write under the same id returns what was stored', async () => {
    const store = new FileStore(root, { filesPerDelivery: 5, bytesPerDelivery: 100, bytesPerConversation: 1000 });
    const first = await store.write('c-replay', 'req_00000004', { source: 's', summary: '' }, [file('a.json', 4)]);
    const again = await store.write('c-replay', 'req_00000004', { source: 's', summary: '' }, [file('a.json', 4)]);
    assert.deepEqual(again, first);
    assert.equal(await store.remaining('c-replay'), 996);
  });
  test('enforces files and bytes per delivery', async () => {
    const store = new FileStore(root, { filesPerDelivery: 1, bytesPerDelivery: 10, bytesPerConversation: 1000 });
    await assert.rejects(store.write('c2', 'req_00000002', { source: 's', summary: '' }, [file('a', 1), file('b', 1)]), /too many files/);
    await assert.rejects(store.write('c2', 'req_00000003', { source: 's', summary: '' }, [file('a', 11)]), /too large/);
  });
  test('enforces bytes per conversation, counting what is already on disk', async () => {
    const limits = { filesPerDelivery: 5, bytesPerDelivery: 100, bytesPerConversation: 10 };
    await new FileStore(root, limits).write('c3', 'req_00000004', { source: 's', summary: '' }, [file('a', 8)]);
    const fresh = new FileStore(root, limits);
    await assert.rejects(fresh.write('c3', 'req_00000005', { source: 's', summary: '' }, [file('a', 3)]), /budget/);
  });
  test('refuses an unsafe conversation id', async () => {
    const store = new FileStore(root, { filesPerDelivery: 5, bytesPerDelivery: 100, bytesPerConversation: 1000 });
    await assert.rejects(store.write('../c', 'req_00000006', { source: 's', summary: '' }, []), /conversation/);
  });
  test('forget removes the data directory', async () => {
    const store = new FileStore(root, { filesPerDelivery: 5, bytesPerDelivery: 100, bytesPerConversation: 1000 });
    await store.write('c4', 'req_00000007', { source: 's', summary: '' }, [file('a', 1)]);
    await store.forget('c4');
    assert.equal(existsSync(path.join(root, 'c4', 'data')), false);
  });
  test('a write queued or arriving after forget is refused until reopen', async () => {
    const store = new FileStore(root, { filesPerDelivery: 5, bytesPerDelivery: 100, bytesPerConversation: 1000 });
    const inFlight = store.write('c5', 'req_00000008', { source: 's', summary: '' }, [file('a', 1)]);
    await store.forget('c5');
    await assert.rejects(inFlight, /closed/);
    assert.equal(existsSync(path.join(root, 'c5', 'data')), false);
    await assert.rejects(store.write('c5', 'req_00000009', { source: 's', summary: '' }, [file('a', 1)]), /closed/);
    assert.equal(existsSync(path.join(root, 'c5', 'data')), false);
    store.reopen('c5');
    await store.write('c5', 'req_00000010', { source: 's', summary: '' }, [file('a', 1)]);
    assert.ok(existsSync(path.join(root, 'c5', 'data', 'req_00000010')));
  });
});

describe('nameError', () => {
  for (const n of ['a/b', 'a\\b', '..', '.', '', 'x\u0000y']) test(`rejects ${JSON.stringify(n)}`, () => assert.notEqual(nameError(n), null));
  test('accepts a plain name', () => assert.equal(nameError('devices.json'), null));
});
