import { randomUUID } from 'node:crypto';
import { rm } from 'node:fs/promises';
import path from 'node:path';
import type { BrokerEvent, InfoBroker, Source } from '../contracts.ts';
import type { AdapterClient, Received } from '../adapter/client.ts';
import { safeConv } from '../adapter/files.ts';
import { isRecord, validate } from '../adapter/kit/schema.ts';
import { redact } from '../router/redact.ts';
import type { Proposal, Router } from '../router/router.ts';
import { errText } from '../util.ts';

export type BrokerLimits = {
  perMessage: number;
  perConversation: number;
  // Different ids (drivers, vehicles, trips, names) one conversation may ask about.
  distinctIds: number;
  // Ceiling on a source's own max_wait_seconds.
  maxWaitSeconds: number;
  // Extra time for the adapter's own `expired` to arrive before we give up.
  graceSeconds: number;
};

export const DEFAULT_BROKER_LIMITS: BrokerLimits = { perMessage: 3, perConversation: 12, distinctIds: 4, maxWaitSeconds: 300, graceSeconds: 5 };

export type BrokerConfig = {
  adapter: AdapterClient;
  router: Router;
  limits?: Partial<BrokerLimits>;
  log?: (msg: string) => void;
};

export type Broker = InfoBroker & { close(): void };

type Inflight = { source: string; key: string; timer: NodeJS.Timeout };
type Conv = {
  issued: number;
  ids: Set<string>;
  inflight: Map<string, Inflight>;
  // key (source + params) → request id, for everything delivered so far.
  delivered: Map<string, { source: string; requestId: string }>;
};

