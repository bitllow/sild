// In-memory Sild for tests: the routes the bridge calls, with the queue semantics it relies on.
import { createServer, type IncomingMessage, type Server, type ServerResponse } from 'node:http';
import type { AddressInfo } from 'node:net';
import type { Assignment, Conversation, Member, Message, Part } from '@sild/client';

export const BOT = 'adm_bot';

type FakeConv = {
  id: string;
  status: 'open' | 'closed';
  queue: string;
  members: Member[];
  assignments: Assignment[];
  messages: Message[];
};

export class FakeSild {
  readonly convs = new Map<string, FakeConv>();
  readonly requests: string[] = [];
  url = '';
  private seq = 0;
  private server?: Server;

  async start(): Promise<string> {
    this.server = createServer((req, res) => {
      void this.handle(req, res).catch((e) => send(res, 500, { error: { code: 'internal', message: String(e) } }));
    });
    await new Promise<void>((r) => this.server!.listen(0, '127.0.0.1', r));
    this.url = `http://127.0.0.1:${(this.server.address() as AddressInfo).port}`;
    return this.url;
  }

  async stop(): Promise<void> {
    this.server?.closeAllConnections();
    await new Promise<void>((r) => this.server?.close(() => r()));
  }

  // ── test controls ─────────────────────────────────────────────────────────

  // A requester opens a triage conversation: queued assignment plus their first message.
  open(requester: string, text: string, queue = 'triage'): string {
    const id = this.id('c');
    this.convs.set(id, {
      id, status: 'open', queue,
      members: [{
        member_kind: 'agent', conv_role: 'client', joined_at: new Date().toISOString(),
        internal_actor_id: requester, external_sub: `fleetline.${requester}`,
      }],
      assignments: [], messages: [],
    });
    this.enqueue(id);
    this.say(id, text);
    return id;
  }

  // A requester message. Re-queues like a queue with requeue_on_reply.
  say(conv: string, text: string, from?: string): Message {
    const c = this.conv(conv);
    const sender = from ?? c.members.find((m) => m.conv_role === 'client')!.internal_actor_id!;
    const m = this.addMessage(c, { sender_kind: 'agent', internal_actor_id: sender, body: text });
    if (c.status === 'open' && !this.openAssignment(c)) this.enqueue(conv);
    return m;
  }

  // A message the bot posted earlier, e.g. before a restart.
  botSaid(conv: string, text: string, parts?: Part[]): Message {
    return this.addMessage(this.conv(conv), { sender_kind: 'bot', internal_actor_id: BOT, body: text, parts });
  }

  assignTo(conv: string, actor: string): void {
    const a = this.latest(conv)!;
    a.status = 'assigned';
    a.assignee_actor_id = actor;
  }

  close(conv: string): void {
    this.conv(conv).status = 'closed';
  }

  latest(conv: string): Assignment | undefined {
    return this.conv(conv).assignments.at(-1);
  }

  messages(conv: string): Message[] {
    return this.conv(conv).messages;
  }

  botMessages(conv: string): Message[] {
    return this.messages(conv).filter((m) => m.internal_actor_id === BOT);
  }

  // ── routes ────────────────────────────────────────────────────────────────

