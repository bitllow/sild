import { makeAutoObservable, runInAction } from "mobx";
import { adminApi, type ApiMember, type ApiMessage, type ApiQueueConversation } from "@/api/admin";
import type { RealtimeEnvelope } from "@/api/realtime";
import type { MessageAttachment, Presence } from "@/components/ds";
import { AttachmentQueue } from "./attachments";
import { clockTime, mapAttachments, relativeTime } from "./map";

// Peer conversations are direct chats between end-user parties with no support
// assignment. Everything role-related is DERIVED from the participants present —
// no role names are hardcoded (a tenant using "patient/clinician" gets those
// labels + colors automatically). The one special role is the support agent,
// identified structurally by member_kind === "agent".

export interface PeerParticipant {
  id: string;
  name: string;
  role: string;
  presence: Presence | null;
  meta: Record<string, string>;
  isAgent: boolean;
}

export interface PeerMessage {
  id: string;
  author: string;
  role: string;
  time: string;
  body: string;
  joinNote: boolean;
  isAgent: boolean;
  /** True only for the signed-in operator's own messages (right-aligned "You").
   *  Another agent in the same thread is isAgent but not mine — rendered as an
   *  incoming, named support message. */
  mine: boolean;
  attachments: MessageAttachment[];
}

export interface PeerConversation {
  id: string;
  reference: string;
  participants: PeerParticipant[];
  preview: string;
  time: string;
  lastActivity: string;
  unread: number;
  joined: boolean;
  messages: PeerMessage[];
}

// Root surface the PeerStore needs: the signed-in agent id and the shared chime.
export interface PeerRoot {
  meId: string | null;
  /** Play the reply-notification chime (honors the shared sound toggle). */
  chime(): void;
}

const isAgentRole = (role: string) => role === "support" || role === "agent";

function participantName(m: ApiMember): string {
  return m.metadata?.name || m.external_user_id || m.internal_actor_id || "Member";
}

function mapParticipant(m: ApiMember): PeerParticipant {
  const meta: Record<string, string> = {};
  for (const [k, v] of Object.entries(m.metadata || {})) {
    if (k === "name" || k === "role") continue; // name is the title; role is its own pill
    meta[k] = String(v);
  }
  const presence = (m.metadata?.presence as Presence | undefined) || null;
  return {
    id: m.external_user_id || m.internal_actor_id || "",
    name: participantName(m),
    role: m.conv_role,
    presence,
    meta,
    isAgent: m.member_kind === "agent",
  };
}

function mapPeerMessage(m: ApiMessage, participants: PeerParticipant[], meId: string | null): PeerMessage {
  const isAgent = m.sender_kind === "agent" || !!m.internal_actor_id;
  const joinNote = m.sender_kind === "system";
  const author = participants.find((p) => p.id && p.id === m.external_user_id);
  // Only the signed-in operator's own messages are "mine" (right-aligned). A
  // system join-note is authored by the joining agent's actor but reads as a
  // centered system line, never as the viewer's own bubble.
  const mine = isAgent && !joinNote && !!meId && m.internal_actor_id === meId;
  return {
    id: m.id,
    author: isAgent || joinNote ? m.author_name || "Support" : author?.name || m.external_user_id || "User",
    role: isAgent ? "support" : author?.role || "",
    time: clockTime(m.created_at),
    body: m.body,
    joinNote,
    isAgent,
    mine,
    attachments: mapAttachments(m),
  };
}

export class PeerStore {
  conversations: PeerConversation[] = [];
  activeId: string | null = null;
  loaded = false;
  loadingThread = false;
  composer = "";
  sending = false;
  // Files queued for the next message — the shared composer attachment queue.
  atts = new AttachmentQueue();
  // Search-with-autocomplete state (the peer list uses this instead of tabs).
  query = "";
  searchOpen = false;
  roleFilter: string | null = null;
  searching = false;
  // Keyset pagination for the default list (infinite scroll) — same shape as the
  // support queue. Null while a text search is active (search has its own paging).
  cursor: string | null = null;
  hasMore = false;
  loadingMore = false;
  // Client-side unread (agents aren't conversation members, so there are no read
  // receipts to derive from): bumped on inbound realtime while a peer row isn't
  // active, cleared on open. Kept out of the row objects so a list refetch keeps it.
  private unreadByConv = new Map<string, number>();
  private searchTimer: ReturnType<typeof setTimeout> | null = null;
  private searchSeq = 0;

