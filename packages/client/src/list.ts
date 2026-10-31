import type { Page } from './wire.ts';

export type FetchPage<T> = (cursor: string | null) => Promise<Page<T>>;

// Walks the list contract (spec §4.0), stopping quietly after maxPages.
export async function* pages<T>(fetchPage: FetchPage<T>, maxPages = Infinity): AsyncGenerator<Page<T>> {
  let cursor: string | null = null;
  for (let i = 0; i < maxPages; i++) {
    const page = await fetchPage(cursor);
    yield page;
    if (!page.has_more || !page.next_cursor) return;
    cursor = page.next_cursor;
  }
}

// The whole collection or an error: a list cut short at the cap would be a silent truncation.
export async function collectAll<T>(fetchPage: FetchPage<T>, maxPages = 50): Promise<T[]> {
  const out: T[] = [];
  let cursor: string | null = null;
  for (let i = 0; i < maxPages; i++) {
    const page = await fetchPage(cursor);
    out.push(...page.items);
    if (!page.has_more || !page.next_cursor) return out;
    cursor = page.next_cursor;
  }
  throw new Error(`collection did not end within ${maxPages} pages`);
}

// A ?since= sync read (spec §5.4), yielded page by page so a caller can apply
// each one and stop early. Continuation is the last id received, never the original.
export async function* sincePages<T extends { id: string }>(
  fetchSince: (since: string) => Promise<Page<T>>,
  since: string,
): AsyncGenerator<T[]> {
  let after = since;
  for (;;) {
    const page = await fetchSince(after);
    // Index, not .at(): the widget bundle targets browsers without it.
    const last = page.items[page.items.length - 1];
    if (page.items.length) yield page.items;
    // A page that does not advance would repeat forever.
    if (!page.has_more || !last || last.id === after) return;
    after = last.id;
  }
}

export async function catchUp<T extends { id: string }>(
  fetchSince: (since: string) => Promise<Page<T>>,
  since: string,
): Promise<T[]> {
  const out: T[] = [];
  for await (const items of sincePages(fetchSince, since)) out.push(...items);
  return out;
}