export function createBroker(cfg: BrokerConfig): Broker {
  const limits = { ...DEFAULT_BROKER_LIMITS, ...cfg.limits };
  const log = cfg.log ?? (() => {});
  const convs = new Map<string, Conv>();
  const listeners = new Set<(e: BrokerEvent) => void>();

  const state = (conv: string): Conv => {
    let c = convs.get(conv);
    if (!c) {
      convs.set(conv, (c = { issued: 0, ids: new Set(), inflight: new Map(), delivered: new Map() }));
      cfg.adapter.files.reopen(conv);
    }
    return c;
  };

  const emit = (e: BrokerEvent) => {
    for (const cb of listeners) {
      try {
        cb(e);
      } catch (err) {
        log(`broker listener failed: ${errText(err)}`);
      }
    }
  };

  const catalog = async (): Promise<Source[]> => {
    try {
      return await cfg.adapter.catalog();
    } catch (err) {
      log(`catalog unavailable: ${errText(err)}`);
      return [];
    }
  };

  const finish = (conv: string, requestId: string): Inflight | null => {
    const f = convs.get(conv)?.inflight.get(requestId);
    if (!f) return null;
    clearTimeout(f.timer);
    convs.get(conv)!.inflight.delete(requestId);
    return f;
  };

  const notAvailable = (conv: string, requestId: string, reason: string) => {
    if (finish(conv, requestId)) emit({ type: 'not_available', conv, requestId, reason });
  };

  const received = async (r: Received) => {
    const f = finish(r.conv, r.requestId);
    if (!f) {
      // Forgotten or timed out while the files were being written.
      if (r.state === 'delivered' && r.files.length) {
        await rm(path.join(cfg.adapter.files.dataDir(r.conv), r.requestId), { recursive: true, force: true });
      }
      return;
    }
    if (r.state === 'delivered') {
      convs.get(r.conv)?.delivered.set(f.key, { source: f.source, requestId: r.requestId });
      emit({ type: 'delivered', conv: r.conv, requestId: r.requestId, summary: r.summary, files: r.files });
      return;
    }
    const reason = r.state === 'declined' ? `declined: ${r.reason}` : r.state === 'expired' ? 'expired: the provider gave up' : `invalid delivery: ${r.reason}`;
    emit({ type: 'not_available', conv: r.conv, requestId: r.requestId, reason });
  };

  const unsubscribe = cfg.adapter.onReceived((r) => void received(r));

  const start = (conv: string, requester: string, source: Source, p: Proposal, reason: string): string => {
    const c = state(conv);
    const requestId = randomUUID().replaceAll('-', '');
    const wait = Math.min(source.max_wait_seconds, limits.maxWaitSeconds) + limits.graceSeconds;
    const timer = setTimeout(() => {
      cfg.adapter.cancel(requestId);
      notAvailable(conv, requestId, `no answer within ${Math.round(wait)}s`);
    }, wait * 1000);
    timer.unref();
    c.issued++;
    for (const id of idsOf(source, p.params)) c.ids.add(id);
    c.inflight.set(requestId, { source: p.source, key: keyOf(p), timer });
    emit({ type: 'requested', conv, requestId, label: redact(p.label).text, source: p.source, params: p.params });
    cfg.adapter
      .request(conv, { id: requestId, requester: { id: requester }, source: p.source, params: p.params, reason })
      .then(
        (sent) => {
          if (sent.type === 'failed') notAvailable(conv, requestId, sent.reason);
        },
        (err: unknown) => notAvailable(conv, requestId, errText(err)),
      );
    return requestId;
  };

  // Null when the proposal is usable, otherwise why not.
  const reject = (c: Conv, sources: Map<string, Source>, p: Proposal): string | null => {
    const s = sources.get(p.source);
    if (!s) return `no source ${p.source}`;
    const errors = validate(s.params, p.params);
    if (errors.length) return `params do not fit ${p.source}: ${errors.join('; ')}`;
    const ids = new Set([...c.ids, ...idsOf(s, p.params)]);
    return ids.size > limits.distinctIds ? `this conversation may ask about at most ${limits.distinctIds} different ids` : null;
  };

  return {
    async onRequesterText(conv, requester, text) {
      safeConv(conv);
      const c = state(conv);
      const budget = Math.min(limits.perMessage, limits.perConversation - c.issued);
      const sources = new Map((await catalog()).map((s) => [s.source, s]));
      const taken = new Set([...[...c.delivered.values()].map((d) => d.source), ...[...c.inflight.values()].map((f) => f.source)]);
      let proposals: Proposal[] = [];
      // A requester the provider cannot identify gets no data: fail closed.
      if (requester) {
        try {
          proposals = (await cfg.router.route({ text, catalog: [...sources.values()], delivered: [...taken], max: Math.max(budget, 0) })).requests;
        } catch (err) {
          log(`router failed: ${errText(err)}`);
        }
      }
      let issued = 0;
      for (const p of proposals) {
        if (issued >= budget || !convs.has(conv)) break;
        if (taken.has(p.source)) continue;
        const why = reject(c, sources, p);
        if (why) {
          log(`router proposal dropped: ${why}`);
          continue;
        }
        taken.add(p.source);
        start(conv, requester, sources.get(p.source)!, p, 'requester message');
        issued++;
      }
      return { text: redact(text).text };
    },

    async request(conv, requester, need, why) {
      safeConv(conv);
      if (!requester) return { notAvailable: 'the requester is not linked to the data provider' };
      const c = state(conv);
      if (c.issued >= limits.perConversation) return { notAvailable: 'request limit for this conversation reached' };
      const sources = new Map((await catalog()).map((s) => [s.source, s]));
      if (!sources.size) return { notAvailable: 'no data sources are reachable' };
      let p: Proposal | undefined;
      try {
        p = (await cfg.router.route({ text: need, catalog: [...sources.values()], delivered: [], max: 1 })).requests[0];
      } catch (err) {
        log(`router failed: ${errText(err)}`);
        return { notAvailable: 'could not match the need to a data source' };
      }
      if (!p) return { notAvailable: 'no data source offers this' };
      const problem = reject(c, sources, p);
      if (problem) return { notAvailable: problem };
      const key = keyOf(p);
      const done = c.delivered.get(key);
      if (done) return { notAvailable: `already delivered in data/${done.requestId}/` };
      for (const [requestId, f] of c.inflight) if (f.key === key) return { requestId };
      if (!convs.has(conv)) return { notAvailable: 'conversation closed' };
      return { requestId: start(conv, requester, sources.get(p.source)!, p, why) };
    },

    subscribe(cb) {
      listeners.add(cb);
      return () => listeners.delete(cb);
    },

    async forget(conv) {
      const c = convs.get(conv);
      convs.delete(conv);
      for (const [requestId, f] of c?.inflight ?? []) {
        clearTimeout(f.timer);
        cfg.adapter.cancel(requestId);
      }
      await cfg.adapter.files.forget(conv);
    },

    instructions: () => cfg.adapter.instructions(),

    catalog: async () => ({ sources: await catalog(), perConversation: limits.perConversation, distinctIds: limits.distinctIds }),

    close() {
      unsubscribe();
      for (const c of convs.values()) for (const f of c.inflight.values()) clearTimeout(f.timer);
      convs.clear();
      cfg.adapter.close();
    },
  };
}

// The values of a request's free-text params, which name what it is about. Enums and numbers do not.
function idsOf(source: Source, params: Record<string, unknown>): string[] {
  const props = isRecord(source.params.properties) ? source.params.properties : {};
  return Object.entries(params).flatMap(([name, v]) => {
    const schema = props[name];
    const free = isRecord(schema) && schema.type === 'string' && !Array.isArray(schema.enum);
    return free && typeof v === 'string' && v.trim() ? [v.trim().toLowerCase()] : [];
  });
}

function keyOf(p: { source: string; params: Record<string, unknown> }): string {
  return `${p.source} ${stable(p.params)}`;
}

function stable(v: unknown): string {
  if (Array.isArray(v)) return `[${v.map(stable).join(',')}]`;
  if (typeof v === 'object' && v !== null) {
    return `{${Object.keys(v).sort().map((k) => `${JSON.stringify(k)}:${stable((v as Record<string, unknown>)[k])}`).join(',')}}`;
  }
  return JSON.stringify(v);
}
