import type { FileStore, IncomingFile } from '../adapter/files.ts';
import { isRecord } from '../adapter/kit/schema.ts';
import * as sild from '@sild/client';
import {
  ApiError, BEFORE_FIRST_MESSAGE, pages, signedUrl,
  type Attachment, type Conversation, type ConversationQuery, type Envelope, type Http, type Message, type NewMessage, type Part, type Step,
} from '@sild/client';
import type { BrokerEvent, InfoBroker } from '../contracts.ts';
import { errText, truncateUtf8 } from '../util.ts';
import { REQUESTS_PER_TURN, type Log, type Ops, type SessionEvents, type SessionHandle, type SessionRunner } from './types.ts';

export type SlotConfig = {
  queue: string;
  slots: number;
  idleReleaseMs: number;
  maxClaimMs: number;
  turnTimeoutMs: number;
  // Safety net for missed realtime events.
  pollMs: number;
  log: Log;
};

const WAIT_PREFIX = 'wait-';
// Queued conversations told their place while every slot is held.
const WAITING_NOTICES = 20;
const TYPING_EVERY_MS = 4_000;
const DOWNLOAD_TIMEOUT_MS = 30_000;
const PART_TEXT_MAX_BYTES = 1024;
const NOT_FINISHED = "I couldn't finish looking into this. Send a message to try again.";
const UNAVAILABLE = 'The assistant is unavailable right now. Please try again later.';
const NOT_PROCESSED = "I couldn't process your last message. Please send it again.";
const RELEASED = 'not sent: this conversation is no longer assigned to you';

type Held = {
  conv: string;
  assignmentId: string;
  requester: string;
  // Sent to data adapters; empty when the requester is not linked to the integrator.
  providerIdentity: string;
  lastSeen: string;
  handle?: SessionHandle;
  released: boolean;
  busy: boolean;
  // An answer tool posted during the current turn.
  answered: boolean;
  timedOut: boolean;
  turnRequests: number;
  lastTyping: number;
  // Requester text and notices reach the session in arrival order.
  input: Promise<void>;
  idle?: NodeJS.Timeout;
  max?: NodeJS.Timeout;
  turn?: NodeJS.Timeout;
};

type StepsMessage = { id?: string; steps: Step[] };

// Survives a release; dropped when the conversation closes.
type ConvState = {
  // This turn's steps message; an answer closes it.
  open?: StepsMessage;
  byRequest: Map<string, { msg: StepsMessage; index: number }>;
  // Session notices that arrived while the conversation was not claimed.
  pending: string[];
  // Last requester message the session was given; absent after a restart.
  seen?: string;
  chain: Promise<void>;
};

export class SlotManager {
  private readonly http: Http;
  private readonly me: string;
  private readonly runner: SessionRunner;
  private readonly broker: InfoBroker;
  private readonly files: FileStore;
  private readonly cfg: SlotConfig;
  private readonly log: Log;

  private readonly held = new Map<string, Held>();
  private readonly convs = new Map<string, ConvState>();
  private readonly noticed = new Set<string>();
  private readonly closedHints = new Set<string>();
  private serial: Promise<void> = Promise.resolve();
  private reconcileQueued = false;
  private readonly catchUpQueued = new Set<string>();
  private fullQueued = false;
  private poll?: NodeJS.Timeout;
  private unsubscribe?: () => void;
  private stopped = false;

  constructor(http: Http, me: string, runner: SessionRunner, broker: InfoBroker, files: FileStore, cfg: SlotConfig) {
    this.http = http;
    this.me = me;
    this.runner = runner;
    this.broker = broker;
    this.files = files;
    this.cfg = cfg;
    this.log = cfg.log;
  }

  start(): void {
    this.unsubscribe = this.broker.subscribe((e) => this.onBroker(e));
    this.poll = setInterval(() => this.kick(), this.cfg.pollMs);
    this.kick(true);
  }

