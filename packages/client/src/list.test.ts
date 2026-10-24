import assert from 'node:assert/strict';
import { test } from 'node:test';
import { catchUp, collectAll, pages, sincePages } from './list.ts';
import type { Page } from './wire.ts';

// A cursor-paged collection of n numbers, `size` per page.
function numbers(n: number, size: number, cursors: (string | null)[] = []) {
  return async (cursor: string | null): Promise<Page<number>> => {
    cursors.push(cursor);
    const start = cursor ? Number(cursor) : 0;
    const items = Array.from({ length: Math.max(0, Math.min(size, n - start)) }, (_, i) => start + i);
    const more = start + size < n;
    return { items, next_cursor: more ? String(start + size) : null, has_more: more };
  };
}

test('collectAll follows next_cursor verbatim to the end', async () => {
  const cursors: (string | null)[] = [];
  assert.deepEqual(await collectAll(numbers(5, 2, cursors)), [0, 1, 2, 3, 4]);
  assert.deepEqual(cursors, [null, '2', '4']);
});

test('collectAll throws rather than return a truncated list', async () => {
  await assert.rejects(collectAll(numbers(10, 2), 3), /did not end within 3 pages/);
});

test('pages stops quietly at its cap', async () => {
  const seen: number[] = [];
  for await (const p of pages(numbers(10, 2), 2)) seen.push(...p.items);
  assert.deepEqual(seen, [0, 1, 2, 3]);
});

test('pages stops when has_more is set but no cursor came back', async () => {
  let calls = 0;
  const broken = async (): Promise<Page<number>> => {
    calls++;
    return { items: [1], next_cursor: null, has_more: true };
  };
  for await (const _ of pages(broken)) { /* drain */ }
  assert.equal(calls, 1);
});

type Msg = { id: string };
const ids = (n: number) => Array.from({ length: n }, (_, i) => ({ id: `m_${String(i + 1).padStart(3, '0')}` }));

// ?since= over a fixed log: everything after `since`, `limit` at a time.
function log(all: Msg[], limit: number, asked: string[] = []) {
  return async (since: string): Promise<Page<Msg>> => {
    asked.push(since);
    const after = all.filter((m) => m.id > since);
    return { items: after.slice(0, limit), next_cursor: null, has_more: after.length > limit };
  };
}

test('catchUp continues from the last id received while has_more', async () => {
  const asked: string[] = [];
  const got = await catchUp(log(ids(250), 100, asked), 'm_000');
  assert.equal(got.length, 250);
  assert.equal(new Set(got.map((m) => m.id)).size, 250);
  assert.deepEqual(asked, ['m_000', 'm_100', 'm_200']);
});

test('catchUp from the middle returns only the tail', async () => {
  const got = await catchUp(log(ids(150), 100), 'm_148');
  assert.deepEqual(got.map((m) => m.id), ['m_149', 'm_150']);
});

test('catchUp stops on an empty page even if has_more is set', async () => {
  let calls = 0;
  const got = await catchUp(async () => {
    calls++;
    return { items: [], next_cursor: null, has_more: true };
  }, 'm_000');
  assert.deepEqual([got, calls], [[], 1]);
});

test('catchUp stops when a page does not advance past since', async () => {
  let calls = 0;
  await catchUp(async () => {
    calls++;
    return { items: [{ id: 'm_005' }], next_cursor: null, has_more: true };
  }, 'm_005');
  assert.equal(calls, 1);
});

test('sincePages lets the caller stop between pages', async () => {
  const asked: string[] = [];
  for await (const items of sincePages(log(ids(300), 100, asked), 'm_000')) {
    if (items.at(-1)!.id === 'm_100') break;
  }
  assert.deepEqual(asked, ['m_000']);
});
