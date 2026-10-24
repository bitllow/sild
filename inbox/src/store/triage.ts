import { computed, makeAutoObservable, runInAction, type IComputedValue } from "mobx";
import {
  ApiError,
  approveDataAccess,
  closeConversation,
  createConversation,
  getConversation,
  listConversations,
  listMessages,
  sendMessage,
  type Assignment,
  type CloseReason,
  type Conversation,
  type ConversationQuery,
  type ConversationRow,
  type Envelope,
  type Message,
} from "@sild/client";
import { errorText, http, WITH_PROFILES } from "@/api/client";
import { stepUp } from "@/api/host";
import { AttachmentQueue } from "./attachments";
import { relativeTime } from "./map";
import { isFromRequester, isTriageActive, requesterOf, triageStatus, type TriageStatus } from "./triageStatus";

export const TRIAGE_QUEUE = "triage";
const PAGE_SIZE = 30;
// The request being written before it exists on the server.
export const DRAFT_ID = "draft";
const ONE_OPEN = "You already have an open request. Close it to start a new one.";

export type TriageFilter = "all" | "active" | "closed";

export interface TriageConversation {
  id: string;
  reference: string;
  status: Conversation["status"];
  close_reason: Conversation["close_reason"];
  assignment?: Assignment;
  members: Conversation["members"];
  lastActivity: string;
  messages: Message[];
}

export interface TriageRoot {
  meId: string | null;
}

const byTime = (a: Message, b: Message) =>
  a.created_at.localeCompare(b.created_at) || a.id.localeCompare(b.id);

function toRow(c: Conversation, messages: Message[], lastActivity?: string): TriageConversation {
  return {
    id: c.id,
    reference: c.reference || c.id,
    status: c.status,
    close_reason: c.close_reason ?? null,
    assignment: c.assignment,
    members: c.members,
    lastActivity: lastActivity || messages[messages.length - 1]?.created_at || c.created_at,
    messages: [...messages].sort(byTime),
  };
}

export class TriageStore {
  rows: TriageConversation[] = [];
  activeId: string | null = null;
  // Requester: may open requests. Operator view: sees everyone's, read only.
  canStart = false;
  viewAll = false;
  drafting = false;
  filter: TriageFilter = "all";
  loaded = false;
  cursor: string | null = null;
  hasMore = false;
  loadingMore = false;
  composer = "";
  sending = false;
  // The conversation whose data access is being approved at the host.
  approving: string | null = null;
  error: string | null = null;
  atts = new AttachmentQueue((msg) => runInAction(() => (this.error = msg)));
  // Message ids whose data requests are expanded.
  openRequests = new Set<string>();
  private listSeq = 0;
  private refreshing = new Map<string, Promise<void>>();
  private refreshAgain = new Set<string>();
  private statuses = new WeakMap<TriageConversation, IComputedValue<TriageStatus>>();

  constructor(private root: TriageRoot) {
    makeAutoObservable<TriageStore, "refreshing" | "refreshAgain" | "statuses">(this, {
      owns: false,
      atts: false,
      refreshing: false,
      refreshAgain: false,
      statuses: false,
    });
  }

  configure(canStart: boolean, viewAll: boolean) {
    this.canStart = canStart;
    this.viewAll = viewAll;
  }

  reset() {
    this.listSeq++;
    this.rows = [];
    this.activeId = null;
    this.canStart = false;
    this.viewAll = false;
    this.drafting = false;
    this.filter = "all";
    this.loaded = false;
    this.cursor = null;
    this.hasMore = false;
    this.composer = "";
    this.error = null;
    this.atts.reset();
    this.openRequests.clear();
  }

  owns(convId: string): boolean {
    return this.rows.some((c) => c.id === convId);
  }

  private get listParams(): ConversationQuery {
    return {
      queue: TRIAGE_QUEUE,
      kind: "support",
      sort: "last_activity",
      order: "desc",
      limit: PAGE_SIZE,
      expand: WITH_PROFILES,
    };
  }

  // Rows carry no parts or senders, and the status is derived from both, so
  // every row's thread is read with it.
  private hydrate = (items: ConversationRow[]) =>
    Promise.all(
      items.map(async (c) => {
        const page = await listMessages(http, c.id).catch(() => ({ items: [] as Message[] }));
        return toRow(c, page.items, c.last_activity);
      })
    );

  load = async () => {
    const seq = ++this.listSeq;
    try {
      const page = await listConversations(http, this.listParams);
      const rows = await this.hydrate(page.items);
      if (seq !== this.listSeq) return;
      runInAction(() => {
        this.rows = rows;
        this.cursor = page.has_more ? page.next_cursor : null;
        this.hasMore = page.has_more;
        this.loaded = true;
        this.ensureSelection();
      });
    } catch (e) {
      if (seq !== this.listSeq) return;
      runInAction(() => {
        this.loaded = true;
        this.error = errorText(e);
      });
    }
  };