  // Keeps the assignments: a restarted bridge adopts them again.
  async stop(): Promise<void> {
    this.stopped = true;
    clearInterval(this.poll);
    this.unsubscribe?.();
    await this.run(async () => {
      for (const h of [...this.held.values()]) await this.free(h, 'shutdown');
    });
  }

  onConnected(): void {
    this.kick(true);
  }

  onEvent(_channel: string, env: Envelope): void {
    if (env.type === 'typing' || env.type === 'message.read' || this.isEcho(env)) return;
    const conv = env.conversation_id;
    if (env.type === 'conversation.closed' && conv) this.closedHints.add(conv);
    if (env.type === 'message.created' && conv && this.held.has(conv)) this.catchUpSoon(conv);
    else this.kick();
  }

  // Our own writes come back as events; they change nothing we do not already know.
  private isEcho(env: Envelope): boolean {
    const d = env.data;
    if (!isRecord(d)) return false;
    if (env.type === 'message.created' || env.type === 'message.updated') return d.internal_actor_id === this.me;
    if (env.type !== 'assignment.updated' || d.assignee_actor_id !== this.me) return false;
    // A close we did not make, or an assignment to us we do not hold yet, still needs a reconcile.
    const holding = !!env.conversation_id && this.held.get(env.conversation_id)?.assignmentId === d.id;
    return d.status === 'assigned' ? holding : d.status === 'closed' && !holding;
  }

  private catchUpSoon(conv: string): void {
    if (this.stopped || this.catchUpQueued.has(conv)) return;
    this.catchUpQueued.add(conv);
    void this.run(async () => {
      this.catchUpQueued.delete(conv);
      const h = this.held.get(conv);
      if (h) await this.catchUp(h);
      else this.kick();
    });
  }

  kick(full = false): void {
    if (this.stopped) return;
    if (full) this.fullQueued = true;
    if (this.reconcileQueued) return;
    this.reconcileQueued = true;
    void this.run(async () => {
      this.reconcileQueued = false;
      const f = this.fullQueued;
      this.fullQueued = false;
      await this.reconcile(f);
    });
  }

  // Every change to the set of held conversations runs here, one at a time.
  private run(fn: () => Promise<void>): Promise<void> {
    const p = this.serial.then(fn).catch((e) => this.log('slot task failed', { error: errText(e) }));
    this.serial = p;
    return p;
  }

  private async reconcile(full: boolean): Promise<void> {
    const base: ConversationQuery = { queue: this.cfg.queue, kind: 'support', status: 'open', limit: 100 };
    const mine = await this.listAll({ ...base, assignee: 'me', assignment_status: 'assigned' });
    const rows = new Map(mine.map((c) => [c.id, c]));

    for (const h of [...this.held.values()]) {
      const row = rows.get(h.conv);
      if (!row || row.assignment?.id !== h.assignmentId) await this.lose(h);
    }
    // Held by us but not by this process: a restart, or a release whose close failed.
    for (const row of mine) {
      if (this.held.has(row.id) || !row.assignment) continue;
      if (this.held.size < this.cfg.slots) await this.adopt(row, row.assignment.id);
      else await this.closeAssignment(row.assignment.id);
    }
    for (const h of [...this.held.values()]) await this.catchUp(h);
    await this.sweep(full);
    await this.fill(base);
  }

  private async fill(base: ConversationQuery): Promise<void> {
    const params: ConversationQuery = { ...base, assignment_status: 'queued', sort: 'waiting_since', order: 'asc' };
    const queued = this.held.size >= this.cfg.slots
      ? (await sild.listConversations(this.http, { ...params, limit: WAITING_NOTICES })).items
      : await this.listAll(params);
    // Notices are owed only to assignments still queued; claimed, lost and closed ones drop out.
    const stillQueued = new Set(queued.map((r) => r.assignment?.id));
    for (const key of this.noticed) if (!stillQueued.has(key)) this.noticed.delete(key);
    const waiting: Conversation[] = [];
    for (const row of queued) {
      if (!row.assignment || this.held.has(row.id)) continue;
      if (this.held.size >= this.cfg.slots) {
        waiting.push(row);
        continue;
      }
      try {
        const claimed = await sild.claimAssignment(this.http, row.assignment.id);
        await this.adopt(row, claimed.id);
      } catch (e) {
        if (isConflict(e) || isGone(e)) continue;
        throw e;
      }
    }
    for (const [ahead, row] of waiting.slice(0, WAITING_NOTICES).entries()) await this.noticeWaiting(row, ahead);
  }

