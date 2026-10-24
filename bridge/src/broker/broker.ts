import { randomUUID } from 'node:crypto';
import { rm } from 'node:fs/promises';
import path from 'node:path';
import type { BrokerEvent, InfoBroker, Source } from '../contracts.ts';
import type { AdapterClient, Received } from '../adapter/client.ts';
import { safeConv } from '../adapter/files.ts';
import { isRecord, validate } from '../adapter/kit/schema.ts';
import { STEP_UP_REQUIRED } from '../adapter/kit/types.ts';
import type { JsonStore } from '../state.ts';
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
  // Keeps each conversation's requests across a restart.
  state?: JsonStore;
};

export type Broker = InfoBroker & { close(): void };

// One request as the session knows it: its id survives a retry, while the
// adapter sees a fresh wire id on every attempt.
type Inflight = {
  waitSeconds: number;
  requester: string;
  proposal: Proposal;
  reason: string;
  key: string;
  wire: string;
  // When the attempt under way gives up (ms); unset while parked until the requester approves.
  deadline?: number;
  timer?: NodeJS.Timeout;
};
type SavedConv = {
  issued: number;
  ids: string[];
  inflight: [string, Omit<Inflight, 'timer'>][];
  delivered: [string, { source: string; dir: string }][];
};
type Conv = {
  issued: number;
  ids: Set<string>;
  inflight: Map<string, Inflight>;
  // wire id → request id, for the attempt under way.
  wires: Map<string, string>;
  // key (source + params) → the data directory of what was delivered.
  delivered: Map<string, { source: string; dir: string }>;
};

