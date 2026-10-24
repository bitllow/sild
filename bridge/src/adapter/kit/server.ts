import { createHash, randomBytes, type KeyObject } from 'node:crypto';
import type { IncomingMessage, ServerResponse } from 'node:http';
import { NonceStore, checkSecret, one, verifyRequest } from './hmac.ts';
import { REQUESTER_HEADER, SildKeys, type SildTrust } from './requester.ts';
import { signDelivery } from './jws.ts';
import { checkSchema, isRecord, validate } from './schema.ts';
import { LIMITS, REQUEST_ID, type Delivery, type InfoRequest, type Instructions, type Source, type WireFile } from './types.ts';

// What a provider hands back for one file. The kit hashes it and decides
// inline vs URL.
export type FileSpec = {
  name: string;
  mime: string;
  data: Buffer | string;
  byUrl?: boolean;
};

export type Final =
  | { type: 'deliver'; summary: string; files: FileSpec[]; approvedUntil?: number }
  | { type: 'decline'; reason: string; code?: string }
  | { type: 'expire' };

export type Outcome =
  // Answered in the POST response (201, or 403 for a decline).
  | Final
  // 202; the kit publishes the result, or `expired` once max_wait passes.
  | { type: 'async'; result: Promise<Final> }
  // 202; the provider calls adapter.publish itself, or never.
  | { type: 'accepted' };

export type Handler = (req: InfoRequest) => Outcome | Promise<Outcome>;

// Null, an empty text or no option at all answers 404.
export type InstructionsOption = string | (() => Instructions | null | Promise<Instructions | null>);

export type AdapterOptions = {
  secret: string;
  privateKey: KeyObject;
  catalog: Source[];
  handle: Handler;
  // Every request must carry Sild's assertion of who asks; see requester.ts.
  sild: SildTrust;
  instructions?: InstructionsOption;
  // Origin the bridge reaches this adapter at; needed for URL-delivered files.
  publicUrl?: string;
  basePath?: string;
  deliveryTtlSeconds?: number;
  fileTtlSeconds?: number;
  retainDeliveries?: number;
  maxBodyBytes?: number;
  now?: () => number;
};

export type Adapter = {
  handler: (req: IncomingMessage, res: ServerResponse) => void;
  publish: (requestId: string, final: Final) => void;
  close: () => void;
};

type Pending = { req: InfoRequest; timer?: NodeJS.Timeout };
type Logged = { seq: number; jws: string; expires: number };
type Waiter = () => void;

const PAGE_ITEMS = 50;
const PAGE_BYTES = 8 * 1024 * 1024;