  loadMore = async () => {
    if (!this.hasMore || this.loadingMore || !this.cursor) return;
    const seq = this.listSeq;
    this.loadingMore = true;
    try {
      const page = await listConversations(http, { ...this.listParams, cursor: this.cursor });
      const rows = await this.hydrate(page.items);
      if (seq !== this.listSeq) return;
      runInAction(() => {
        for (const r of rows) if (!this.owns(r.id)) this.rows.push(r);
        this.cursor = page.has_more ? page.next_cursor : null;
        this.hasMore = page.has_more;
      });
    } catch {
      /* transient; scrolling again retries */
    } finally {
      runInAction(() => {
        this.loadingMore = false;
      });
    }
  };

  // Re-read one conversation and its newest messages, inserting it if new.
  // Calls during a read share it, and one more read follows for what they missed.
  refresh = (id: string): Promise<void> => {
    const running = this.refreshing.get(id);
    if (running) {
      this.refreshAgain.add(id);
      return running;
    }
    const read = this.readRow(id).finally(() => {
      this.refreshing.delete(id);
      if (this.refreshAgain.delete(id)) void this.refresh(id);
    });
    this.refreshing.set(id, read);
    return read;
  };

  private readRow = async (id: string) => {
    try {
      const [conv, page] = await Promise.all([
        getConversation(http, id, { expand: WITH_PROFILES }),
        listMessages(http, id),
      ]);
      if (conv.queue && conv.queue !== TRIAGE_QUEUE) return;
      runInAction(() => {
        const held = this.rows.find((c) => c.id === id);
        const fresh = toRow(conv, page.items);
        if (!held) {
          this.rows.unshift(fresh);
          return;
        }
        // Merge by id: the page wins for what it holds; anything else held — older
        // history, or a message that arrived live while this read was in flight — stays.
        const have = new Set(fresh.messages.map((m) => m.id));
        const kept = held.messages.filter((m) => !have.has(m.id));
        const messages = [...kept, ...fresh.messages].sort((a, b) =>
          a.created_at === b.created_at ? a.id.localeCompare(b.id) : a.created_at.localeCompare(b.created_at),
        );
        Object.assign(held, { ...fresh, messages });
      });
    } catch {
      /* transient; the next event or reconnect reconciles */
    }
  };

  private ensureSelection() {
    if (this.activeId === DRAFT_ID && this.drafting) return;
    if (this.activeId && this.owns(this.activeId)) return;
    const firstActive = this.rows.find((c) => isTriageActive(this.statusOf(c)));
    this.activeId = (firstActive ?? (this.viewAll ? this.rows[0] : undefined))?.id ?? null;
  }

  // ── derived ─────────────────────────────────────────────────────────────
  statusOf = (c: TriageConversation): TriageStatus => {
    let s = this.statuses.get(c);
    if (!s) {
      s = computed(() => triageStatus(c, c.messages));
      this.statuses.set(c, s);
    }
    return s.get();
  };

  /** Is this the signed-in staff member's own request? */
  isOwn = (c: TriageConversation): boolean => {
    const me = this.root.meId;
    return !!me && requesterOf(c.members)?.internal_actor_id === me;
  };

  ownerName = (c: TriageConversation): string => {
    const r = requesterOf(c.members);
    return r?.name || r?.internal_actor_id || r?.external_user_id || "Requester";
  };

  title = (c: TriageConversation): string => {
    const r = requesterOf(c.members);
    const first = c.messages.find((m) => isFromRequester(m, r) && m.body.trim())?.body.trim();
    if (!first) return "New request";
    return first.length > 46 ? first.slice(0, 44) + "…" : first;
  };

  preview = (c: TriageConversation): string => {
    for (let i = c.messages.length - 1; i >= 0; i--) {
      const m = c.messages[i];
      if (m.sender_kind === "system" || m.visibility === "internal") continue;
      if (m.body.trim()) return m.body;
    }
    return "";
  };

  get counts(): Record<TriageFilter, number> {
    const active = this.rows.filter((c) => isTriageActive(this.statusOf(c))).length;
    return { all: this.rows.length, active, closed: this.rows.length - active };
  }

  get visibleRows(): TriageConversation[] {
    const rows = [...this.rows].sort((a, b) => b.lastActivity.localeCompare(a.lastActivity));
    if (!this.viewAll || this.filter === "all") return rows;
    const want = this.filter === "active";
    return rows.filter((c) => isTriageActive(this.statusOf(c)) === want);
  }

  get active(): TriageConversation | null {
    return this.rows.find((c) => c.id === this.activeId) ?? null;
  }

  get draftActive(): boolean {
    return this.drafting && this.activeId === DRAFT_ID;
  }

  /** The requester's open request — at most one per queue. */
  get ownOpen(): TriageConversation | undefined {
    return this.rows.find((c) => c.status === "open" && this.isOwn(c));
  }

  // ── actions ─────────────────────────────────────────────────────────────
  setFilter = (f: TriageFilter) => {
    this.filter = f;
  };