  private async noticeWaiting(row: Conversation, ahead: number): Promise<void> {
    const key = row.assignment!.id;
    if (this.noticed.has(key)) return;
    this.noticed.add(key);
    try {
      // client_msg_id makes the notice idempotent across restarts.
      await sild.sendMessage(this.http, row.id, {
        body: `Waiting for the assistant · ${ahead} ahead`,
        client_msg_id: WAIT_PREFIX + key,
      });
    } catch (e) {
      this.noticed.delete(key);
      this.log('waiting notice failed', { conv: row.id, error: errText(e) });
    }
  }

  private async adopt(row: Conversation, assignmentId: string): Promise<void> {
    const client = row.members.find((m) => m.conv_role === 'client');
    const h: Held = {
      conv: row.id,
      assignmentId,
      requester: client?.internal_actor_id ?? client?.external_user_id ?? '',
      providerIdentity: client?.external_sub ?? '',
      lastSeen: BEFORE_FIRST_MESSAGE,
      released: false,
      busy: false,
      answered: false,
      timedOut: false,
      turnRequests: 0,
      lastTyping: 0,
      input: Promise.resolve(),
    };
    this.held.set(row.id, h);
    try {
      const msgs = await sild.messagesSince(this.http, row.id, BEFORE_FIRST_MESSAGE);
      h.lastSeen = msgs.at(-1)?.id ?? BEFORE_FIRST_MESSAGE;
      // After a restart nothing records what the session saw: replay what came after our last answer.
      const seen = this.convs.get(row.id)?.seen;
      const unanswered = seen
        ? msgs.filter((m) => m.id > seen && this.fromRequester(h, m))
        : msgs.slice(msgs.findLastIndex((m) => this.isAnswer(m)) + 1).filter((m) => this.fromRequester(h, m));

      h.handle = await this.runner.open(row.id, this.ops(h), this.events(h));
      if (h.released) {
        await h.handle.close();
        return;
      }
      h.max = setTimeout(() => void this.run(() => this.release(h, 'max_claim')), this.cfg.maxClaimMs);
      this.armIdle(h);
      this.log('claimed', { conv: row.id, assignment: assignmentId, unanswered: unanswered.length });

      const st = this.state(row.id);
      for (const notice of st.pending.splice(0)) this.enqueue(h, notice);
      for (const m of unanswered) this.route(h, m);
    } catch (e) {
      this.log('claim setup failed', { conv: row.id, error: errText(e) });
      await sild.sendMessage(this.http, row.id, { body: UNAVAILABLE }).catch(() => {});
      await this.release(h, 'setup failed');
    }
  }

  private async catchUp(h: Held): Promise<void> {
    const msgs = await sild.messagesSince(this.http, h.conv, h.lastSeen);
    if (h.released) return;
    for (const m of msgs) {
      h.lastSeen = m.id;
      if (!this.fromRequester(h, m)) continue;
      this.armIdle(h);
      this.route(h, m);
    }
  }

  // Our own decision to give the conversation back. Sild re-queues it on the next requester message.
  private async release(h: Held, why: string): Promise<void> {
    if (h.released || this.held.get(h.conv) !== h) return;
    // Their next message re-queues the conversation; say so rather than go quiet.
    if (h.busy && !h.answered) await this.answer(h, { body: NOT_FINISHED }).catch(() => {});
    await this.free(h, why);
    await this.closeAssignment(h.assignmentId);
    await this.requeueIfMissed(h);
    this.kick();
  }