export function createAdapter(opts: AdapterOptions): Adapter {
  checkSecret(opts.secret);
  for (const s of opts.catalog) {
    const errors = checkSchema(s.params);
    if (errors.length) throw new Error(`catalog ${s.source}: ${errors.join('; ')}`);
  }
  if (typeof opts.instructions === 'string' && opts.instructions) {
    const bad = instructionsError({ text: opts.instructions });
    if (bad) throw new Error(`instructions: ${bad}`);
  }
  const now = opts.now ?? Date.now;
  const sildKeys = new SildKeys(opts.sild);
  const base = (opts.basePath ?? '').replace(/\/$/, '');
  const nonces = new NonceStore();
  const sources = new Map(opts.catalog.map((s) => [s.source, s]));
  const pending = new Map<string, Pending>();
  const seenIds = new Set<string>();
  const log: Logged[] = [];
  const files = new Map<string, { data: Buffer; mime: string; expires: number }>();
  let waiters: Waiter[] = [];
  // Wall-clock seeded, so a restarted adapter's deliveries sort after every cursor it issued before.
  let seq = now();
  let closed = false;

  const wire = (spec: FileSpec): WireFile => {
    const data = typeof spec.data === 'string' ? Buffer.from(spec.data) : spec.data;
    const meta = { name: spec.name, mime: spec.mime, size: data.length, sha256: createHash('sha256').update(data).digest('hex') };
    if (!spec.byUrl && data.length <= LIMITS.inlineFileBytes) return { ...meta, inline: data.toString('base64') };
    if (!opts.publicUrl) throw new Error('publicUrl is required to deliver files by URL');
    const token = randomBytes(24).toString('base64url');
    files.set(token, { data, mime: spec.mime, expires: now() + (opts.fileTtlSeconds ?? 600) * 1000 });
    return { ...meta, url: `${opts.publicUrl.replace(/\/$/, '')}${base}/files/${token}` };
  };

  const seal = (req: InfoRequest, final: Final): string => {
    const delivery: Delivery =
      final.type === 'deliver'
        ? { id: req.id, state: 'delivered', summary: clip(final.summary), files: final.files.map(wire), ...(final.approvedUntil && { approved_until: Math.floor(final.approvedUntil) }) }
        : final.type === 'decline'
          ? { id: req.id, state: 'declined', summary: clip(final.reason), files: [], ...(final.code && { code: final.code }) }
          : { id: req.id, state: 'expired', summary: '', files: [] };
    const iat = Math.floor(now() / 1000);
    return signDelivery(opts.privateKey, {
      request_id: req.id,
      conversation_ref: req.conversation_ref,
      iat,
      exp: iat + (opts.deliveryTtlSeconds ?? 300),
      delivery,
    });
  };

  const publish = (requestId: string, final: Final) => {
    const p = pending.get(requestId);
    if (!p || closed) return;
    clearTimeout(p.timer);
    pending.delete(requestId);
    seq = Math.max(seq + 1, now());
    log.push({ seq, jws: seal(p.req, final), expires: now() + (opts.deliveryTtlSeconds ?? 300) * 1000 });
    // Past its exp a delivery is refused by the bridge anyway.
    while (log.length && (log[0]!.expires <= now() || log.length > (opts.retainDeliveries ?? 1000))) log.shift();
    const ws = waiters;
    waiters = [];
    for (const w of ws) w();
  };

  const after = (cursor: number) => {
    const page: Logged[] = [];
    let bytes = 0;
    for (const l of log) {
      if (l.seq <= cursor || l.expires <= now()) continue;
      if (page.length && (page.length >= PAGE_ITEMS || bytes + l.jws.length > PAGE_BYTES)) break;
      page.push(l);
      bytes += l.jws.length;
    }
    return page;
  };

  const routes = async (req: IncomingMessage, res: ServerResponse, url: URL, body: Buffer) => {
    if (url.pathname !== base && !url.pathname.startsWith(`${base}/`)) return json(res, 404, { error: 'not found' });
    const path = url.pathname.slice(base.length);
    if (req.method === 'GET' && path === '/catalog') return json(res, 200, opts.catalog);

    if (req.method === 'GET' && path === '/instructions') {
      const i = typeof opts.instructions === 'function' ? await opts.instructions() : opts.instructions ? { text: opts.instructions } : null;
      if (!i?.text) return json(res, 404, { error: 'no instructions' });
      const bad = instructionsError(i);
      if (bad) return json(res, 500, { error: `instructions: ${bad}` });
      return json(res, 200, i.version === undefined ? { text: i.text } : { text: i.text, version: i.version });
    }

    if (req.method === 'POST' && path === '/requests') {
      let parsed: unknown;
      try {
        parsed = JSON.parse(body.toString('utf8'));
      } catch {
        return json(res, 400, { error: 'body is not JSON' });
      }
      const bad = requestErrors(parsed);
      if (bad) return json(res, 400, { error: bad });
      const ir = parsed as InfoRequest;
      const unvouched = await sildKeys.check(one(req.headers[REQUESTER_HEADER]), body, ir, now());
      if (unvouched) return json(res, 401, { error: unvouched });
      const source = sources.get(ir.source);
      if (!source) return json(res, 404, { error: `unknown source ${ir.source}` });
      const errors = validate(source.params, ir.params);
      if (errors.length) return json(res, 400, { error: errors.join('; ') });
      if (seenIds.has(ir.id)) return json(res, 409, { error: 'duplicate request id' });
      seenIds.add(ir.id);

      const outcome = await opts.handle(ir);
      if (outcome.type === 'decline') {
        return json(res, 403, { declined: true, reason: clip(outcome.reason), ...(outcome.code && { code: outcome.code }) });
      }
      if (outcome.type === 'deliver' || outcome.type === 'expire') return json(res, 201, { delivery: seal(ir, outcome) });

      const p: Pending = { req: ir };
      pending.set(ir.id, p);
      if (outcome.type === 'async') {
        p.timer = setTimeout(() => publish(ir.id, { type: 'expire' }), source.max_wait_seconds * 1000);
        p.timer.unref();
        outcome.result.then(
          (final) => publish(ir.id, final),
          () => publish(ir.id, { type: 'expire' }),
        );
      }
      return json(res, 202, { id: ir.id });
    }

    if (req.method === 'GET' && path === '/deliveries') {
      const raw = url.searchParams.get('cursor') ?? '';
      if (raw !== '' && !/^\d{1,15}$/.test(raw)) return json(res, 400, { error: 'bad cursor' });
      // A cursor past the head was issued before a restart under a clock that has since gone back.
      const cursor = raw === '' || Number(raw) > seq ? 0 : Number(raw);
      const wait = Math.min(Math.max(Number(url.searchParams.get('wait') ?? 0) || 0, 0), LIMITS.maxWaitSeconds);
      let items = after(cursor);
      if (!items.length && wait > 0) {
        await new Promise<void>((resolve) => {
          const t = setTimeout(done, wait * 1000);
          function done() {
            clearTimeout(t);
            waiters = waiters.filter((w) => w !== done);
            resolve();
          }
          waiters.push(done);
          res.once('close', done);
        });
        items = after(cursor);
      }
      const next = items.length ? items[items.length - 1]!.seq : Math.max(cursor, 0);
      return json(res, 200, { items: items.map((i) => i.jws), next_cursor: String(next) });
    }

    const file = req.method === 'GET' && path.startsWith('/files/') ? files.get(path.slice('/files/'.length)) : undefined;
    if (file && file.expires > now()) {
      res.writeHead(200, { 'content-type': file.mime, 'content-length': file.data.length });
      return void res.end(file.data);
    }
    return json(res, 404, { error: 'not found' });
  };

  const handler = (req: IncomingMessage, res: ServerResponse) => {
    const max = opts.maxBodyBytes ?? 64 * 1024;
    const chunks: Buffer[] = [];
    let size = 0;
    let over = false;
    req.on('data', (c: Buffer) => {
      size += c.length;
      if (size > max) over = true;
      else chunks.push(c);
    });
    req.on('end', () => {
      if (over) return json(res, 413, { error: 'body too large' });
      const body = Buffer.concat(chunks);
      const target = req.url ?? '/';
      const verdict = verifyRequest(opts.secret, nonces, { method: req.method ?? 'GET', target, body }, req.headers, now());
      if (!verdict.ok) return json(res, 401, { error: verdict.reason });
      routes(req, res, new URL(target, 'http://adapter'), body).catch((err: unknown) => {
        if (!res.headersSent) json(res, 500, { error: err instanceof Error ? err.message : 'internal error' });
      });
    });
  };

  const close = () => {
    closed = true;
    for (const p of pending.values()) clearTimeout(p.timer);
    pending.clear();
    for (const w of waiters) w();
    waiters = [];
  };

  return { handler, publish, close };
}