  constructor(private root: PeerRoot) {
    makeAutoObservable(this, { owns: false, atts: false });
  }

  reset() {
    this.conversations = [];
    this.activeId = null;
    this.loaded = false;
    this.query = "";
    this.searchOpen = false;
    this.roleFilter = null;
    this.searching = false;
    this.cursor = null;
    this.hasMore = false;
    this.composer = "";
    this.atts.reset();
    this.unreadByConv.clear();
  }

  // A text search is active — the list shows server search hits, not the paginated
  // default list, so infinite scroll is paused (search has its own result set).
  private get isSearching(): boolean {
    return this.query.trim().length > 0;
  }

  // owns reports whether a conversation id belongs to the peer surface, so the
  // root can route its realtime events here. Not observable (called from realtime).
  owns(convId: string): boolean {
    return convId === this.activeId || this.conversations.some((c) => c.id === convId);
  }

  // loadConversations (re)loads the FIRST page of the default list, honoring the
  // active role filter. Server-side keyset pagination — not a fetch-all.
  loadConversations = async () => {
    try {
      const { conversations, next_cursor, has_more } = await adminApi.listPeerConversations({
        role: this.roleFilter || undefined,
      });
      runInAction(() => {
        this.conversations = conversations.map((c) => this.buildRow(c));
        this.cursor = has_more ? next_cursor : null;
        this.hasMore = has_more;
        this.loaded = true;
        if (!this.activeId && this.conversations.length) void this.setActive(this.conversations[0].id);
      });
    } catch {
      runInAction(() => {
        this.loaded = true;
      });
    }
  };

  // loadMore appends the next keyset page (infinite scroll), default list only.
  loadMore = async () => {
    if (this.isSearching || !this.hasMore || this.loadingMore || !this.cursor) return;
    this.loadingMore = true;
    try {
      const { conversations, next_cursor, has_more } = await adminApi.listPeerConversations({
        role: this.roleFilter || undefined,
        cursor: this.cursor,
      });
      runInAction(() => {
        const seen = new Set(this.conversations.map((c) => c.id));
        for (const c of conversations) if (!seen.has(c.id)) this.conversations.push(this.buildRow(c));
        this.cursor = has_more ? next_cursor : null;
        this.hasMore = has_more;
        this.loadingMore = false;
      });
    } catch {
      runInAction(() => {
        this.loadingMore = false;
      });
    }
  };

  // runSearch replaces the list with server search hits (GET /admin/search?peer=true
  // — the shared search backend, so id/metadata/keyword matching matches support
  // search). Hits are hydrated into peer rows via the shared conversation endpoints.
  private runSearch = async (q: string) => {
    const seq = ++this.searchSeq;
    runInAction(() => {
      this.searching = true;
    });
    try {
      const { conversations } = await adminApi.searchPeer(q);
      const rows = await Promise.all(conversations.map((hit) => this.hydrateHit(hit.conversation_id, hit.snippet)));
      if (seq !== this.searchSeq) return; // stale response
      runInAction(() => {
        this.conversations = rows.filter((r): r is PeerConversation => r !== null);
        this.cursor = null;
        this.hasMore = false;
        this.searching = false;
      });
    } catch {
      runInAction(() => {
        this.searching = false;
      });
    }
  };

  // hydrateHit turns a search hit id into a peer row using the shared conversation
  // + messages endpoints (same pattern as the support inbox's search hydration).
  private async hydrateHit(id: string, snippet?: string): Promise<PeerConversation | null> {
    try {
      const [conv, page] = await Promise.all([adminApi.getConversation(id), adminApi.listMessages(id)]);
      const participants = conv.members.map(mapParticipant);
      const messages = [...page.messages]
        .sort((a, b) => a.id.localeCompare(b.id))
        .map((m) => mapPeerMessage(m, participants, this.root.meId));
      const last = page.messages[page.messages.length - 1];
      const lastActivity = last?.created_at || conv.created_at;
      return {
        id: conv.id,
        reference: conv.reference || conv.id,
        participants,
        preview: snippet || messages.filter((m) => !m.joinNote).slice(-1)[0]?.body || "",
        time: relativeTime(lastActivity),
        lastActivity,
        unread: this.unreadByConv.get(conv.id) || 0,
        joined: participants.some((p) => p.isAgent),
        messages,
      };
    } catch {
      return null;
    }
  }