  // A requester message that landed while the assignment was still ours did not re-queue the conversation.
  private async requeueIfMissed(h: Held): Promise<void> {
    try {
      const msgs = await sild.messagesSince(this.http, h.conv, h.lastSeen);
      if (msgs.some((m) => this.fromRequester(h, m))) await this.requeue(h.conv);
    } catch (e) {
      this.log('release check failed', { conv: h.conv, error: errText(e) });
    }
  }

  // The assignment is no longer ours: someone else took it, or the conversation closed.
  private async lose(h: Held): Promise<void> {
    let closed = false;
    try {
      closed = (await sild.getConversation(this.http, h.conv)).status === 'closed';
    } catch (e) {
      if (!isGone(e)) throw e;
      closed = true;
    }
    await this.free(h, closed ? 'conversation closed' : 'assignment lost');
    if (!closed) return;
    // Closing a conversation leaves its assignment open.
    await this.closeAssignment(h.assignmentId);
    await this.forget(h.conv);
  }

  private async free(h: Held, why: string): Promise<void> {
    h.released = true;
    clearTimeout(h.idle);
    clearTimeout(h.max);
    clearTimeout(h.turn);
    if (this.held.get(h.conv) === h) this.held.delete(h.conv);
    this.log('released', { conv: h.conv, why });
    try {
      await h.handle?.close();
    } catch (e) {
      this.log('session close failed', { conv: h.conv, error: errText(e) });
    }
  }

  private async closeAssignment(id: string): Promise<void> {
    try {
      await sild.closeAssignment(this.http, id);
    } catch (e) {
      if (!isConflict(e) && !isGone(e)) this.log('assignment close failed', { assignment: id, error: errText(e) });
    }
  }

  // Conversations closed while not held still have a transcript and files on disk.
  private async sweep(full: boolean): Promise<void> {
    const known = await this.runner.known();
    const ids = full ? known : [...this.closedHints].filter((id) => known.includes(id) || this.convs.has(id));
    this.closedHints.clear();
    for (const id of ids) {
      if (this.held.has(id)) continue;
      try {
        if ((await sild.getConversation(this.http, id)).status === 'closed') await this.forget(id);
      } catch (e) {
        if (isGone(e)) await this.forget(id);
        else this.log('sweep failed', { conv: id, error: errText(e) });
      }
    }
  }

  private async forget(conv: string): Promise<void> {
    this.convs.delete(conv);
    try {
      await this.runner.destroy(conv);
    } catch (e) {
      this.log('session cleanup failed', { conv, error: errText(e) });
    }
    try {
      await this.broker.forget(conv);
    } catch (e) {
      this.log('broker forget failed', { conv, error: errText(e) });
    }
    this.log('forgotten', { conv });
  }

  // ── session input and turns ──────────────────────────────────────────────

  private route(h: Held, m: Message): void {
    h.input = h.input.then(async () => {
      if (h.released) return;
      if (isApproval(m)) {
        const retried = await this.broker.approved(h.conv).catch((e) => {
          this.log('approval retry failed', { conv: h.conv, error: errText(e) });
          return 0;
        });
        if (!retried) await this.expireApprovals(h.conv);
        if (!h.released) {
          this.push(h, retried
            ? 'The requester approved data access. The requests waiting for it are being retried.'
            : 'The requester approved data access, but no request was waiting for it any more. Ask again for what you still need.');
        }
        this.state(h.conv).seen = m.id;
        return;
      }
      let text: string;
      try {
        text = (await this.broker.onRequesterText(h.conv, h.providerIdentity, m.body)).text;
      } catch (e) {
        this.log('router failed', { conv: h.conv, error: errText(e) });
        await this.answer(h, { body: NOT_PROCESSED }).catch(() => {});
        this.state(h.conv).seen = m.id;
        return;
      }
      const attached = await this.attachments(h.conv, m);
      if (h.released) return;
      this.push(h, requesterMessage(text) + attached);
      this.state(h.conv).seen = m.id;
    });
  }