function requestErrors(v: unknown): string | null {
  if (!isRecord(v)) return 'body must be an object';
  if (typeof v.id !== 'string' || !REQUEST_ID.test(v.id)) return 'bad id';
  if (!isRecord(v.requester) || typeof v.requester.id !== 'string' || !v.requester.id) return 'bad requester';
  if (typeof v.conversation_ref !== 'string' || !v.conversation_ref) return 'bad conversation_ref';
  if (typeof v.source !== 'string') return 'bad source';
  if (!isRecord(v.params)) return 'params must be an object';
  if (typeof v.reason !== 'string') return 'bad reason';
  return null;
}

// Null when v is a GET /instructions body the bridge accepts.
export function instructionsError(v: unknown): string | null {
  if (!isRecord(v)) return 'not an object';
  if (typeof v.text !== 'string' || !v.text.trim()) return 'text is missing or empty';
  if (Buffer.byteLength(v.text) > LIMITS.instructionsBytes) return `text over ${LIMITS.instructionsBytes} bytes`;
  if (v.version !== undefined && typeof v.version !== 'string') return 'version is not a string';
  return null;
}

// Summaries are one line and at most LIMITS.summaryBytes of UTF-8.
export function clip(s: string): string {
  return truncateUtf8(s.replace(/[\u0000-\u001f\u007f]+/g, ' ').trim(), LIMITS.summaryBytes);
}

// Cuts on a character boundary so the result, suffix included, fits in maxBytes.
export function truncateUtf8(s: string, maxBytes: number, suffix = ''): string {
  const b = Buffer.from(s);
  if (b.length <= maxBytes) return s;
  let end = Math.max(maxBytes - Buffer.byteLength(suffix), 0);
  while (end > 0 && (b[end] & 0xc0) === 0x80) end--;
  return b.subarray(0, end).toString('utf8') + suffix;
}

function json(res: ServerResponse, status: number, body: unknown) {
  const data = JSON.stringify(body);
  res.writeHead(status, { 'content-type': 'application/json', 'content-length': Buffer.byteLength(data) });
  res.end(data);
}