export function createBroker(cfg: BrokerConfig): Broker {
  const limits = { ...DEFAULT_BROKER_LIMITS, ...cfg.limits };
  const log = cfg.log ?? (() => {});
  const convs = new Map<string, Conv>();
  const listeners = new Set<(e: BrokerEvent) => void>();

  const state = (conv: string): Conv => {
    let c = convs.get(conv);
    if (!c) {
      convs.set(conv, (c = { issued: 0, ids: new Set(), inflight: new Map(), wires: new Map(), delivered: new Map() }));
      cfg.adapter.files.reopen(conv);
    }
    return c;
  };

  const save = (conv: string, c: Conv) => {
    if (!convs.has(conv)) return;
    cfg.state?.set(conv, {
      issued: c.issued,
      ids: [...c.ids],
      inflight: [...c.inflight].map(([id, { timer: _t, ...f }]) => [id, f]),
      delivered: [...c.delivered],
    } satisfies SavedConv);
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

  const sourcesByName = async (): Promise<Map<string, Source>> => {
    try {
      return new Map((await cfg.adapter.catalog()).map((s) => [s.source, s]));
    } catch (err) {
      log(`catalog unavailable: ${errText(err)}`);
      return new Map();
    }
  };

  // Ends the attempt under way; the request stays known unless done.
  const settle = (c: Conv, requestId: string, done: boolean): Inflight | null => {
    const f = c.inflight.get(requestId);
    if (!f) return null;
    clearTimeout(f.timer);
    f.timer = undefined;
    c.wires.delete(f.wire);
    f.deadline = undefined;
    if (done) c.inflight.delete(requestId);
    return f;
  };

  const notAvailable = (conv: string, requestId: string, reason: string) => {
    const c = convs.get(conv);
    if (!c || !settle(c, requestId, true)) return;
    save(conv, c);
    emit({ type: 'not_available', conv, requestId, reason });
  };

  const received = async (r: Received) => {
    const c = convs.get(r.conv);
    const requestId = c?.wires.get(r.requestId);
    if (!c || !requestId) {
      // Forgotten or timed out while the files were being written.
      if (r.state === 'delivered' && r.files.length) {
        await rm(path.join(cfg.adapter.files.dataDir(r.conv), r.requestId), { recursive: true, force: true });
      }
      return;
    }
    if (r.state === 'declined' && r.code === STEP_UP_REQUIRED) {
      settle(c, requestId, false);
      save(r.conv, c);
      emit({ type: 'parked', conv: r.conv, requestId, reason: r.reason });
      return;
    }
    const f = settle(c, requestId, true)!;
    if (r.state === 'delivered') c.delivered.set(f.key, { source: f.proposal.source, dir: r.requestId });
    save(r.conv, c);
    if (r.state === 'delivered') {
      emit({ type: 'delivered', conv: r.conv, requestId, summary: r.summary, files: r.files, approvedUntil: r.approvedUntil });
      return;
    }
    const reason = r.state === 'declined' ? `declined: ${r.reason}` : r.state === 'expired' ? 'expired: the provider gave up' : `invalid delivery: ${r.reason}`;
    emit({ type: 'not_available', conv: r.conv, requestId, reason });
  };

  const unsubscribe = cfg.adapter.onReceived((r) => void received(r));

  // Gives up on the attempt under way at its deadline.
  const arm = (conv: string, c: Conv, requestId: string, f: Inflight) => {
    const wire = f.wire;
    c.wires.set(wire, requestId);
    f.timer = setTimeout(() => {
      cfg.adapter.cancel(wire);
      notAvailable(conv, requestId, `no answer within ${Math.round(f.waitSeconds)}s`);
    }, Math.max(f.deadline! - Date.now(), 0));
    f.timer.unref();
  };

  // One attempt at the adapter, under a fresh wire id.
  const send = (conv: string, c: Conv, requestId: string, f: Inflight) => {
    f.wire = randomUUID().replaceAll('-', '');
    f.deadline = Date.now() + f.waitSeconds * 1000;
    arm(conv, c, requestId, f);
    save(conv, c);
    const p = f.proposal;
    cfg.adapter
      .request(conv, { id: f.wire, requester: { id: f.requester }, source: p.source, params: p.params, reason: f.reason })
      .then(
        (sent) => {
          if (sent.type === 'failed') notAvailable(conv, requestId, sent.reason);
        },
        (err: unknown) => notAvailable(conv, requestId, errText(err)),
      );
  };

  const start = (conv: string, requester: string, source: Source, p: Proposal, reason: string): string => {
    const c = state(conv);
    const requestId = randomUUID().replaceAll('-', '');
    c.issued++;
    for (const id of idsOf(source, p.params)) c.ids.add(id);
    const waitSeconds = Math.min(source.max_wait_seconds, limits.maxWaitSeconds) + limits.graceSeconds;
    const f: Inflight = { waitSeconds, requester, proposal: p, reason, key: keyOf(p), wire: '' };
    c.inflight.set(requestId, f);
    emit({ type: 'requested', conv, requestId, label: redact(p.label).text, source: p.source, params: p.params });
    send(conv, c, requestId, f);
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

  // A restart resumes each attempt's remaining wait; parked requests keep waiting for approval.
  for (const conv of cfg.state?.keys() ?? []) {
    const saved = cfg.state!.get<SavedConv>(conv);
    if (!saved) continue;
    const c: Conv = {
      issued: saved.issued, ids: new Set(saved.ids), inflight: new Map(saved.inflight),
      wires: new Map(), delivered: new Map(saved.delivered),
    };
    convs.set(conv, c);
    for (const [requestId, f] of c.inflight) if (f.deadline) arm(conv, c, requestId, f);
  }

  return {
    async onRequesterText(conv, requester, text) {
      safeConv(conv);
      const c = state(conv);
      const budget = Math.min(limits.perMessage, limits.perConversation - c.issued);
      const sources = await sourcesByName();
      const taken = new Set([...[...c.delivered.values()].map((d) => d.source), ...[...c.inflight.values()].map((f) => f.proposal.source)]);
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
      const sources = await sourcesByName();
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
      if (done) return { notAvailable: `already delivered in data/${done.dir}/` };
      for (const [requestId, f] of c.inflight) {
        if (f.key !== key) continue;
        return f.timer ? { requestId } : { notAvailable: 'already asked; it waits for the requester to approve data access and is retried then' };
      }
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
      cfg.state?.delete(conv);
      for (const f of c?.inflight.values() ?? []) {
        if (!f.timer) continue;
        clearTimeout(f.timer);
        cfg.adapter.cancel(f.wire);
      }
      await cfg.adapter.files.forget(conv);
    },

    instructions: () => cfg.adapter.instructions(),

    catalog: async () => ({ sources: [...(await sourcesByName()).values()], perConversation: limits.perConversation, distinctIds: limits.distinctIds }),

    async approved(conv) {
      const c = convs.get(conv);
      let retried = 0;
      for (const [requestId, f] of c?.inflight ?? []) {
        if (f.timer) continue;
        emit({ type: 'requested', conv, requestId, label: redact(f.proposal.label).text, source: f.proposal.source, params: f.proposal.params });
        send(conv, c!, requestId, f);
        retried++;
      }
      return retried;
    },

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