  // Requester files land in the conversation's data directory; the session is told where.
  private async attachments(conv: string, m: Message): Promise<string> {
    if (!m.attachments?.length) return '';
    try {
      const incoming = await Promise.all(m.attachments.map((a) => this.download(a)));
      const saved = await this.files.write(conv, m.id, { source: 'requester', summary: 'attached by the requester' }, incoming);
      return saved.map((f) => `\nThe requester attached ${JSON.stringify(clip(f.name))} (${f.mime}, ${f.size} bytes): ${f.path}`).join('');
    } catch (e) {
      this.log('attachments failed', { conv, message: m.id, error: errText(e) });
      return `\nThe requester attached ${m.attachments.length} file(s) that could not be stored: ${clip(errText(e))}`;
    }
  }

  private async download(a: Attachment): Promise<IncomingFile> {
    const limit = this.files.limits.bytesPerDelivery;
    if (!a.url) throw new Error('an attachment has no download link');
    if (a.size_bytes > limit) throw new Error(`an attachment is larger than ${limit} bytes`);
    const res = await fetch(signedUrl(this.http, a.url).url, { signal: AbortSignal.timeout(DOWNLOAD_TIMEOUT_MS) });
    if (!res.ok) throw new Error(`attachment download failed: ${res.status}`);
    const data = Buffer.from(await res.arrayBuffer());
    if (data.length > limit) throw new Error(`an attachment is larger than ${limit} bytes`);
    return { name: a.filename, mime: a.mime_type, data };
  }

  private enqueue(h: Held, text: string): void {
    h.input = h.input.then(() => {
      if (h.released) this.state(h.conv).pending.push(text);
      else this.push(h, text);
    });
  }

  private push(h: Held, text: string): void {
    h.handle!.push(text);
    if (!h.busy) this.startTurn(h);
  }

  private startTurn(h: Held): void {
    h.busy = true;
    h.answered = false;
    h.timedOut = false;
    h.turnRequests = 0;
    clearTimeout(h.idle);
    clearTimeout(h.turn);
    h.turn = setTimeout(() => this.turnTimeout(h), this.cfg.turnTimeoutMs);
    this.typing(h);
  }

  private turnTimeout(h: Held): void {
    if (h.released) return;
    if (h.timedOut) {
      void this.run(() => this.release(h, 'turn did not stop'));
      return;
    }
    h.timedOut = true;
    this.log('turn timeout', { conv: h.conv });
    h.handle?.interrupt().catch((e) => this.log('interrupt failed', { conv: h.conv, error: errText(e) }));
    h.turn = setTimeout(() => this.turnTimeout(h), this.cfg.turnTimeoutMs);
  }

  private armIdle(h: Held): void {
    clearTimeout(h.idle);
    h.idle = setTimeout(() => {
      // A running turn re-arms the timer when it ends.
      if (!h.released && !h.busy) void this.run(() => this.release(h, 'idle'));
    }, this.cfg.idleReleaseMs);
  }

  private typing(h: Held): void {
    const now = Date.now();
    if (now - h.lastTyping < TYPING_EVERY_MS) return;
    h.lastTyping = now;
    sild.typing(this.http, h.conv).catch(() => {});
  }

  private events(h: Held): SessionEvents {
    return {
      activity: () => {
        if (h.released) return;
        if (!h.busy) this.startTurn(h);
        this.typing(h);
      },
      turnEnd: (outcome) => {
        if (h.released) return;
        h.busy = false;
        clearTimeout(h.turn);
        if (outcome !== 'success' && !h.answered) {
          this.log('turn failed', { conv: h.conv, outcome, timedOut: h.timedOut });
          void this.answer(h, { body: NOT_FINISHED }).catch(() => {});
        }
        this.armIdle(h);
        if (outcome === 'error_max_budget_usd') void this.run(() => this.release(h, 'budget'));
      },
      exit: (error) => {
        if (h.released) return;
        this.log('session ended', { conv: h.conv, error });
        void this.run(() => this.release(h, 'session ended'));
      },
    };
  }

