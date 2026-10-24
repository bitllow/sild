import { createHash, type KeyObject } from 'node:crypto';
import type { DeliveredFile, IntegratorInstructions, Source } from '../contracts.ts';
import { nameError, safeConv, type FileStore, type IncomingFile } from './files.ts';
import { checkSchema, isRecord } from './kit/schema.ts';
import { signRequest } from './kit/hmac.ts';
import { REQUESTER_HEADER } from './kit/requester.ts';
import { loadPublicKey, openDelivery } from './kit/jws.ts';
import { clip, instructionsError } from './kit/server.ts';
import type { JsonStore } from '../state.ts';
import { errText } from '../util.ts';
import { DECLINE_CODE, LIMITS, SOURCE_NAME, type Delivery, type InfoRequest, type WireFile } from './kit/types.ts';
import { resolveTarget, send, systemResolver, type Resolver, type Response } from './net.ts';

// Sild's assertion of who asks this exact request body (kit/requester.ts).
export type Assert = (conv: string, body: string) => Promise<string>;

export type AdapterClientConfig = {
  url: string; // the adapter's registered base URL
  secret: string;
  publicKey: string; // pinned Ed25519 key, base64url raw or PEM
  // Shared with the slot manager, which stores requester attachments in the same budget.
  files: FileStore;
  assert: Assert;
  // Keeps outstanding requests and the deliveries cursor across a restart.
  state?: JsonStore;
  // Permits http:// to localhost / 127.0.0.1 / ::1. Local development only.
  allowInsecureLocal?: boolean;
  catalogTtlMs?: number;
  instructionsTtlMs?: number;
  timeoutMs?: number;
  resolve?: Resolver;
  log?: (msg: string) => void;
};

export type Received =
  | { requestId: string; conv: string; source: string; state: 'delivered'; summary: string; files: DeliveredFile[]; approvedUntil?: number }
  | { requestId: string; conv: string; source: string; state: 'declined' | 'expired' | 'invalid'; reason: string; code?: string };

// Unless the request failed, its outcome arrives through onReceived, even one answered in the response.
export type Sent = { type: 'pending' } | { type: 'failed'; reason: string };

type Outstanding = { conv: string; source: string };

const POLL_WAIT_SECONDS = 30;
// Retried sooner than the TTL after a failure: a session opened without them never gets them.
const INSTRUCTIONS_RETRY_MS = 30_000;
const PAGE_BYTES = 64 * 1024 * 1024;
const STATE_KEY = 'adapter';

type SavedState = { cursor: string; outstanding: Record<string, Outstanding> };

export class AdapterClient {
  #cfg: AdapterClientConfig;
  #base: URL;
  #key: KeyObject;
  #files: FileStore;
  #resolve: Resolver;
  #log: (msg: string) => void;
  #catalog: { at: number; sources: Source[] } | null = null;
  #instructions: { until: number; value: IntegratorInstructions | null } | null = null;
  #instructionsFetch: Promise<IntegratorInstructions | null> | null = null;
  #outstanding = new Map<string, Outstanding>();
  #listeners = new Set<(r: Received) => void>();
  #cursor = '';
  #polling: AbortController | null = null;

