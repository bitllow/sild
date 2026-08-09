import { makeAutoObservable, runInAction } from "mobx";
import {
  adminApi,
  profilesOf,
  type ApiMember,
  type ApiMessage,
  type ApiQueueConversation,
  type ConversationParams,
} from "@/api/admin";
import type { RealtimeEnvelope } from "@/api/realtime";
import type { MessageAttachment, Presence } from "@/components/ds";
import { AttachmentQueue } from "./attachments";
import { clockTime, mapAttachments, relativeTime, type Profiles } from "./map";

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
  return m.name || m.external_user_id || m.internal_actor_id || "Member";
}

// profile is the person's blob from the response's contacts block; a member view
// carries only the name.
function mapParticipant(m: ApiMember, profile?: Record<string, unknown>): PeerParticipant {
  const meta: Record<string, string> = {};
  for (const [k, v] of Object.entries(profile || {})) {
    if (k === "name" || k === "role") continue; // name is the title; role is its own pill
    meta[k] = String(v);
  }
  const presence = (profile?.presence as Presence | undefined) || null;
  return {
    id: m.external_user_id || m.internal_actor_id || "",
    name: participantName(m),
    role: m.conv_role,
    presence,
    meta,
    isAgent: m.member_kind === "agent",
  };
}

// mapParticipants pairs each member with its profile from the response's block.
function mapParticipants(members: ApiMember[], profiles: Profiles): PeerParticipant[] {
  return members.map((m) => mapParticipant(m, m.external_user_id ? profiles[m.external_user_id] : undefined));
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
  // A failed send / upload surfaced inline above the composer, so an error is
  // never swallowed silently (the typed body is also kept — see send()).
  sendError: string | null = null;
  // Files queued for the next message — the shared composer attachment queue.
  // The onError callback surfaces a failed upload instead of it vanishing.
  atts = new AttachmentQueue((msg) => runInAction(() => (this.sendError = msg)));
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
  // Bumped whenever the list identity changes (role filter, query). A page
  // in flight when it changes belongs to the previous filter set, so appending it
  // would mix results and hand back a cursor minted for a different fingerprint.
  private listSeq = 0;

  constructor(private root: PeerRoot) {
    makeAutoObservable(this, { owns: false, atts: false });
  }

  reset() {
    this.retireInFlight(); // or a response in flight lands into the cleared store
    this.conversations = [];
    this.activeId = null;
    this.loaded = false;
    this.query = "";
    this.searchOpen = false;
    this.roleFilter = null;
    this.searching = false;
    this.composer = "";
    this.sendError = null;
    this.atts.reset();
    this.unreadByConv.clear();
  }

  // A text search is active. Search is a filter on the same list endpoint, so it
  // pages the same way — only the filter set differs.
  private get isSearching(): boolean {
    return this.query.trim().length > 0;
  }

  // listParams is the filter set for the current mode. A cursor is bound to a
  // fingerprint of these, so the continuation MUST send exactly what the first
  // page sent — building both from here is what keeps them identical.
  private listParams(): ConversationParams {
    if (this.isSearching) return { kind: "peer", q: this.query.trim() };
    return { kind: "peer", role: this.roleFilter || undefined };
  }

  // owns reports whether a conversation id belongs to the peer surface, so the
  // root can route its realtime events here. Not observable (called from realtime).
  owns(convId: string): boolean {
    return convId === this.activeId || this.conversations.some((c) => c.id === convId);
  }

  // loadConversations (re)loads the FIRST page of the default list, honoring the
  // active role filter. Server-side keyset pagination — not a fetch-all.
  loadConversations = async () => {
    const seq = ++this.listSeq;
    try {
      const { items, next_cursor, has_more, contacts } = await adminApi.listConversations(this.listParams());
      if (seq !== this.listSeq) return; // a newer load superseded this one
      const profiles = profilesOf(contacts);
      runInAction(() => {
        this.conversations = items.map((c) => this.buildRow(c, profiles));
        this.cursor = has_more ? next_cursor : null;
        this.hasMore = has_more;
        this.loaded = true;
        this.ensureSelection();
      });
    } catch {
      runInAction(() => {
        this.loaded = true;
      });
    }
  };

  // loadMore appends the next keyset page (infinite scroll), search included.
  loadMore = async () => {
    if (!this.hasMore || this.loadingMore || !this.cursor) return;
    const seq = this.listSeq;
    runInAction(() => {
      this.loadingMore = true;
    });
    try {
      const { items, next_cursor, has_more, contacts } = await adminApi.listConversations({
        ...this.listParams(),
        cursor: this.cursor,
      });
      if (seq !== this.listSeq) return; // filter changed mid-flight — drop this page
      const profiles = profilesOf(contacts);
      runInAction(() => {
        const seen = new Set(this.conversations.map((c) => c.id));
        for (const it of items) {
          if (!seen.has(it.id)) this.conversations.push(this.buildRow(it, profiles));
        }
        this.cursor = has_more ? next_cursor : null;
        this.hasMore = has_more;
      });
    } catch {
      /* transient; user can scroll again to retry */
    } finally {
      // Unconditional: a superseded page still owns the flag, and nothing else
      // clears it — leaving it set wedges pagination for good.
      runInAction(() => {
        this.loadingMore = false;
      });
    }
  };

  // Search is a filter on the same list endpoint, so it returns full rows and
  // pages like any other list.
  private runSearch = async (q: string) => {
    const seq = ++this.searchSeq;
    this.listSeq++; // a page in flight for the previous query must not land here
    runInAction(() => {
      this.searching = true;
    });
    try {
      const { items, next_cursor, has_more, contacts } = await adminApi.listConversations({ kind: "peer", q });
      if (seq !== this.searchSeq) return; // stale response
      const profiles = profilesOf(contacts);
      runInAction(() => {
        this.conversations = items.map((it) => this.buildRow(it, profiles, it.snippet));
        this.cursor = has_more ? next_cursor : null;
        this.hasMore = has_more;
        this.searching = false;
      });
    } catch {
      runInAction(() => {
        this.searching = false;
      });
    }
  };

  // fetchRow turns a conversation id into a peer row using the shared conversation
  // + messages endpoints (same pattern as the support inbox's search hydration).
  // Used both by search hydration and by surface() for a single live arrival.
  private async fetchRow(id: string, snippet?: string): Promise<PeerConversation | null> {
    try {
      const [conv, page] = await Promise.all([adminApi.getConversation(id), adminApi.listMessages(id)]);
      const participants = mapParticipants(conv.members, profilesOf(conv.contacts));
      const messages = [...page.items]
        .sort((a, b) => a.id.localeCompare(b.id))
        .map((m) => mapPeerMessage(m, participants, this.root.meId));
      const last = page.items[page.items.length - 1];
      const lastActivity = last?.created_at || conv.created_at;
      return {
        id: conv.id,
        reference: conv.reference || conv.id,
        participants,
        preview: snippet || messages.filter((m) => !m.joinNote).slice(-1)[0]?.body || "",
        time: relativeTime(lastActivity),
        lastActivity,
        unread: this.unreadByConv.get(conv.id) || 0,
        joined: this.hasJoined(participants),
        messages,
      };
    } catch {
      return null;
    }
  }

  // surface inserts or refreshes a SINGLE conversation's row in place — the
  // targeted alternative to loadConversations() for a live arrival. It preserves
  // already-loaded pages (infinite scroll) and the current ordering instead of
  // collapsing back to page 1. No-op during a text search, whose result set must
  // not be mutated by background events.
  private surface = async (id: string) => {
    if (this.isSearching) return;
    const row = await this.fetchRow(id);
    if (!row) return;
    runInAction(() => {
      const existing = this.conversations.find((c) => c.id === id);
      if (existing) {
        // Refresh the fields a join / new message changes, but keep the existing
        // object — and thus its optimistic messages, unread count, and position.
        existing.participants = row.participants;
        existing.joined = row.joined;
        existing.preview = row.preview;
        existing.time = row.time;
        existing.lastActivity = row.lastActivity;
      } else {
        // New (or previously-unloaded) conversation: peer rows are newest-activity
        // first and a live arrival is the newest, so prepend it.
        this.conversations.unshift(row);
      }
    });
  };

  // refreshParticipants re-reads just the members of a loaded conversation (a
  // getConversation, no message page) and updates its roster + joined flag in
  // place — for member.added, which changes nothing but who's in the room. Cheaper
  // than surface(): a join needs no message fetch, and for the active conversation
  // loadMessages already pulls the page (so surface() here would fetch it twice).
  private refreshParticipants = async (id: string) => {
    if (!this.conversations.some((c) => c.id === id)) return;
    try {
      const conv = await adminApi.getConversation(id);
      const participants = mapParticipants(conv.members, profilesOf(conv.contacts));
      runInAction(() => {
        const row = this.conversations.find((c) => c.id === id);
        if (!row) return;
        row.participants = participants;
        row.joined = this.hasJoined(participants);
      });
    } catch {
      /* transient */
    }
  };

  // Whether THIS operator stepped in — any-agent would show "You joined" to every
  // colleague as soon as one of them joined.
  private hasJoined(participants: PeerParticipant[]): boolean {
    const me = this.root.meId;
    return !!me && participants.some((p) => p.isAgent && p.id === me);
  }

  private buildRow(c: ApiQueueConversation, profiles: Profiles, snippet?: string): PeerConversation {
    const participants = mapParticipants(c.members, profiles);
    const joined = this.hasJoined(participants);
    const existing = this.conversations.find((x) => x.id === c.id);
    return {
      id: c.id,
      reference: c.reference || c.id,
      participants,
      // Preview the matching fragment, or an old-message hit looks unrelated.
      preview: snippet || c.last_message?.body || "",
      time: relativeTime(c.last_activity),
      lastActivity: c.last_activity,
      unread: this.unreadByConv.get(c.id) || 0,
      joined,
      messages: existing?.messages || [],
    };
  }

  // ensureSelection defaults the open thread to the first row when nothing valid
  // is selected — the single expression of "never sit on an empty pane", shared by
  // the initial load and the live close of the active conversation.
  private ensureSelection() {
    if (!this.activeId && this.conversations.length) void this.setActive(this.conversations[0].id);
  }

  setActive = async (id: string) => {
    this.activeId = id;
    this.composer = "";
    this.sendError = null;
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
          conv.messages = [...page.items]
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
    if (this.sendError) this.sendError = null;
  };

  // send posts the agent's message; the first send implicitly joins them (the
  // backend adds the agent participant + a system join-note). We optimistically
  // append the returned message; the join-note + membership arrive over realtime.
  send = async () => {
    const id = this.activeId;
    const body = this.composer.trim();
    if (!id || (!body && this.atts.pending.length === 0) || this.sending || this.atts.isUploading) return;
    this.sending = true;
    this.sendError = null;
    // Do NOT clear the composer until the send succeeds: if the request fails the
    // operator's typed message is preserved (and shown with an error) rather than
    // silently lost. Attachments are likewise cleared only on success (refs()).
    const refs = this.atts.refs();
    try {
      // The implicit join happens server-side, from operator + peer conversation.
      const msg = await adminApi.postMessage(id, body, "participants", refs);
      runInAction(() => {
        this.composer = "";
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
      runInAction(() => {
        this.sendError = "Couldn’t send — check your connection and try again.";
      });
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
      const own = m.sender_kind === "agent" || !!m.internal_actor_id;
      const inbound = !own && m.sender_kind !== "system";
      const bumpUnread = inbound && cid !== this.activeId;
      const conv = this.conversations.find((c) => c.id === cid);
      if (!conv) {
        // A message for a conversation not in the loaded list (a new peer chat, or
        // one beyond the loaded pages). Surface JUST that row — preserving the
        // already-loaded pages and their order — instead of resetting to page 1.
        // Skipped while a text search is active, whose result set we must not touch.
        if (!this.isSearching) {
          if (bumpUnread) this.unreadByConv.set(cid, (this.unreadByConv.get(cid) || 0) + 1);
          void this.surface(cid);
        }
        return;
      }
      runInAction(() => {
        this.appendMessage(conv, m);
        if (bumpUnread) {
          const n = (this.unreadByConv.get(cid) || 0) + 1;
          this.unreadByConv.set(cid, n);
          conv.unread = n;
        }
      });
      if (inbound) this.root.chime(); // match the support inbox's inbound chime
    } else if (env.type === "member.added") {
      // Someone joined (an agent stepped in) — refresh that row's participants in
      // place, preserving loaded pages / scroll depth (not a full first-page
      // refetch). A join changes only the roster, so this reads the conversation
      // WITHOUT the message page. Skipped while a text search is active; the
      // participants panel still refreshes on open.
      if (!this.isSearching) void this.refreshParticipants(cid);
      if (cid === this.activeId) void this.loadMessages(cid);
    } else if (env.type === "conversation.closed") {
      // A closed peer conversation is read-only and drops off the peer surface —
      // remove it live so an observing operator isn't left composing into a dead
      // thread (the backend fans conversation.closed out to the peer channel).
      const wasActive = cid === this.activeId;
      runInAction(() => {
        this.conversations = this.conversations.filter((c) => c.id !== cid);
        this.unreadByConv.delete(cid);
        if (wasActive) {
          this.activeId = null;
          this.composer = "";
          this.sendError = null;
          this.atts.reset();
        }
      });
      // If the closed thread was the open one, land on the next conversation
      // rather than an empty pane.
      if (wasActive) this.ensureSelection();
    }
  };

  // A nudge on the tenant peer channel (new peer conversation created): refresh
  // the list. No resubscribe needed — the single peer:<tenant> channel already
  // covers every peer conversation, including ones created after connect.
  onTenantNudge = (cid?: string) => {
    if (!this.loaded || this.isSearching) return; // don't clobber active search results
    // A specific conversation nudge (a new peer chat, or a message for one not in
    // the loaded pages) surfaces just that row, preserving loaded pages; a bare
    // nudge falls back to a first-page refresh.
    if (cid) void this.surface(cid);
    else void this.loadConversations();
  };

  // A profile changed somewhere in the tenant. Names live on the contact, so a
  // loaded row can be showing a stale one — but a profile can never create,
  // remove or reorder a conversation. So this merges into rows already loaded
  // and does nothing else: no append, no cursor move, no selection change,
  // which is what keeps later pages, a search result set and the open thread
  // intact where a list reload would collapse them to page one.
  onProfileNudge = async () => {
    if (!this.loaded) return;
    const seq = this.listSeq;
    try {
      const { items, contacts } = await adminApi.listConversations(this.listParams());
      if (seq !== this.listSeq) return; // the list moved under us
      const profiles = profilesOf(contacts);
      runInAction(() => {
        for (const it of items) {
          const row = this.conversations.find((c) => c.id === it.id);
          if (!row) continue;
          row.participants = mapParticipants(it.members, profiles);
          row.joined = this.hasJoined(row.participants);
        }
      });
    } catch {
      /* transient; the next event or reconnect reconciles */
    }
  };

  // ── search + filter (server-side) ────────────────────────────────────────
  // Free text runs a debounced server search (shared search backend, peer-scoped);
  // clearing it restores the paginated default list. Mirrors the support inbox.
  setQuery = (v: string) => {
    this.query = v;
    this.searchOpen = true;
    if (this.searchTimer) clearTimeout(this.searchTimer);
    // query updates on every keystroke while runSearch is debounced, so anything
    // in flight belongs to text the user has already replaced.
    this.retireInFlight();
    if (!v.trim()) {
      this.searching = false;
      void this.loadConversations();
      return;
    }
    this.searchTimer = setTimeout(() => void this.runSearch(v), 280);
  };

  // retireInFlight abandons every request already on the wire and drops the
  // cursor they were positioned by. BOTH generations: a search response landing
  // after the default list was restored would replace it with stale hits.
  private retireInFlight() {
    this.listSeq++;
    this.searchSeq++;
    this.cursor = null;
    this.hasMore = false;
  }
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
    this.retireInFlight();
    void this.loadConversations();
  };
  clearRole = () => {
    this.roleFilter = null;
    this.retireInFlight();
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