  private async answer(h: Held, msg: NewMessage): Promise<string> {
    if (h.released) return RELEASED;
    await sild.sendMessage(this.http, h.conv, msg);
    h.answered = true;
    this.state(h.conv).open = undefined;
    return 'sent';
  }

  private ops(h: Held): Ops {
    return {
      reply: (text) => this.answer(h, { body: text }),
      proposeFix: (cause, fix, evidence) => this.answer(h, {
        body: [
          cause, '', ...fix.map((f, i) => `${i + 1}. ${f}`),
          '', 'Why I think so:', ...evidence.map((e) => `- ${e.finding} (${e.source})`),
        ].join('\n'),
        parts: [{
          type: 'diagnosis', cause, fix,
          evidence: evidence.map((e) => ({ finding: e.finding, source: e.source, ...(e.ref && { internal: { ref: e.ref } }) })),
        }],
      }),
      requestInfo: async (need, why) => {
        if (h.released) return RELEASED;
        if (h.turnRequests >= REQUESTS_PER_TURN) return 'not available: request limit reached; continue with what you have';
        h.turnRequests++;
        const r = await this.broker.request(h.conv, h.providerIdentity, need, why);
        if ('notAvailable' in r) return `not available: ${r.notAvailable}`;
        return `requested ${r.requestId}. The data arrives later as a message naming its files, or never; do not wait for it.`;
      },
    };
  }

  // ── broker events → steps part + session notices ─────────────────────────

  private onBroker(e: BrokerEvent): void {
    const st = this.state(e.conv);
    st.chain = st.chain
      .then(() => this.applyBroker(st, e))
      .catch((err) => this.log('step update failed', { conv: e.conv, request: e.requestId, error: errText(err) }));
  }

  private async applyBroker(st: ConvState, e: BrokerEvent): Promise<void> {
    switch (e.type) {
      case 'requested': {
        if (st.byRequest.has(e.requestId)) {
          await this.updateStep(e.conv, st, e.requestId, { state: 'running', result: undefined });
          return;
        }
        const msg = st.open ?? { steps: [] };
        st.open = msg;
        const index = msg.steps.push({
          label: clip(e.label), state: 'running', internal: { source: e.source, params: e.params },
        }) - 1;
        st.byRequest.set(e.requestId, { msg, index });
        await this.writeSteps(e.conv, msg);
        return;
      }
      case 'delivered': {
        const files = e.files.map((f) => f.path).join(', ') || 'no files';
        this.notify(e.conv, `Data ${e.requestId} arrived: ${files} — ${clip(e.summary)}`);
        await this.updateStep(e.conv, st, e.requestId, { state: 'done', result: clip(e.summary) });
        return;
      }
      case 'parked': {
        this.notify(e.conv, `Request ${e.requestId} waits for the requester to approve data access. They see one Approve button; it is retried once they approve. Tell them.`);
        await this.updateStep(e.conv, st, e.requestId, { state: 'needs_approval', result: 'Waiting for your approval' });
        return;
      }
      case 'not_available': {
        this.notify(e.conv, `Request ${e.requestId} is not available: ${clip(e.reason)}`);
        await this.updateStep(e.conv, st, e.requestId, { state: 'not_available', result: clip(e.reason) });
        return;
      }
    }
  }

  private async updateStep(conv: string, st: ConvState, requestId: string, change: Partial<Step>): Promise<void> {
    const ref = st.byRequest.get(requestId);
    if (!ref) return;
    Object.assign(ref.msg.steps[ref.index], change);
    await this.writeSteps(conv, ref.msg);
  }

