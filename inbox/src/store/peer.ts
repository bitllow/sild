import { makeAutoObservable, runInAction } from "mobx";
import { adminApi, type ApiMember, type ApiMessage, type ApiQueueConversation } from "@/api/admin";
import type { RealtimeEnvelope } from "@/api/realtime";
import type { Presence } from "@/components/ds";
import { clockTime, relativeTime } from "./map";

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

// Root surface the PeerStore needs: the signed-in agent id, and a way to ask the
// shared realtime connection to resubscribe when a brand-new peer conversation
// appears (subscriptions are derived at connect time).
export interface PeerRoot {
  meId: string | null;
  requestReconnect(): void;
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
  };
}

export class PeerStore {
  conversations: PeerConversation[] = [];
  activeId: string | null = null;
  loaded = false;
  loadingThread = false;
  composer = "";
  sending = false;
  // Search-with-autocomplete state (the peer list uses this instead of tabs).
  query = "";
  searchOpen = false;
  roleFilter: string | null = null;
  // Client-side unread (agents aren't conversation members, so there are no read
  // receipts to derive from): bumped on inbound realtime while a peer row isn't
  // active, cleared on open. Kept out of the row objects so a list refetch keeps it.
  private unreadByConv = new Map<string, number>();

  constructor(private root: PeerRoot) {
    makeAutoObservable(this, { owns: false });
  }

  reset() {
    this.conversations = [];
    this.activeId = null;
    this.loaded = false;
    this.query = "";
    this.searchOpen = false;
    this.roleFilter = null;
    this.composer = "";
    this.unreadByConv.clear();
  }

  // owns reports whether a conversation id belongs to the peer surface, so the
  // root can route its realtime events here. Not observable (called from realtime).
  owns(convId: string): boolean {
    return convId === this.activeId || this.conversations.some((c) => c.id === convId);
  }

  loadConversations = async () => {
    try {
      const { conversations } = await adminApi.listPeerConversations();
      runInAction(() => {
        this.conversations = conversations.map((c) => this.buildRow(c));
        this.loaded = true;
        if (!this.activeId && this.conversations.length) void this.setActive(this.conversations[0].id);
      });
    } catch {
      runInAction(() => {
        this.loaded = true;
      });
    }
  };

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
    if (!id || !body || this.sending) return;
    this.sending = true;
    this.composer = "";
    try {
      const msg = await adminApi.postPeerMessage(id, body);
      runInAction(() => {
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
      runInAction(() => {
        this.appendMessage(conv, m);
        const own = m.sender_kind === "agent" || !!m.internal_actor_id;
        if (cid !== this.activeId && !own && m.sender_kind !== "system") {
          const n = (this.unreadByConv.get(cid) || 0) + 1;
          this.unreadByConv.set(cid, n);
          conv.unread = n;
        }
      });
    } else if (env.type === "member.added") {
      // Someone joined (an agent stepped in) — refresh participants + the row.
      void this.loadConversations();
      if (cid === this.activeId) void this.loadMessages(cid);
    }
  };

  // A tenant-wide nudge (new peer conversation created): refresh the list and, if
  // a genuinely new conversation appeared, resubscribe realtime for its channel.
  onTenantNudge = () => {
    if (!this.loaded) return;
    const before = new Set(this.conversations.map((c) => c.id));
    void this.loadConversations().then(() => {
      if (this.conversations.some((c) => !before.has(c.id))) this.root.requestReconnect();
    });
  };

  // ── search + filter (derived, tenant-agnostic) ───────────────────────────
  setQuery = (v: string) => {
    this.query = v;
    this.searchOpen = true;
  };
  openSearch = () => {
    this.searchOpen = true;
  };
  closeSearch = () => {
    this.searchOpen = false;
  };
  pickRole = (role: string) => {
    this.roleFilter = role;
    this.query = "";
    this.searchOpen = false;
  };
  clearRole = () => {
    this.roleFilter = null;
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

  // The free-text haystack for a conversation: reference, and for every
  // participant their name, id (external_user_id / internal_actor_id) and all
  // metadata values (phone, vehicle, plan…), plus the last-message preview — so
  // an agent can find a peer chat by trip reference, a participant's id, or any
  // metadata value, not just their display name.
  private searchHay(c: PeerConversation): string {
    const parts: string[] = [c.reference, c.preview];
    for (const p of c.participants) {
      parts.push(p.name, p.id, ...Object.values(p.meta));
    }
    return parts.join(" ").toLowerCase();
  }

  // The conversations shown in the list, after the active role filter + free text.
  get filtered(): PeerConversation[] {
    const q = this.query.trim().toLowerCase();
    return this.conversations.filter((c) => {
      if (this.roleFilter && !this.partyRoles(c).includes(this.roleFilter)) return false;
      if (!q) return true;
      return this.searchHay(c).includes(q);
    });
  }

  // Autocomplete suggestions: one row per distinct role (+ count), and — when
  // typing — matching conversations.
  get roleSuggestions(): { role: string; count: number }[] {
    const q = this.query.trim().toLowerCase();
    return this.roleOrder
      .filter((r) => !q || r.toLowerCase().includes(q))
      .map((r) => ({ role: r, count: this.conversations.filter((c) => this.partyRoles(c).includes(r)).length }));
  }
  get convSuggestions(): { id: string; title: string; reference: string }[] {
    const q = this.query.trim().toLowerCase();
    if (!q) return [];
    return this.conversations
      .filter((c) => this.searchHay(c).includes(q))
      .slice(0, 5)
      .map((c) => ({ id: c.id, title: this.title(c), reference: c.reference }));
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
