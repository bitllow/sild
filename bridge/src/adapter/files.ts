import { mkdir, readdir, readFile, rename, rm, stat, writeFile } from 'node:fs/promises';
import path from 'node:path';
import type { DeliveredFile } from '../contracts.ts';

export type FileLimits = {
  filesPerDelivery: number;
  bytesPerDelivery: number;
  bytesPerConversation: number;
};

export const DEFAULT_FILE_LIMITS: FileLimits = {
  filesPerDelivery: 10,
  bytesPerDelivery: 8 * 1024 * 1024,
  bytesPerConversation: 64 * 1024 * 1024,
};

export type IncomingFile = { name: string; mime: string; data: Buffer };

const CONV = /^[A-Za-z0-9_-]{1,128}$/;
const MIME = /^[a-z0-9.+-]+\/[a-z0-9.+-]+$/i;
const EXT: Record<string, string> = {
  'application/json': 'json',
  'text/plain': 'txt',
  'text/csv': 'csv',
  'text/markdown': 'md',
  'application/x-ndjson': 'ndjson',
  'application/pdf': 'pdf',
  'image/png': 'png',
  'image/jpeg': 'jpg',
  'image/gif': 'gif',
  'image/webp': 'webp',
};
const NAME_EXT = /\.([a-z0-9]{1,8})$/i;

// The session's Read tool picks how to open a file by its extension.
function extension(mime: string, name: string): string {
  return EXT[mime] ?? NAME_EXT.exec(name)?.[1].toLowerCase() ?? 'bin';
}

export function safeConv(conv: string): string {
  if (!CONV.test(conv)) throw new Error('bad conversation id');
  return conv;
}

// The adapter's name is display text only; it never becomes a path.
export function nameError(name: string): string | null {
  if (!name || name.length > 200) return 'file name length';
  if (/[/\\\u0000]/.test(name) || name === '.' || name === '..') return `file name ${JSON.stringify(name)} has a separator`;
  return null;
}

// Files land in <workRoot>/<conv>/data/<requestId>/ under generated names,
// next to a manifest that maps them back to the adapter's names.
export class FileStore {
  #root: string;
  #limits: FileLimits;
  #used = new Map<string, number>();
  #queue = new Map<string, Promise<unknown>>();
  // Forgotten conversations; a delivery still in flight for one must not recreate its directory.
  #closed = new Set<string>();

  constructor(workRoot: string, limits: FileLimits) {
    this.#root = path.resolve(workRoot);
    this.#limits = limits;
  }

  get limits(): FileLimits {
    return this.#limits;
  }

  dataDir(conv: string): string {
    return path.join(this.#root, safeConv(conv), 'data');
  }

  // Remaining byte budget for the conversation.
  async remaining(conv: string): Promise<number> {
    return this.#limits.bytesPerConversation - (await this.#usage(conv));
  }

  write(conv: string, requestId: string, meta: { source: string; summary: string }, files: IncomingFile[]): Promise<DeliveredFile[]> {
    return this.#serial(conv, () => {
      if (this.#closed.has(conv)) throw new Error('conversation closed');
      return this.#write(conv, requestId, meta, files);
    });
  }

  // A conversation in use again after forget accepts deliveries.
  reopen(conv: string): void {
    this.#closed.delete(conv);
  }

  async #write(
    conv: string,
    requestId: string,
    meta: { source: string; summary: string },
    files: IncomingFile[],
  ): Promise<DeliveredFile[]> {
    if (!/^[A-Za-z0-9_-]{8,64}$/.test(requestId)) throw new Error('bad request id');
    const total = files.reduce((n, f) => n + f.data.length, 0);
    if (files.length > this.#limits.filesPerDelivery) throw new Error('too many files');
    if (total > this.#limits.bytesPerDelivery) throw new Error('delivery too large');
    if (total > (await this.remaining(conv))) throw new Error('conversation data budget exhausted');

    const data = this.dataDir(conv);
    const final = path.join(data, requestId);
    const kept = await stored(final);
    if (kept) return kept;
    const tmp = path.join(data, `.tmp-${requestId}`);
    await rm(tmp, { recursive: true, force: true });
    await mkdir(tmp, { recursive: true });
    const out: DeliveredFile[] = [];
    const manifest = [];
    for (const [i, f] of files.entries()) {
      const mime = MIME.test(f.mime) ? f.mime.toLowerCase() : 'application/octet-stream';
      const file = `${i + 1}.${extension(mime, f.name)}`;
      // The runner container may run as another uid; the directory is this conversation's alone.
      await writeFile(path.join(tmp, file), f.data, { mode: 0o644 });
      out.push({ path: path.join(final, file), name: f.name, mime, size: f.data.length });
      manifest.push({ file, name: f.name, mime, size: f.data.length });
    }
    await writeFile(
      path.join(tmp, 'manifest.json'),
      JSON.stringify({ request: requestId, source: meta.source, summary: meta.summary, files: manifest }, null, 2),
    );
    await rename(tmp, final);
    this.#used.set(conv, (await this.#usage(conv)) + total);
    return out;
  }

  async forget(conv: string): Promise<void> {
    this.#closed.add(conv);
    await this.#serial(conv, () => rm(this.dataDir(conv), { recursive: true, force: true }));
    this.#used.delete(conv);
  }

  // Per conversation, so two deliveries cannot both spend the same budget and forget cannot interleave a write.
  #serial<T>(conv: string, fn: () => Promise<T>): Promise<T> {
    const next = (this.#queue.get(conv) ?? Promise.resolve()).then(fn, fn);
    const tail = next.then(() => {}, () => {});
    this.#queue.set(conv, tail);
    void tail.then(() => {
      if (this.#queue.get(conv) === tail) this.#queue.delete(conv);
    });
    return next;
  }

  async #usage(conv: string): Promise<number> {
    const known = this.#used.get(conv);
    if (known !== undefined) return known;
    const used = await du(this.dataDir(conv));
    this.#used.set(conv, used);
    return used;
  }
}

// A write already made under this id, replayed after a restart.
async function stored(dir: string): Promise<DeliveredFile[] | null> {
  let manifest: { files: { file: string; name: string; mime: string; size: number }[] };
  try {
    manifest = JSON.parse(await readFile(path.join(dir, 'manifest.json'), 'utf8'));
  } catch {
    return null;
  }
  return manifest.files.map((f) => ({ path: path.join(dir, f.file), name: f.name, mime: f.mime, size: f.size }));
}

async function du(dir: string): Promise<number> {
  let entries;
  try {
    entries = await readdir(dir, { withFileTypes: true, recursive: true });
  } catch {
    return 0;
  }
  let n = 0;
  for (const e of entries) {
    if (e.isFile() && e.name !== 'manifest.json') n += (await stat(path.join(e.parentPath, e.name))).size;
  }
  return n;
}