  // Steps still shown as waiting for approval whose requests this process no longer holds, e.g. after a restart.
  private async expireApprovals(conv: string): Promise<void> {
    try {
      for (const m of await sild.messagesSince(this.http, conv, BEFORE_FIRST_MESSAGE)) {
        if (m.internal_actor_id !== this.me || !m.parts?.some((p) => p.type === 'steps' && p.steps.some((s) => s.state === 'needs_approval'))) continue;
        const parts = m.parts.map((p) => (p.type !== 'steps' ? p : {
          ...p, steps: p.steps.map((s) => (s.state === 'needs_approval' ? { ...s, state: 'not_available' as const, result: 'No longer waiting; ask again' } : s)),
        }));
        await sild.patchMessage(this.http, conv, m.id, parts);
      }
    } catch (e) {
      this.log('expiring approvals failed', { conv, error: errText(e) });
    }
  }

  private async writeSteps(conv: string, msg: StepsMessage): Promise<void> {
    const parts: Part[] = [{ type: 'steps', steps: msg.steps }];
    if (msg.id) {
      await sild.patchMessage(this.http, conv, msg.id, parts);
      return;
    }
    msg.id = (await sild.sendMessage(this.http, conv, { body: 'Looking into it', parts })).id;
  }

  // While released, a notice waits for the next claim, and the conversation is
  // queued again so that claim happens without the requester writing first.
  private notify(conv: string, text: string): void {
    const h = this.held.get(conv);
    if (h?.handle && !h.released) {
      this.enqueue(h, text);
      return;
    }
    this.state(conv).pending.push(text);
    void this.requeue(conv);
  }

  private async requeue(conv: string): Promise<void> {
    try {
      const c = await sild.getConversation(this.http, conv);
      if (c.status !== 'open' || (c.assignment && c.assignment.status !== 'closed')) return;
      await sild.openAssignment(this.http, conv);
      this.kick();
    } catch (e) {
      this.log('requeue failed', { conv, error: errText(e) });
    }
  }

  private state(conv: string): ConvState {
    let st = this.convs.get(conv);
    if (!st) {
      st = { byRequest: new Map(), pending: [], chain: Promise.resolve() };
      this.convs.set(conv, st);
    }
    return st;
  }

  // A queue deeper than this is served from its head; the rest waits for a later pass.
  private async listAll(query: ConversationQuery): Promise<Conversation[]> {
    const out: Conversation[] = [];
    for await (const page of pages((cursor) => sild.listConversations(this.http, { ...query, cursor }), 10)) {
      out.push(...page.items);
    }
    return out;
  }

  // ── message classification ───────────────────────────────────────────────

  // A message that answered the requester. Waiting notices and steps messages do not.
  private isAnswer(m: Message): boolean {
    if (m.internal_actor_id !== this.me) return false;
    if (m.client_msg_id?.startsWith(WAIT_PREFIX)) return false;
    return !m.parts?.some((p) => p.type === 'steps');
  }

  private fromRequester(h: Held, m: Message): boolean {
    if (m.visibility === 'internal' || m.internal_actor_id === this.me) return false;
    if (h.requester) return m.internal_actor_id === h.requester || m.external_user_id === h.requester;
    return m.sender_kind === 'user' || m.sender_kind === 'agent';
  }
}

// The system message a requester's approval of data access leaves in the conversation.
function isApproval(m: Message): boolean {
  return m.sender_kind === 'system' && !!m.parts?.some((p) => p.type === 'approval');
}

// Fenced so requester text cannot pass for an application notice.
export function requesterMessage(text: string): string {
  const safe = text.replace(/<\/requester_message/gi, '<\\/requester_message');
  return `<requester_message>\n${safe}\n</requester_message>`;
}

function clip(s: string): string {
  return truncateUtf8(s, PART_TEXT_MAX_BYTES, '…');
}

const isConflict = (e: unknown): boolean => e instanceof ApiError && e.status === 409;
// Not found, or no longer ours to read.
const isGone = (e: unknown): boolean => e instanceof ApiError && (e.status === 404 || e.status === 403);