  private buildRow(c: ApiQueueConversation): PeerConversation {
    const participants = c.members.map(mapParticipant);
    const joined = participants.some((p) => p.isAgent);
    const existing = this.conversations.find((x) => x.id === c.id);
    return {
      id: c.id,
      reference: c.reference || c.id,
      participants,
      preview: c.last_message?.body || "",
      time: relativeTime(c.last_activity),
      lastActivity: c.last_activity,
      unread: this.unreadByConv.get(c.id) || 0,
      joined,
      messages: existing?.messages || [],
    };
  }

  setActive = async (id: string) => {
    this.activeId = id;
    this.composer = "";
    this.atts.reset(); // discard any upload still in flight for the previous conv
    this.unreadByConv.set(id, 0);
    const conv = this.conversations.find((c) => c.id === id);
    if (conv) conv.unread = 0;
    await this.loadMessages(id);
  };

  loadMessages = async (id: string) => {
    this.loadingThread = true;
    try {
      const page = await adminApi.listMessages(id);
      runInAction(() => {
        const conv = this.conversations.find((c) => c.id === id);
        if (conv) {
          conv.messages = [...page.messages]
            .sort((a, b) => a.id.localeCompare(b.id))
            .map((m) => mapPeerMessage(m, conv.participants, this.root.meId));
        }
        this.loadingThread = false;
      });
    } catch {
      runInAction(() => {
        this.loadingThread = false;
      });
    }
  };

  setComposer = (v: string) => {
    this.composer = v;
  };

  // send posts the agent's message; the first send implicitly joins them (the
  // backend adds the agent participant + a system join-note). We optimistically
  // append the returned message; the join-note + membership arrive over realtime.
  send = async () => {
    const id = this.activeId;
    const body = this.composer.trim();
    if (!id || (!body && this.atts.pending.length === 0) || this.sending || this.atts.isUploading) return;
    this.sending = true;
    this.composer = "";
    const refs = this.atts.refs();
    try {
      const msg = await adminApi.postPeerMessage(id, body, refs);
      runInAction(() => {
        this.atts.clear();
        const conv = this.conversations.find((c) => c.id === id);
        if (conv) {
          this.appendMessage(conv, msg);
          conv.joined = true;
        }
      });
      // The implicit join added the agent participant + a join-note; reload the
      // conversation so the participants panel + "You joined" marker reflect it.
      void this.loadConversations();
      void this.loadMessages(id);
    } catch {
      /* transient */
    } finally {
      runInAction(() => {
        this.sending = false;
      });
    }
  };

  private appendMessage(conv: PeerConversation, m: ApiMessage) {
    if (conv.messages.some((x) => x.id === m.id)) return;
    conv.messages.push(mapPeerMessage(m, conv.participants, this.root.meId));
    conv.messages.sort((a, b) => a.id.localeCompare(b.id));
    if (m.sender_kind !== "system") {
      conv.preview = m.body;
      conv.lastActivity = m.created_at;
      conv.time = relativeTime(m.created_at);
    }
  }

  // ── realtime (routed here by the root for peer conversation ids) ──────────
  onRealtime = (env: RealtimeEnvelope) => {
    const cid = env.conversation_id;
    if (!cid) return;
    if (env.type === "message.created") {
      const m = env.data as ApiMessage;
      const conv = this.conversations.find((c) => c.id === cid);
      if (!conv) {
        void this.loadConversations();
        return;
      }
      const own = m.sender_kind === "agent" || !!m.internal_actor_id;
      const inbound = !own && m.sender_kind !== "system";
      runInAction(() => {
        this.appendMessage(conv, m);
        if (inbound && cid !== this.activeId) {
          const n = (this.unreadByConv.get(cid) || 0) + 1;
          this.unreadByConv.set(cid, n);
          conv.unread = n;
        }
      });
      if (inbound) this.root.chime(); // match the support inbox's inbound chime
    } else if (env.type === "member.added") {
      // Someone joined (an agent stepped in) — refresh participants + the row.
      void this.loadConversations();
      if (cid === this.activeId) void this.loadMessages(cid);
    }
  };