  private async handle(req: IncomingMessage, res: ServerResponse): Promise<void> {
    const url = new URL(req.url ?? '/', 'http://x');
    const method = req.method ?? 'GET';
    this.requests.push(`${method} ${url.pathname}`);
    if (req.headers.authorization !== 'Bearer sild_live_test') return send(res, 401, err('unauthorized'));
    const body = method === 'GET' ? {} : await readJSON(req);
    const parts = url.pathname.split('/').filter(Boolean);
    const q = url.searchParams;

    if (method === 'GET' && url.pathname === '/v1/principal') {
      return send(res, 200, { kind: 'admin', tenant_id: 't_1', subject: { id: BOT, assignments: [] } });
    }
    if (method === 'GET' && url.pathname === '/v1/realtime/token') {
      return send(res, 200, { token: 'rt', expires_at: new Date(Date.now() + 3600_000).toISOString() });
    }
    if (method === 'GET' && url.pathname === '/v1/conversations') return send(res, 200, this.list(q));
    if (parts[0] === 'v1' && parts[1] === 'conversations' && parts[2]) {
      const c = this.convs.get(parts[2]);
      if (!c) return send(res, 404, err('not_found'));
      if (method === 'GET' && parts.length === 3) return send(res, 200, view(c));
      if (method === 'GET' && parts[3] === 'messages') {
        const since = q.get('since') ?? '';
        const limit = Number(q.get('limit') ?? 50);
        const after = c.messages.filter((m) => m.id > since);
        return send(res, 200, { items: after.slice(0, limit), next_cursor: null, has_more: after.length > limit });
      }
      if (method === 'POST' && parts[3] === 'messages') {
        if (c.status === 'closed') return send(res, 409, err('conversation_closed'));
        const b = body as { body: string; parts?: Part[]; client_msg_id?: string };
        const dup = b.client_msg_id && c.messages.find((m) => m.client_msg_id === b.client_msg_id);
        if (dup) return send(res, 201, dup);
        return send(res, 201, this.addMessage(c, {
          sender_kind: 'bot', internal_actor_id: BOT, body: b.body, parts: b.parts, client_msg_id: b.client_msg_id,
        }));
      }
      if (method === 'PATCH' && parts[3] === 'messages' && parts[4]) {
        const m = c.messages.find((x) => x.id === parts[4]);
        if (!m) return send(res, 404, err('not_found'));
        if (m.internal_actor_id !== BOT) return send(res, 403, err('forbidden'));
        if (c.status === 'closed') return send(res, 409, err('conversation_closed'));
        m.parts = (body as { parts: Part[] }).parts;
        return send(res, 200, m);
      }
      if (method === 'POST' && parts[3] === 'typing') return send(res, 204);
      if (method === 'POST' && parts[3] === 'assignments') {
        if (this.openAssignment(c)) return send(res, 409, err('assignment_open'));
        this.enqueue(c.id);
        return send(res, 201, c.assignments.at(-1));
      }
    }
    if (method === 'PATCH' && parts[1] === 'assignments' && parts[2]) {
      const a = [...this.convs.values()].flatMap((c) => c.assignments).find((x) => x.id === parts[2]);
      if (!a) return send(res, 404, err('not_found'));
      const b = body as { assignee_actor_id?: string; status?: string };
      if (b.status === 'closed') {
        a.status = 'closed';
        return send(res, 200, a);
      }
      if (b.assignee_actor_id === 'me') {
        if (a.status === 'closed') return send(res, 409, err('assignment_already_closed'));
        if (a.status === 'assigned' && a.assignee_actor_id !== BOT) return send(res, 409, err('assignment_already_taken'));
        a.status = 'assigned';
        a.assignee_actor_id = BOT;
        return send(res, 200, a);
      }
      return send(res, 400, err('bad_request'));
    }
    send(res, 404, err('not_found'));
  }

  private list(q: URLSearchParams) {
    let rows = [...this.convs.values()].filter((c) => c.queue === (q.get('queue') ?? 'support'));
    const status = q.get('status');
    if (status) rows = rows.filter((c) => c.status === status);
    const as = q.get('assignment_status');
    if (as) rows = rows.filter((c) => c.assignments.at(-1)?.status === as);
    if (q.get('assignee') === 'me') rows = rows.filter((c) => c.assignments.at(-1)?.assignee_actor_id === BOT);
    if (q.get('sort') === 'waiting_since') rows.sort((a, b) => a.assignments.at(-1)!.id.localeCompare(b.assignments.at(-1)!.id));
    const limit = Number(q.get('limit') ?? 30);
    const offset = Number(q.get('cursor') ?? 0);
    const page = rows.slice(offset, offset + limit);
    const more = offset + limit < rows.length;
    return { items: page.map(view), next_cursor: more ? String(offset + limit) : null, has_more: more };
  }

  private enqueue(conv: string): void {
    const c = this.conv(conv);
    c.assignments.push({ id: this.id('as'), conversation_id: conv, status: 'queued', created_at: new Date().toISOString() });
  }

  private openAssignment(c: FakeConv): Assignment | undefined {
    const a = c.assignments.at(-1);
    return a && a.status !== 'closed' ? a : undefined;
  }

  private addMessage(c: FakeConv, m: Partial<Message>): Message {
    const msg: Message = {
      id: this.id('m'), conversation_id: c.id, sender_kind: 'agent', visibility: 'participants', channel: 'app',
      body: '', attachments: [], created_at: new Date().toISOString(), ...m,
    };
    c.messages.push(msg);
    return msg;
  }

  private conv(id: string): FakeConv {
    const c = this.convs.get(id);
    if (!c) throw new Error(`no conversation ${id}`);
    return c;
  }

  private id(prefix: string): string {
    return `${prefix}_${String(++this.seq).padStart(26, '0')}`;
  }
}

function view(c: FakeConv): Conversation {
  return {
    id: c.id, status: c.status, kind: 'support', queue: c.queue, reference: '', metadata: null,
    created_at: c.assignments[0]?.created_at ?? '', members: c.members, assignment: c.assignments.at(-1),
  };
}

const err = (code: string) => ({ error: { code, message: code } });

function send(res: ServerResponse, status: number, body?: unknown): void {
  if (body === undefined) {
    res.writeHead(status).end();
    return;
  }
  res.writeHead(status, { 'content-type': 'application/json' }).end(JSON.stringify(body));
}

async function readJSON(req: IncomingMessage): Promise<unknown> {
  const chunks: Buffer[] = [];
  for await (const c of req) chunks.push(c as Buffer);
  const raw = Buffer.concat(chunks).toString();
  return raw ? JSON.parse(raw) : {};
}

export async function waitFor(cond: () => boolean, what = 'condition', ms = 3000): Promise<void> {
  const end = Date.now() + ms;
  while (!cond()) {
    if (Date.now() > end) throw new Error(`timed out waiting for ${what}`);
    await new Promise((r) => setTimeout(r, 10));
  }
}