  setActive = (id: string) => {
    if (this.activeId === id) return;
    this.activeId = id;
    this.composer = "";
    this.error = null;
    this.atts.reset();
    if (id !== DRAFT_ID) void this.refresh(id);
  };

  newRequest = () => {
    const open = this.ownOpen;
    if (open) {
      this.setActive(open.id);
      this.error = ONE_OPEN;
      return;
    }
    this.drafting = true;
    this.setActive(DRAFT_ID);
  };

  setComposer = (v: string) => {
    this.composer = v;
    if (this.error) this.error = null;
  };

  toggleRequests = (messageId: string) => {
    if (this.openRequests.has(messageId)) this.openRequests.delete(messageId);
    else this.openRequests.add(messageId);
  };

  send = async () => {
    const body = this.composer.trim();
    if ((!body && this.atts.pending.length === 0) || this.sending || this.atts.isUploading) return;
    this.sending = true;
    this.error = null;
    try {
      let id = this.activeId;
      if (this.draftActive) {
        const conv = await createConversation(http, { queue: TRIAGE_QUEUE, open_assignment: true });
        runInAction(() => {
          if (!this.owns(conv.id)) this.rows.unshift(toRow(conv, []));
          this.drafting = false;
          this.activeId = conv.id;
        });
        id = conv.id;
      }
      if (!id) return;
      const msg = await sendMessage(http, id, { body, visibility: "participants", attachments: this.atts.refs() });
      runInAction(() => {
        this.composer = "";
        this.atts.clear();
        const row = this.rows.find((c) => c.id === id);
        if (row) this.append(row, msg);
      });
    } catch (e) {
      runInAction(() => {
        this.error = e instanceof ApiError && e.status === 409 ? ONE_OPEN : errorText(e);
      });
      if (e instanceof ApiError && e.status === 409) void this.load();
    } finally {
      runInAction(() => {
        this.sending = false;
      });
    }
  };

  /** Runs the host's step-up (a passkey), then tells the assistant data access is approved. */
  approve = async (conv: string, reason: string) => {
    if (this.approving) return;
    this.approving = conv;
    this.error = null;
    try {
      if (!(await stepUp(conv, reason))) {
        runInAction(() => (this.error = "Data access was not approved."));
        return;
      }
      const msg = await approveDataAccess(http, conv);
      runInAction(() => {
        const row = this.rows.find((c) => c.id === conv);
        if (row) this.append(row, msg);
      });
    } catch (e) {
      runInAction(() => (this.error = errorText(e)));
    } finally {
      runInAction(() => (this.approving = null));
    }
  };

  /** "Yes, it's fixed" → resolved; "Close conversation" → closed. */
  close = async (reason: CloseReason) => {
    if (this.draftActive) {
      this.drafting = false;
      this.activeId = null;
      this.composer = "";
      this.atts.reset();
      this.ensureSelection();
      return;
    }
    const id = this.activeId;
    if (!id || this.sending) return;
    this.sending = true;
    try {
      await closeConversation(http, id, reason);
      runInAction(() => {
        const row = this.rows.find((c) => c.id === id);
        if (row) this.markClosed(row, reason);
      });
    } catch (e) {
      runInAction(() => {
        this.error = errorText(e);
      });
    } finally {
      runInAction(() => {
        this.sending = false;
      });
    }
  };

  private markClosed(row: TriageConversation, reason: CloseReason) {
    row.status = "closed";
    row.close_reason = reason;
  }

  private append(row: TriageConversation, m: Message) {
    const i = row.messages.findIndex((x) => x.id === m.id);
    if (i >= 0) row.messages[i] = m;
    else {
      row.messages.push(m);
      row.messages.sort(byTime);
    }
    if (m.created_at > row.lastActivity) row.lastActivity = m.created_at;
  }

  // ── realtime ────────────────────────────────────────────────────────────
  onRealtime = (env: Envelope) => {
    const cid = env.conversation_id;
    if (!cid) return;
    const row = this.rows.find((c) => c.id === cid);
    if (!row) {
      if (this.loaded && env.type !== "typing" && env.type !== "message.read") void this.refresh(cid);
      return;
    }
    switch (env.type) {
      case "message.created":
      case "message.updated":
        runInAction(() => this.append(row, env.data as Message));
        break;
      case "assignment.updated": {
        const a = env.data as Assignment;
        runInAction(() => {
          // A requeue is a new assignment, not an edit of the old one.
          row.assignment = row.assignment?.id === a.id ? { ...row.assignment, ...a } : a;
        });
        break;
      }
      case "conversation.closed": {
        const reason = (env.data as { reason?: CloseReason } | null)?.reason ?? "closed";
        runInAction(() => this.markClosed(row, reason));
        break;
      }
      default:
        break;
    }
  };

  onReconnect = () => {
    if (!this.loaded) return;
    void this.load();
  };

  // Rows show their own relative time; recomputed on render.
  timeOf = (c: TriageConversation) => relativeTime(c.lastActivity);
}