  // A nudge on the tenant peer channel (new peer conversation created): refresh
  // the list. No resubscribe needed — the single peer:<tenant> channel already
  // covers every peer conversation, including ones created after connect.
  onTenantNudge = () => {
    if (!this.loaded) return;
    void this.loadConversations();
  };

  // ── search + filter (server-side) ────────────────────────────────────────
  // Free text runs a debounced server search (shared search backend, peer-scoped);
  // clearing it restores the paginated default list. Mirrors the support inbox.
  setQuery = (v: string) => {
    this.query = v;
    this.searchOpen = true;
    if (this.searchTimer) clearTimeout(this.searchTimer);
    if (!v.trim()) {
      this.searching = false;
      void this.loadConversations();
      return;
    }
    this.searchTimer = setTimeout(() => void this.runSearch(v), 280);
  };
  openSearch = () => {
    this.searchOpen = true;
  };
  closeSearch = () => {
    this.searchOpen = false;
  };
  // Picking a role clears any text search and reloads the list filtered server-side.
  pickRole = (role: string) => {
    this.roleFilter = role;
    this.query = "";
    this.searchOpen = false;
    void this.loadConversations();
  };
  clearRole = () => {
    this.roleFilter = null;
    void this.loadConversations();
  };

  private partyRoles(c: PeerConversation): string[] {
    return c.participants.filter((p) => !p.isAgent).map((p) => p.role);
  }

  // The distinct non-agent roles in first-appearance order — the derived ordering
  // that maps role → color (index 0 = blue, 1 = green, …; agent = coral).
  get roleOrder(): string[] {
    const seen: string[] = [];
    for (const c of this.conversations) {
      for (const r of this.partyRoles(c)) if (r && !seen.includes(r)) seen.push(r);
    }
    return seen;
  }

  roleStyle(role: string, isAgent = false): { color: string; bg: string } {
    if (isAgent || isAgentRole(role)) return { color: "var(--coral-600)", bg: "var(--coral-50)" };
    const ramp = [
      ["--blue-600", "--blue-50"],
      ["--green-600", "--green-50"],
      ["--amber-600", "--amber-50"],
      ["--red-600", "--red-50"],
    ];
    const [c, b] = ramp[this.roleOrder.indexOf(role)] || ["--slate-600", "--slate-100"];
    return { color: `var(${c})`, bg: `var(${b})` };
  }

  // Role-pair type tag for a row: label = non-agent roles joined by " · ";
  // color = green for mixed roles (different parties) vs blue for same role.
  kindLabel(c: PeerConversation): string {
    return this.partyRoles(c).join(" · ");
  }
  isMixed(c: PeerConversation): boolean {
    return new Set(this.partyRoles(c)).size > 1;
  }

  // The list shown in the left column. Filtering (role) + free-text search run
  // server-side (loadConversations / runSearch), so this is just the current set.
  get filtered(): PeerConversation[] {
    return this.conversations;
  }

  // Autocomplete "Filter by role" suggestions — one row per distinct role (+ count)
  // over the loaded rows. The set of roles is small and repeats, so the loaded page
  // surfaces them; picking one re-queries the server (pickRole).
  get roleSuggestions(): { role: string; count: number }[] {
    const q = this.query.trim().toLowerCase();
    return this.roleOrder
      .filter((r) => !q || r.toLowerCase().includes(q))
      .map((r) => ({ role: r, count: this.conversations.filter((c) => this.partyRoles(c).includes(r)).length }));
  }
  // "Conversations" suggestions in the dropdown = the top of the current (server
  // list or server search) results, so the dropdown always reflects server state.
  get convSuggestions(): { id: string; title: string; reference: string }[] {
    if (!this.query.trim()) return [];
    return this.conversations.slice(0, 5).map((c) => ({ id: c.id, title: this.title(c), reference: c.reference }));
  }

  // Row / header title: non-agent participant names.
  title(c: PeerConversation): string {
    const parties = c.participants.filter((p) => !p.isAgent);
    return parties.map((p) => p.name).join("  ·  ") || c.reference;
  }

  get active(): PeerConversation | null {
    return this.conversations.find((c) => c.id === this.activeId) || null;
  }
  get attention(): number {
    return this.conversations.filter((c) => c.unread > 0).length;
  }
}