  constructor(cfg: AdapterClientConfig) {
    this.#cfg = cfg;
    this.#base = new URL(cfg.url.endsWith('/') ? cfg.url : `${cfg.url}/`);
    this.#key = loadPublicKey(cfg.publicKey);
    this.#files = cfg.files;
    this.#resolve = cfg.resolve ?? systemResolver;
    this.#log = cfg.log ?? (() => {});
    const saved = cfg.state?.get<SavedState>(STATE_KEY);
    if (saved) {
      this.#cursor = saved.cursor;
      this.#outstanding = new Map(Object.entries(saved.outstanding));
      if (this.#outstanding.size) this.#poll();
    }
  }

  get files(): FileStore {
    return this.#files;
  }

  onReceived(cb: (r: Received) => void): () => void {
    this.#listeners.add(cb);
    return () => this.#listeners.delete(cb);
  }

  // Entries whose name or params schema falls outside the contract are dropped.
  async catalog(): Promise<Source[]> {
    const ttl = this.#cfg.catalogTtlMs ?? 60_000;
    if (this.#catalog && Date.now() - this.#catalog.at < ttl) return this.#catalog.sources;
    const res = await this.#call('GET', 'catalog', '', 1024 * 1024);
    if (res.status !== 200) throw new Error(`catalog: HTTP ${res.status}`);
    const raw: unknown = JSON.parse(res.body.toString('utf8'));
    if (!Array.isArray(raw)) throw new Error('catalog: not a list');
    const sources: Source[] = [];
    for (const s of raw) {
      const problem = catalogError(s);
      if (problem) this.#log(`catalog entry dropped: ${problem}`);
      else sources.push(s as Source);
    }
    this.#catalog = { at: Date.now(), sources };
    return sources;
  }

  // 404 or a body outside the contract is none; a failed fetch keeps the last good value.
  async instructions(): Promise<IntegratorInstructions | null> {
    if (this.#instructions && Date.now() < this.#instructions.until) return this.#instructions.value;
    this.#instructionsFetch ??= this.#fetchInstructions().finally(() => (this.#instructionsFetch = null));
    return this.#instructionsFetch;
  }

  async #fetchInstructions(): Promise<IntegratorInstructions | null> {
    const ttl = this.#cfg.instructionsTtlMs ?? 5 * 60_000;
    let value: IntegratorInstructions | null = null;
    try {
      const res = await this.#call('GET', 'instructions', '', 64 * 1024);
      if (res.status === 200) {
        const body = parseJson(res.body);
        const bad = instructionsError(body);
        if (bad) this.#log(`instructions rejected: ${bad}`);
        else {
          const { text, version } = body as { text: string; version?: string };
          value = { text, ...(version !== undefined && { version }), from: this.#base.host };
        }
      } else if (res.status !== 404) {
        throw new Error(`HTTP ${res.status}`);
      }
    } catch (err) {
      this.#log(`instructions unavailable: ${errText(err)}`);
      const last = this.#instructions?.value ?? null;
      this.#instructions = { until: Date.now() + Math.min(ttl, INSTRUCTIONS_RETRY_MS), value: last };
      return last;
    }
    this.#instructions = { until: Date.now() + ttl, value };
    return value;
  }

  async request(conv: string, req: Omit<InfoRequest, 'conversation_ref'>): Promise<Sent> {
    safeConv(conv);
    const body = JSON.stringify({ ...req, conversation_ref: conv } satisfies InfoRequest);
    this.#track(req.id, { conv, source: req.source });
    let assertion: string;
    try {
      assertion = await this.#cfg.assert(conv, body);
    } catch (err) {
      this.#untrack(req.id);
      return { type: 'failed', reason: `Sild would not vouch for the requester: ${errText(err)}` };
    }
    let res: Response;
    try {
      res = await this.#call('POST', 'requests', body, PAGE_BYTES, { headers: { [REQUESTER_HEADER]: assertion } });
    } catch (err) {
      this.#untrack(req.id);
      return { type: 'failed', reason: `adapter unreachable: ${errText(err)}` };
    }
    const parsed = parseJson(res.body);
    if (res.status === 201 && isRecord(parsed) && typeof parsed.delivery === 'string') {
      const result = await this.#accept(parsed.delivery);
      if (result) this.#emit(result);
      if (result?.requestId === req.id) return { type: 'pending' };
      this.#untrack(req.id);
      return { type: 'failed', reason: 'adapter answered with a delivery for another request' };
    }
    if (res.status === 202) {
      this.#poll();
      return { type: 'pending' };
    }
    this.#untrack(req.id);
    if (res.status === 403) {
      const reason = isRecord(parsed) && typeof parsed.reason === 'string' ? clip(parsed.reason) || 'declined' : 'declined';
      this.#emit({ requestId: req.id, conv, source: req.source, state: 'declined', reason, ...declineCode(parsed) });
      return { type: 'pending' };
    }
    const error = isRecord(parsed) && typeof parsed.error === 'string' ? `: ${clip(parsed.error)}` : '';
    return { type: 'failed', reason: `adapter refused the request (HTTP ${res.status}${error})` };
  }

  // A delivery that arrives after this is rejected.
  cancel(requestId: string): void {
    this.#untrack(requestId);
  }

  // Stops polling; what is outstanding stays saved for the next process.
  close(): void {
    this.#outstanding.clear();
    this.#polling?.abort();
    this.#polling = null;
  }

  #track(id: string, o: Outstanding): void {
    this.#outstanding.set(id, o);
    this.#save();
  }

  #untrack(id: string): void {
    if (this.#outstanding.delete(id)) this.#save();
  }

  #save(): void {
    this.#cfg.state?.set(STATE_KEY, { cursor: this.#cursor, outstanding: Object.fromEntries(this.#outstanding) } satisfies SavedState);
  }

  // Long-polls only while something is outstanding.
  #poll(): void {
    if (this.#polling) return;
    const ctl = new AbortController();
    this.#polling = ctl;
    void (async () => {
      let backoff = 1000;
      while (!ctl.signal.aborted && this.#outstanding.size > 0) {
        try {
          const q = new URLSearchParams({ cursor: this.#cursor, wait: String(POLL_WAIT_SECONDS) });
          const res = await this.#call('GET', `deliveries?${q}`, '', PAGE_BYTES, { signal: ctl.signal, timeoutMs: (POLL_WAIT_SECONDS + 15) * 1000 });
          if (res.status !== 200) throw new Error(`deliveries: HTTP ${res.status}`);
          const page = parseJson(res.body);
          if (!isRecord(page) || !Array.isArray(page.items) || typeof page.next_cursor !== 'string') throw new Error('deliveries: bad page');
          for (const item of page.items) {
            if (typeof item !== 'string') continue;
            const result = await this.#accept(item);
            if (result) this.#emit(result);
          }
          this.#cursor = page.next_cursor;
          this.#save();
          backoff = 1000;
        } catch (err) {
          if (ctl.signal.aborted) break;
          this.#log(`deliveries poll failed: ${errText(err)}`);
          await sleep(backoff, ctl.signal);
          backoff = Math.min(backoff * 2, 30_000);
        }
      }
      if (this.#polling === ctl) this.#polling = null;
      // A request may have gone outstanding just as the loop decided to stop.
      if (!ctl.signal.aborted && this.#outstanding.size > 0) this.#poll();
    })();
  }

  #emit(r: Received): void {
    for (const cb of this.#listeners) cb(r);
  }

  // Verifies a delivery and stores its files. Null when it is not ours to take.
  async #accept(jws: string): Promise<Received | null> {
    const opened = openDelivery(this.#key, jws);
    if (!opened.ok) {
      this.#log(`delivery rejected: ${opened.reason}`);
      return null;
    }
    const { request_id: id, conversation_ref: conv, delivery } = opened.claims;
    const o = this.#outstanding.get(id);
    if (!o || o.conv !== conv) {
      this.#log(`delivery ${id} rejected: no outstanding request for that conversation`);
      return null;
    }
    this.#untrack(id);
    const base = { requestId: id, conv, source: o.source };
    if (delivery.state !== 'delivered') {
      return {
        ...base, state: delivery.state, ...declineCode(delivery),
        reason: delivery.state === 'expired' ? 'the provider gave up' : clip(delivery.summary) || 'declined',
      };
    }
    try {
      const files = await this.#store(conv, id, o.source, delivery);
      const until = delivery.approved_until;
      return { ...base, state: 'delivered', summary: delivery.summary, files, ...(Number.isInteger(until) && { approvedUntil: until }) };
    } catch (err) {
      return { ...base, state: 'invalid', reason: clip(errText(err)) };
    }
  }

  async #store(conv: string, id: string, source: string, d: Delivery): Promise<DeliveredFile[]> {
    if (Buffer.byteLength(d.summary) > LIMITS.summaryBytes || /[\u0000-\u001f\u007f]/.test(d.summary)) {
      throw new Error('summary is not a single line within 1 KiB');
    }
    const limits = this.#files.limits;
    if (d.files.length > limits.filesPerDelivery) throw new Error('too many files');
    const declared = d.files.reduce((n, f) => n + f.size, 0);
    if (declared > limits.bytesPerDelivery) throw new Error('delivery too large');
    if (declared > (await this.#files.remaining(conv))) throw new Error('conversation data budget exhausted');

    const incoming: IncomingFile[] = [];
    for (const f of d.files) {
      const bad = nameError(f.name);
      if (bad) throw new Error(bad);
      const data = f.inline !== undefined ? inline(f) : await this.#fetchFile(f);
      if (data.length !== f.size) throw new Error(`${f.name}: size mismatch`);
      if (createHash('sha256').update(data).digest('hex') !== f.sha256) throw new Error(`${f.name}: sha256 mismatch`);
      incoming.push({ name: f.name, mime: f.mime, data });
    }
    return this.#files.write(conv, id, { source, summary: d.summary }, incoming);
  }

  async #fetchFile(f: WireFile): Promise<Buffer> {
    let url: URL;
    try {
      url = new URL(f.url!);
    } catch {
      throw new Error(`${f.name}: bad url`);
    }
    if (url.host !== this.#base.host || url.protocol !== this.#base.protocol) throw new Error(`${f.name}: url is not on the adapter's host`);
    const target = await resolveTarget(url, this.#cfg.allowInsecureLocal ?? false, this.#resolve);
    const path = url.pathname + url.search;
    const res = await send(target, {
      method: 'GET',
      headers: signRequest(this.#cfg.secret, { method: 'GET', target: path, body: '' }),
      timeoutMs: this.#cfg.timeoutMs ?? 10_000,
      maxBytes: f.size,
    });
    if (res.status !== 200) throw new Error(`${f.name}: HTTP ${res.status}`);
    return res.body;
  }

  async #call(
    method: string, rel: string, body: string, maxBytes: number,
    opts: { signal?: AbortSignal; timeoutMs?: number; headers?: Record<string, string> } = {},
  ): Promise<Response> {
    const url = new URL(rel, this.#base);
    const target = await resolveTarget(url, this.#cfg.allowInsecureLocal ?? false, this.#resolve);
    const headers: Record<string, string> = { ...opts.headers, ...signRequest(this.#cfg.secret, { method, target: url.pathname + url.search, body }) };
    if (body) headers['content-type'] = 'application/json';
    return send(target, { method, headers, body, maxBytes, signal: opts.signal, timeoutMs: opts.timeoutMs ?? this.#cfg.timeoutMs ?? 10_000 });
  }
}

function declineCode(v: unknown): { code?: string } {
  return isRecord(v) && typeof v.code === 'string' && DECLINE_CODE.test(v.code) ? { code: v.code } : {};
}

function inline(f: WireFile): Buffer {
  if (f.size > LIMITS.inlineFileBytes) throw new Error(`${f.name}: inline file over 1 MiB`);
  return Buffer.from(f.inline!, 'base64');
}

function catalogError(s: unknown): string | null {
  if (!isRecord(s)) return 'not an object';
  if (typeof s.source !== 'string' || !SOURCE_NAME.test(s.source)) return `bad source name ${JSON.stringify(s.source)}`;
  if (typeof s.description !== 'string') return `${s.source}: no description`;
  if (typeof s.max_wait_seconds !== 'number' || !(s.max_wait_seconds > 0)) return `${s.source}: bad max_wait_seconds`;
  const errors = checkSchema(s.params);
  if (errors.length) return `${s.source}: ${errors.join('; ')}`;
  if (isRecord(s.params) && s.params.type !== 'object') return `${s.source}: params must be an object schema`;
  return null;
}

function parseJson(b: Buffer): unknown {
  try {
    return JSON.parse(b.toString('utf8'));
  } catch {
    return null;
  }
}

function sleep(ms: number, signal: AbortSignal): Promise<void> {
  return new Promise((resolve) => {
    const t = setTimeout(resolve, ms);
    signal.addEventListener('abort', () => {
      clearTimeout(t);
      resolve();
    }, { once: true });
  });
}
