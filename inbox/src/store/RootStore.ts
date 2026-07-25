import { makeAutoObservable, runInAction } from "mobx";
import type { Centrifuge } from "centrifuge";
import {
  adminApi,
  type ApiAssignmentStatus,
  type ApiMessage,
  type QueueOrder,
  type ConversationParams,
  type ApiQueuePage,
  type QueueSort,
} from "@/api/admin";
import { ApiError } from "@/api/client";
import { createRealtime, type RealtimeEnvelope, type RealtimeState } from "@/api/realtime";
import { AttachmentQueue } from "./attachments";
import { PeerStore } from "./peer";
import {
  buildConversation,
  buildQueueRow,
  mapApiKey,
  mapEmailChannel,
  mapRealtimeMessage,
  mapTeamMember,
  mapWebhook,
  relativeTime,
} from "./map";
import type {
  ApiKey,
  BrandConfig,
  Brand,
  Conversation,
  EmailChannel,
  InboxFilter,
  InboxView,
  PlatformRole,
  SessionState,
  SettingsTab,
  TeamMember,
  UiStatus,
  Webhook,
} from "./types";

// Queue page size for the inbox list + scroll-loading (§4.3).
const PAGE_SIZE = 30;

// ─────────────────────────── notification sound ───────────────────────────
// A single shared AudioContext, reused across chimes. Browsers start an
// AudioContext suspended until a user gesture (autoplay policy); realtime events
// aren't gestures, so a fresh context per chime would stay silent. We create one
// lazily and resume it on the first user interaction (and on each chime), so
// event-driven chimes actually sound. Module-level so it stays out of MobX.
let audioCtx: AudioContext | null = null;
let audioUnlockBound = false;

function ensureAudioContext(): AudioContext | null {
  if (typeof window === "undefined") return null;
  const Ctx = window.AudioContext || (window as unknown as { webkitAudioContext?: typeof AudioContext }).webkitAudioContext;
  if (!Ctx) return null;
  if (!audioCtx) {
    try {
      audioCtx = new Ctx();
    } catch {
      return null;
    }
  }
  return audioCtx;
}

// Bind this ONCE at startup (not lazily on the first chime): browsers only let an
// AudioContext leave the "suspended" state in response to a user gesture, and a
// realtime message isn't one. Priming on the agent's earlier clicks means the
// context is already running by the time a message arrives and we chime.
function installAudioUnlock(): void {
  if (audioUnlockBound || typeof window === "undefined") return;
  audioUnlockBound = true;
  const unlock = () => void ensureAudioContext()?.resume().catch(() => {});
  window.addEventListener("pointerdown", unlock);
  window.addEventListener("keydown", unlock);
}

// playChime sounds a short ascending two-tone notification on the shared context.
// Tones are scheduled off ctx.currentTime, so they must fire while the context
// is running — if it's suspended, we resume FIRST and play in the callback (a
// suspended context advances no time, so scheduling into it would be silent).
function playChime(): void {
  const ac = ensureAudioContext();
  if (!ac) return;
  const fire = () => {
    const tone = (freq: number, at: number) => {
      const osc = ac.createOscillator();
      const gain = ac.createGain();
      osc.type = "sine";
      osc.frequency.value = freq;
      osc.connect(gain);
      gain.connect(ac.destination);
      const t = ac.currentTime + at;
      gain.gain.setValueAtTime(0.0001, t);
      gain.gain.exponentialRampToValueAtTime(0.18, t + 0.02);
      gain.gain.exponentialRampToValueAtTime(0.0001, t + 0.2);
      osc.start(t);
      osc.stop(t + 0.22);
    };
    try {
      tone(660, 0);
      tone(880, 0.14);
    } catch {
      /* audio unavailable — silent */
    }
  };
  if (ac.state === "suspended") ac.resume().then(fire).catch(() => {});
  else fire();
}

export class RootStore {
  // --- session ---
  session: SessionState = "loading";
  authError: string | null = null;
  authBusy = false;

  // --- navigation ---
  inboxView: InboxView = "inbox";
  settingsTab: SettingsTab = "keys";

  // --- inbox ---
  convs: Conversation[] = [];
  activeId: string | null = null;
  filter: InboxFilter = "all";
  // "Show closed" — closed conversations are hidden from every scope until this
  // is on (the old "Closed" tab, now a toggle next to the open count).
  showClosed = false;
  // Sort the queue by last activity (default), date started, or waiting since,
  // with an asc/desc toggle. Applied server-side (§4.3).
  sortBy: QueueSort = "last_activity";
  sortDir: QueueOrder = "desc";
  // Open-conversation count for the inbox header badge (§8), plus the live
  // scope-tab counters (assigned to me / unassigned) and the closed count for the
  // "Show closed · N" toggle — all tenant-wide, refreshed from the queue endpoint.
  openCount = 0;
  youCount = 0;
  unassignedCount = 0;
  closedCount = 0;
  panelOpen = true;
  // New-message sound: chimes on inbound messages when on; the header toggle
  // mutes it (persisted). Starts muted-safe: read the saved preference on init.
  soundOn = true;
  // Contact drill-down: the active contact's other threads (Details-panel
  // "Earlier from …" list), and — when the agent clicks "View all" — a name +
  // external id that scopes the whole list to that contact via a dismissible chip.
  contactHistory: Conversation[] = [];
  authorFilter: string | null = null;
  // Generation token for contact-history fetches: bumped on each request so a
  // slow response for a previously-active contact can't overwrite a newer one.
  private contactSeq = 0;
  composer = "";
  internal = false;
  // Files queued for the next outgoing message — the shared composer attachment
  // queue (uploads, in-flight count, conversation-switch guard). See AttachmentQueue.
  atts = new AttachmentQueue((msg) => runInAction(() => (this.convError = msg)));
  loadingConvs = false;
  // pagination (cursor-based, scroll-loading)
  nextCursor: string | null = null;
  hasMore = false;
  loadingMore = false;
  convError: string | null = null;
  // Generation token: bumped on every fresh queue load (filter change / reload)
  // so a slow in-flight request for a previous filter can't overwrite newer state.
  private queueSeq = 0;
  sending = false;
  actionBusy = false;

  // --- search (§4.3) ---
  searchQuery = "";
  searching = false;
  searchResults: Conversation[] | null = null;
  private searchTimer: ReturnType<typeof setTimeout> | null = null;
  private searchSeq = 0;

  // --- settings ---
  keys: ApiKey[] = [];
  webhooks: Webhook[] = [];
  team: TeamMember[] = [];
  emailChannel: EmailChannel | null = null;
  channelCopied = false;
  settingsLoaded = false;
  keyDialog = false;
  revealedKey: string | null = null;

  // --- settings: appearance (§8) ---
  // The staged brand set; edits are local until Save. brandBaseline is the last
  // saved snapshot (JSON) — Reset restores it and dirty is any diff from it.
  brands: Brand[] = [];
  activeBrandId = "";
  brandsLoaded = false;
  brandBaseline = "";
  brandSaving = false;
  brandMenuOpen = false;
  brandEditingName = false;
  brandPreview: "home" | "chat" = "home";

  // --- realtime (§5) ---
  rtState: RealtimeState = "disconnected";
  typingConvId: string | null = null;
  private rt: Centrifuge | null = null;
  private typingTimer: ReturnType<typeof setTimeout> | null = null;
  private safetyTimer: ReturnType<typeof setInterval> | null = null;

  // --- signed-in operator + peer conversations ---
  // meId identifies the signed-in agent (Team "You" marker); peerAccess gates the
  // peer-conversations nav. The peer surface lives in its own store, fed the shared
  // realtime connection's events for peer conversation ids.
  meId: string | null = null;
  peerAccess = false;
  peer = new PeerStore(this);

  constructor() {
    makeAutoObservable(this, { peer: false, atts: false });
    if (typeof window !== "undefined") {
      this.soundOn = window.localStorage.getItem("sild_inbox_sound") !== "off";
      installAudioUnlock(); // prime notification audio on the agent's first gesture
    }
  }

  // ─────────────────────────── session ───────────────────────────
  // loadMe resolves the signed-in operator (id + peer access). Best-effort — a
  // failure just leaves the peer nav hidden; it never blocks the inbox.
  loadMe = async () => {
    try {
      const me = await adminApi.me();
      runInAction(() => {
        this.meId = me.subject?.id ?? "";
        // Peer visibility is the scope of conversations.list, not a flag
        // re-interpreted here.
        const list = me.grants.find((g) => g.action === "conversations.list");
        const kinds = list?.scope?.kinds;
        this.peerAccess = !!list && (!kinds || kinds.includes("peer"));
      });
      // Load peer conversations up front (not lazily on first visit) so the nav
      // attention badge is live from session start and realtime peer messages
      // route to the peer store — otherwise, until the surface is opened once,
      // peer.owns() is false and peer arrivals are dropped into a queue refetch.
      if (this.peerAccess) void this.peer.loadConversations();
    } catch {
      /* leave peer surface hidden */
    }
  };

  bootstrap = async () => {
    try {
      await this.loadConversations();
      void this.loadMe();
      runInAction(() => {
        this.session = "authed";
      });
      this.connectRealtime();
    } catch (e) {
      runInAction(() => {
        this.session = e instanceof ApiError && e.isUnauthorized ? "anon" : "anon";
        if (e instanceof ApiError && !e.isUnauthorized) this.convError = e.message;
      });
    }
  };

  loginPassword = async (email: string, password: string) => {
    this.authBusy = true;
    this.authError = null;
    try {
      await adminApi.loginPassword(email, password);
      await this.loadConversations();
      void this.loadMe();
      runInAction(() => {
        this.session = "authed";
        this.authBusy = false;
      });
      this.connectRealtime();
    } catch (e) {
      runInAction(() => {
        this.authBusy = false;
        this.authError =
          e instanceof ApiError && e.isUnauthorized
            ? "Invalid email or password."
            : e instanceof ApiError
              ? e.message
              : "Sign-in failed.";
      });
    }
  };

  loginGoogle = () => {
    window.location.href = adminApi.googleLoginUrl();
  };

  logout = async () => {
    this.dispose();
    try {
      await adminApi.logout();
    } catch {
      /* best-effort */
    }
    runInAction(() => {
      this.session = "anon";
      this.convs = [];
      this.activeId = null;
      this.settingsLoaded = false;
      this.inboxView = "inbox";
      this.meId = null;
      this.peerAccess = false;
      this.peer.reset();
    });
  };

  // ─────────────────────────── inbox load ───────────────────────────
  // Map the active filter to server-side query params. Filtering + sorting +
  // pagination all happen on the backend (§4.3); the list endpoint returns the
  // last message per row, not history.
  // Two state machines, two parameters: `status` is the conversation lifecycle
  // (the inbox's "closed"), `assignment_status` the assignment's. Every scope
  // pins kind=support, or a peer_access operator's All tab would admit peer rows.
  private get queueParams(): ConversationParams {
    const base: ConversationParams = {
      kind: "support",
      sort: this.sortBy,
      order: this.sortDir,
      limit: PAGE_SIZE,
    };
    // Hide closed conversations server-side too whenever the client hides them —
    // otherwise a closed row would consume a page slot and then vanish
    // client-side, shrinking the visible page.
    switch (this.filter) {
      case "unassigned":
        // A closed queued conversation isn't "unassigned" in the UI (its derived
        // status is closed), so it's always excluded here.
        return { ...base, assignmentStatus: "queued", status: "open" };
      case "you":
        // "You" = assigned to me (closing keeps assignee_actor_id set). Show
        // closed reveals my closed threads; otherwise they're excluded.
        return this.showClosed
          ? { ...base, assignee: "me", assignmentStatus: "assigned" }
          : { ...base, assignee: "me", assignmentStatus: "assigned", status: "open" };
      default:
        return this.showClosed ? base : { ...base, status: "open" };
    }
  }

  // Load (or reload) the first page for the current filter. Bumps queueSeq so any
  // earlier in-flight load/loadMore/syncQueue for a previous filter is discarded.
  loadConversations = async () => {
    const seq = ++this.queueSeq;
    runInAction(() => {
      this.loadingConvs = true;
      this.convError = null;
    });
    try {
      const page = await adminApi.listConversations(this.queueParams);
      if (seq !== this.queueSeq) return; // a newer load superseded this one
      const built = page.items.map(buildQueueRow);
      runInAction(() => {
        this.convs = built;
        this.nextCursor = page.next_cursor;
        this.hasMore = page.has_more;
        this.applyCounts(page);
        if (!this.activeId || !built.some((c) => c.id === this.activeId)) {
          this.activeId = built[0]?.id ?? null;
        }
      });
      // Queue rows carry no history (messages: []); load the auto-selected
      // conversation's thread now rather than waiting for the websocket — REST
      // works even when realtime doesn't.
      if (this.activeId) {
        await this.refreshActiveMessages();
        void this.refreshContactHistory();
      }
    } finally {
      if (seq === this.queueSeq) {
        runInAction(() => {
          this.loadingConvs = false;
        });
      }
    }
  };

  // Append the next page (scroll-loading). No-op while a load is in flight, when
  // exhausted, or while a search is active (search results aren't paginated here).
  loadMore = async () => {
    if (this.loadingMore || !this.hasMore || !this.nextCursor || this.searchResults !== null) return;
    const seq = this.queueSeq; // page belongs to the current filter generation
    runInAction(() => {
      this.loadingMore = true;
    });
    try {
      const page = await adminApi.listConversations({ ...this.queueParams, cursor: this.nextCursor });
      if (seq !== this.queueSeq) return; // filter changed mid-flight — drop this page
      runInAction(() => {
        const have = new Set(this.convs.map((c) => c.id));
        for (const it of page.items) {
          if (!have.has(it.id)) this.convs.push(buildQueueRow(it));
        }
        this.nextCursor = page.next_cursor;
        this.hasMore = page.has_more;
      });
    } catch {
      /* transient; user can scroll again to retry */
    } finally {
      if (seq === this.queueSeq) {
        runInAction(() => {
          this.loadingMore = false;
        });
      }
    }
  };

  private refreshActiveMessages = async () => {
    const id = this.activeId;
    if (!id) return;
    try {
      const [conv, page] = await Promise.all([
        adminApi.getConversation(id),
        adminApi.listMessages(id),
      ]);
      const rebuilt = buildConversation(conv, page);
      runInAction(() => {
        // The open thread may live in the queue, in search results, or in the
        // contact-history list — refresh it wherever it is.
        for (const list of [this.convs, this.searchResults, this.contactHistory]) {
          if (!list) continue;
          const i = list.findIndex((c) => c.id === id);
          if (i >= 0) list[i] = rebuilt;
        }
      });
    } catch {
      /* transient; next tick retries */
    }
  };

  // ─────────────────────────── realtime (§5) ───────────────────────────
  connectRealtime = () => {
    if (this.rt || typeof window === "undefined") return;
    // We arrive here right after a login/bootstrap gesture — a good moment to
    // create + resume the notification AudioContext so it's already running when
    // the first realtime chime fires (autoplay policy allows resume post-gesture).
    void ensureAudioContext()?.resume().catch(() => {});
    this.rt = createRealtime({
      onState: (s) => {
        runInAction(() => {
          this.rtState = s;
        });
        // reconnect catch-up is the only correctness mechanism (§5.4)
        if (s === "connected") {
          void this.refreshActiveMessages();
          void this.syncQueue();
        }
      },
      onEvent: (channel, env) => this.handleEvent(channel, env),
    });
    this.rt.connect();
    // safety reconcile — the socket guarantees nothing (§5.4)
    if (!this.safetyTimer) {
      this.safetyTimer = setInterval(() => void this.refreshActiveMessages(), 30000);
    }
  };

  private reconnectRealtime = () => {
    if (!this.rt) return;
    try {
      this.rt.disconnect();
      this.rt.connect();
    } catch {
      /* ignore */
    }
  };

  dispose = () => {
    if (this.safetyTimer) {
      clearInterval(this.safetyTimer);
      this.safetyTimer = null;
    }
    if (this.typingTimer) {
      clearTimeout(this.typingTimer);
      this.typingTimer = null;
    }
    if (this.rt) {
      try {
        this.rt.disconnect();
      } catch {
        /* ignore */
      }
      this.rt = null;
    }
    runInAction(() => {
      this.rtState = "disconnected";
    });
  };

  // Reload the queue; if the conversation set changed, resubscribe realtime so a
  // brand-new support request gets its live conv channel (§5.2 — agents aren't
  // members, so new conversations need a fresh server-side subscription set).
  // Merge the first page into the loaded list on a tenant-wide queue change —
  // updating existing rows in place and prepending genuinely new ones — WITHOUT
  // dropping already scroll-loaded pages or resetting the cursor. If a new
  // conversation appears, resubscribe so its realtime channel is covered (§5.2).
  private syncQueue = async () => {
    const seq = this.queueSeq; // merge belongs to the current filter generation
    try {
      const page = await adminApi.listConversations(this.queueParams);
      if (seq !== this.queueSeq) return; // filter changed mid-flight — drop the merge
      runInAction(() => {
        this.applyCounts(page);
        const byId = new Map(this.convs.map((c) => [c.id, c]));
        let added = false;
        for (const it of page.items) {
          const existing = byId.get(it.id);
          const fresh = buildQueueRow(it);
          if (existing) {
            // refresh lightweight row fields; keep any loaded history + members
            existing.preview = fresh.preview;
            existing.time = fresh.time;
            existing.lastActivity = fresh.lastActivity;
            existing.status = fresh.status;
            existing.assignmentId = fresh.assignmentId;
            existing.assignmentStatus = fresh.assignmentStatus;
            existing.convClosed = fresh.convClosed;
          } else {
            this.convs.push(fresh);
            added = true;
          }
        }
        if (!this.activeId && this.convs.length) this.activeId = this.convs[0].id;
        if (added) {
          this.reconnectRealtime();
          this.chime(); // a new request landed in the queue
        }
      });
    } catch {
      /* transient; the next event or the safety reconcile retries */
    }
  };

  private handleEvent = (channel: string, env: RealtimeEnvelope) => {
    if (channel.startsWith("agents:")) {
      void this.syncQueue(); // tenant-wide queue change (new/updated request)
      return;
    }
    // The tenant peer channel (peer:<tenant>) carries the whole peer surface for
    // peer-access operators: a member.added for a not-yet-loaded conversation is
    // the "new peer conversation" nudge; everything else is an event in a peer
    // conversation the peer store handles.
    if (channel.startsWith("peer:")) {
      if (env.conversation_id && this.peer.owns(env.conversation_id)) {
        this.peer.onRealtime(env);
      } else {
        this.peer.onTenantNudge(env.conversation_id); // new/unloaded peer conversation — surface just it
      }
      return;
    }
    // A peer conversation the peer store already owns may also see conv:<id>
    // events (e.g. legacy subscriptions) — route them to the peer store.
    if (env.conversation_id && this.peer.owns(env.conversation_id)) {
      this.peer.onRealtime(env);
      return;
    }
    switch (env.type) {
      case "message.created":
        this.onMessageCreated(env);
        break;
      case "assignment.updated":
        this.onAssignmentUpdated(env);
        break;
      case "conversation.closed":
        this.onConversationClosed(env);
        break;
      case "typing":
        this.onTyping(env);
        break;
      default:
        break;
    }
  };

  private onMessageCreated = (env: RealtimeEnvelope) => {
    const cid = env.conversation_id;
    if (!cid) return;
    const conv = this.convs.find((c) => c.id === cid);
    if (!conv) {
      void this.syncQueue();
      // Might be a peer conversation the peer store doesn't have loaded (beyond the
      // first page, or while a search replaced the list) — peer-access agents are
      // subscribed to ALL peer channels, so surface that one row rather than
      // dropping the message.
      if (this.peerAccess) this.peer.onTenantNudge(cid);
      return;
    }
    const m = env.data as ApiMessage;
    if (conv.messages.some((x) => x.id === m.id)) return; // dedupe own/echoed sends
    const msg = mapRealtimeMessage(m, conv);
    runInAction(() => {
      conv.messages.push(msg);
      if (!msg.internal && !msg.system) {
        conv.preview = msg.body;
        conv.time = relativeTime(m.created_at);
        conv.lastActivity = m.created_at; // bumps it to the top of the desc-sorted list
        if (msg.dir === "in") {
          // Sound on any inbound message — a reply in the open thread as well as
          // one in a background conversation; only background ones bump unread.
          if (cid !== this.activeId) conv.unread += 1;
          this.chime();
        }
      }
    });
  };

  private onAssignmentUpdated = (env: RealtimeEnvelope) => {
    const cid = env.conversation_id;
    const conv = cid ? this.convs.find((c) => c.id === cid) : null;
    if (!conv) {
      void this.syncQueue();
      return;
    }
    const d = env.data as { status?: string };
    if (d.status === "queued" || d.status === "assigned" || d.status === "closed") {
      runInAction(() => {
        conv.assignmentStatus = d.status as typeof conv.assignmentStatus;
        if (!conv.convClosed) conv.status = d.status as typeof conv.status;
      });
    }
  };

  private onConversationClosed = (env: RealtimeEnvelope) => {
    const conv = env.conversation_id ? this.convs.find((c) => c.id === env.conversation_id) : null;
    if (!conv) return;
    runInAction(() => {
      conv.convClosed = true;
      conv.status = "closed";
      conv.preview = "Conversation closed";
    });
  };

  private onTyping = (env: RealtimeEnvelope) => {
    if (env.conversation_id !== this.activeId) return;
    runInAction(() => {
      this.typingConvId = env.conversation_id ?? null;
    });
    if (this.typingTimer) clearTimeout(this.typingTimer);
    this.typingTimer = setTimeout(
      () => runInAction(() => {
        this.typingConvId = null;
      }),
      3500
    );
  };

  // ─────────────────────────── navigation ───────────────────────────
  goInbox = () => {
    this.inboxView = "inbox";
  };
  goPeer = () => {
    if (!this.peerAccess) return; // gated per-user (Settings → Team)
    this.inboxView = "peer";
    if (!this.peer.loaded) void this.peer.loadConversations();
  };
  goSettings = () => {
    this.inboxView = "settings";
    if (!this.settingsLoaded) void this.loadSettings();
  };
  setSettingsTab = (t: SettingsTab) => {
    this.settingsTab = t;
  };

  setActive = (id: string) => {
    this.activeId = id;
    this.composer = "";
    this.internal = false;
    this.atts.reset(); // abandon the previous conversation's uploads + in-flight ones
    const conv = this.convs.find((c) => c.id === id) || this.contactHistory.find((c) => c.id === id);
    if (conv) conv.unread = 0;
    void this.refreshActiveMessages();
    void this.refreshContactHistory();
  };
  setFilter = (f: InboxFilter) => {
    if (this.filter === f) return;
    this.filter = f;
    // filter is applied server-side → reset pagination and reload page 1.
    this.nextCursor = null;
    this.hasMore = false;
    void this.loadConversations();
  };
  setSort = (s: QueueSort) => {
    if (this.sortBy === s) return;
    this.sortBy = s;
    this.reloadQueue();
  };
  toggleSortDir = () => {
    this.sortDir = this.sortDir === "desc" ? "asc" : "desc";
    this.reloadQueue();
  };
  // Sorting is applied server-side → reset pagination and reload page 1.
  private reloadQueue = () => {
    this.nextCursor = null;
    this.hasMore = false;
    void this.loadConversations();
  };
  // "Show closed" changes the server-side scope (§4.3) → reload page 1.
  toggleClosed = () => {
    this.showClosed = !this.showClosed;
    this.reloadQueue();
  };
  newRequest = () => {
    this.setFilter("unassigned");
  };
  togglePanel = () => {
    this.panelOpen = !this.panelOpen;
  };

  // Apply the tenant-wide badge counts from a queue page response.
  private applyCounts = (page: ApiQueuePage) => {
    if (!page.counts) return; // emitted only for the support queue
    this.openCount = page.counts.open;
    this.youCount = page.counts.you;
    this.unassignedCount = page.counts.unassigned;
    this.closedCount = page.counts.closed;
  };

  // Conversations with unread inbound messages — drives the coral attention badge
  // on the nav-rail inbox icon.
  get attentionCount(): number {
    return this.convs.filter((c) => c.unread > 0).length;
  }

  // ─────────────────────────── sound (§8) ───────────────────────────
  toggleSound = () => {
    this.soundOn = !this.soundOn;
    if (typeof window !== "undefined") {
      window.localStorage.setItem("sild_inbox_sound", this.soundOn ? "on" : "off");
    }
    if (this.soundOn) this.chime(); // confirm audibly on unmute
  };

  // chime plays the two-tone notification when unmuted (no-op otherwise). The
  // Web Audio plumbing + autoplay unlock live at module scope (see playChime).
  // chime plays the reply-notification sound (honors the mute toggle). Public so
  // the peer store (PeerRoot) can chime on inbound peer messages too.
  chime = () => {
    if (this.soundOn) playChime();
  };

  // ─────────────────────────── contact history (§4.3) ───────────────────────
  // The active conversation's client contact (external id + display name), used
  // to fetch that contact's other threads.
  get activeContact(): { extId: string; name: string } | null {
    const a = this.active;
    if (!a) return null;
    const client = a.members.find((m) => m.extId);
    if (!client?.extId) return null;
    return { extId: client.extId, name: client.name || a.name };
  }

  // The contact's OTHER threads (excludes the one currently open) — the Details
  // panel's "Earlier from …" list.
  get contactEarlier(): Conversation[] {
    return this.contactHistory.filter((c) => c.id !== this.activeId);
  }

  // Load every thread for the active conversation's contact. Cheap (denormalized
  // previews, no history) and keyed to the active contact so the "Earlier from"
  // list and the "View all" drill-down share one fetch.
  private refreshContactHistory = async () => {
    const seq = ++this.contactSeq;
    const contact = this.activeContact;
    if (!contact) {
      runInAction(() => {
        if (seq === this.contactSeq) this.contactHistory = [];
      });
      return;
    }
    try {
      const items = await adminApi.listAllConversations({ participant: contact.extId });
      const built = items.map(buildQueueRow);
      runInAction(() => {
        // Drop a response that a newer active-contact switch has superseded.
        if (seq === this.contactSeq) this.contactHistory = built;
      });
    } catch {
      /* transient */
    }
  };

  // "View all" — scope the whole list to the active contact (dismissible chip).
  viewAllFromActive = () => {
    const contact = this.activeContact;
    if (!contact) return;
    this.authorFilter = contact.name;
    this.searchQuery = "";
    this.searchResults = null;
  };
  clearAuthorFilter = () => {
    this.authorFilter = null;
  };
  get authorCount(): number {
    return this.contactHistory.length;
  }

  get filteredConvs(): Conversation[] {
    // The server already scopes the list to the filter; this client filter only
    // mirrors status-based views so realtime transitions (e.g. a claimed request
    // leaving "Unassigned") drop out immediately without a refetch.
    const wantStatus: UiStatus | null =
      this.filter === "unassigned" ? "queued" : this.filter === "you" ? "assigned" : null;
    const rows = this.convs.filter((c) => {
      // "Show closed" off hides closed rows from every scope.
      if (!this.showClosed && c.status === "closed") return false;
      // "You" with closed shown includes my closed threads alongside assigned.
      if (this.filter === "you") return c.status === "assigned" || (this.showClosed && c.status === "closed");
      return !wantStatus || c.status === wantStatus;
    });
    // Re-sort by the active key (carried per row) so realtime arrivals and
    // last-activity bumps land in the right place without a refetch — matching
    // the server's ordering for whichever sort is active.
    const key = (c: Conversation) =>
      this.sortBy === "created" ? c.dateStarted : this.sortBy === "waiting_since" ? c.waitingSince : c.lastActivity;
    rows.sort((a, b) => {
      const asc = key(a).localeCompare(key(b));
      return this.sortDir === "desc" ? -asc : asc; // desc = newest first (default)
    });
    return rows;
  }

  get active(): Conversation | null {
    return (
      this.convs.find((c) => c.id === this.activeId) ||
      this.searchResults?.find((c) => c.id === this.activeId) ||
      this.contactHistory.find((c) => c.id === this.activeId) ||
      this.convs[0] ||
      null
    );
  }

  // The list shown in the left column, in priority order: search results while a
  // query is active; else the contact drill-down when "View all from" is on; else
  // the filtered assignment queue.
  get listConvs(): Conversation[] {
    if (this.searchResults !== null) return this.searchResults;
    if (this.authorFilter) return this.contactHistory;
    return this.filteredConvs;
  }

  // ─────────────────────────── search (§4.3) ───────────────────────────
  setSearchQuery = (q: string) => {
    this.searchQuery = q;
    if (this.searchTimer) clearTimeout(this.searchTimer);
    if (!q.trim()) {
      this.searchResults = null;
      this.searching = false;
      return;
    }
    this.searchTimer = setTimeout(() => void this.runSearch(q), 280);
  };

  private runSearch = async (q: string) => {
    const seq = ++this.searchSeq;
    runInAction(() => {
      this.searching = true;
    });
    try {
      const { items } = await adminApi.listConversations({ kind: "support", q });
      const built = await Promise.all(
        items.map(async (hit) => {
          // Rows arrive whole from the list, so only the thread is fetched.
          const page = await adminApi.listMessages(hit.id);
          const c = buildConversation(hit, page);
          if (hit.snippet) c.preview = hit.snippet;
          return c;
        })
      );
      if (seq !== this.searchSeq) return; // stale response
      runInAction(() => {
        this.searchResults = built;
        this.searching = false;
      });
    } catch {
      if (seq !== this.searchSeq) return;
      runInAction(() => {
        this.searchResults = [];
        this.searching = false;
      });
    }
  };

  get assignLabel(): string {
    const s = this.active?.status;
    return s === "queued" ? "Unclaimed" : s === "closed" ? "Closed" : "Assigned";
  }

  get activeChannelTag(): string {
    return this.active?.channel === "email" ? "channel:email" : "channel:app";
  }

  // ─────────────────────────── composer / lifecycle ───────────────────────────
  setComposer = (v: string) => {
    this.composer = v;
  };
  setInternal = (v: boolean) => {
    this.internal = v;
  };

  sendMessage = async () => {
    const conv = this.active;
    const text = this.composer.trim();
    if (!conv || (!text && this.atts.pending.length === 0) || this.sending || this.atts.isUploading) return;
    const internal = this.internal;
    this.sending = true;
    const refs = this.atts.refs();
    try {
      await adminApi.postMessage(conv.id, text, internal ? "internal" : "participants", refs);
      runInAction(() => {
        this.composer = "";
        this.atts.clear();
      });
      await this.refreshActiveMessages();
    } catch (e) {
      runInAction(() => {
        this.convError = e instanceof ApiError ? e.message : "Failed to send.";
      });
    } finally {
      runInAction(() => {
        this.sending = false;
      });
    }
  };

  claim = async () => {
    const conv = this.active;
    if (!conv?.assignmentId || this.actionBusy) return;
    this.actionBusy = true;
    try {
      await adminApi.claimAssignment(conv.assignmentId);
      runInAction(() => {
        const c = this.convs.find((x) => x.id === conv.id);
        if (c) {
          c.assignmentStatus = "assigned";
          if (!c.convClosed) c.status = "assigned";
        }
      });
      void this.syncQueue(); // scope moved (unassigned → you): refresh the counters
    } catch (e) {
      runInAction(() => {
        this.convError = e instanceof ApiError ? e.message : "Claim failed.";
      });
    } finally {
      runInAction(() => {
        this.actionBusy = false;
      });
    }
  };

  closeConv = async () => {
    const conv = this.active;
    if (!conv || this.actionBusy) return;
    this.actionBusy = true;
    try {
      await adminApi.closeConversation(conv.id);
      runInAction(() => {
        const c = this.convs.find((x) => x.id === conv.id);
        if (c) {
          c.convClosed = true;
          c.status = "closed";
          c.preview = "Conversation closed";
        }
      });
      void this.syncQueue(); // it left the open scope + bumped closed: refresh counters
    } catch (e) {
      runInAction(() => {
        this.convError = e instanceof ApiError ? e.message : "Close failed.";
      });
    } finally {
      runInAction(() => {
        this.actionBusy = false;
      });
    }
  };

  // ─────────────────────────── settings ───────────────────────────
  loadSettings = async () => {
    try {
      const [keys, webhooks, team, email, brands] = await Promise.all([
        adminApi.listApiKeys(),
        adminApi.listWebhooks(),
        adminApi.listTeam(),
        adminApi.getEmailChannel(),
        adminApi.getBrands(),
      ]);
      runInAction(() => {
        this.keys = keys.filter((k) => !k.revoked_at).map(mapApiKey);
        this.webhooks = webhooks.map(mapWebhook);
        this.team = team.map(mapTeamMember);
        this.emailChannel = mapEmailChannel(email);
        this.applyBrands(brands.brands, brands.active_brand_id);
        this.settingsLoaded = true;
      });
    } catch {
      runInAction(() => {
        this.settingsLoaded = true;
      });
    }
  };

  // ─────────────────────────── settings: appearance (§8) ───────────────────
  private applyBrands = (brands: { id: string; name: string; config: BrandConfig }[], activeId: string) => {
    this.brands = brands.map((b) => ({ id: b.id, name: b.name, config: { ...b.config } }));
    this.activeBrandId = activeId || this.brands[0]?.id || "";
    this.brandBaseline = this.brandSnapshot();
    this.brandsLoaded = true;
  };

  // Snapshot of everything Save persists (the brand set + which is active), used
  // for dirty tracking and Reset. Resolved asset URLs (logoUrl/iconImgUrl) are
  // excluded: they're server-signed and rotate on each read, so including them
  // would flag spurious "unsaved changes".
  private brandSnapshot = (): string =>
    JSON.stringify({
      b: this.brands.map((x) => ({ id: x.id, name: x.name, config: this.persistedConfig(x.config) })),
      a: this.activeBrandId,
    });

  private persistedConfig = (c: BrandConfig): BrandConfig => {
    const { logoUrl: _l, iconImgUrl: _i, ...rest } = c;
    return rest as BrandConfig;
  };

  get brandDirty(): boolean {
    return this.brandsLoaded && this.brandBaseline !== this.brandSnapshot();
  }

  get activeBrand(): Brand | null {
    return this.brands.find((b) => b.id === this.activeBrandId) ?? this.brands[0] ?? null;
  }

  selectBrand = (id: string) => {
    this.activeBrandId = id;
    this.brandMenuOpen = false;
  };
  toggleBrandMenu = () => {
    this.brandMenuOpen = !this.brandMenuOpen;
  };
  closeBrandMenu = () => {
    this.brandMenuOpen = false;
  };
  startBrandRename = () => {
    this.brandEditingName = true;
    this.brandMenuOpen = false;
  };
  stopBrandRename = () => {
    this.brandEditingName = false;
  };
  setBrandPreview = (v: "home" | "chat") => {
    this.brandPreview = v;
  };

  // Edit the active brand's config (any control) — mutates in place; dirty falls
  // out of the snapshot diff.
  patchBrand = (patch: Partial<BrandConfig>) => {
    const b = this.activeBrand;
    if (b) Object.assign(b.config, patch);
  };
  renameBrand = (name: string) => {
    const b = this.activeBrand;
    if (b) b.name = name;
  };

  // Upload a logo/icon to the bucket via the shared signed-PUT flow (same as
  // message attachments) and return the object key to store on the config —
  // assets live in storage, not as base64 in the brand row.
  uploadBrandAsset = async (file: File): Promise<string> => {
    const mime = file.type || "application/octet-stream";
    const grant = await adminApi.issueUpload(mime, file.size, file.name);
    // Local backend returns an absolute public-origin URL; PUT to its relative
    // /v1 path so it goes same-origin through the Next proxy. Cloud signed URLs
    // (no local route) are used as-is.
    const marker = "/v1/uploads/local/";
    const at = grant.upload_url.indexOf(marker);
    const putUrl = at >= 0 ? grant.upload_url.slice(at) : grant.upload_url;
    const res = await fetch(putUrl, {
      method: "PUT",
      body: file,
      headers: { "Content-Type": mime },
      credentials: at >= 0 ? "include" : "omit",
    });
    if (!res.ok) throw new Error("upload failed");
    return grant.object_key;
  };

  // New brand clones the active config (minus logo) and opens rename immediately.
  // The temp id (no br_ prefix) tells the backend to mint a real id on save.
  newBrand = () => {
    const base = this.activeBrand;
    const config: BrandConfig = base ? { ...base.config, logo: "" } : ({} as BrandConfig);
    const id = "new_" + Math.random().toString(36).slice(2, 10);
    this.brands.push({ id, name: "New brand", config });
    this.activeBrandId = id;
    this.brandMenuOpen = false;
    this.brandEditingName = true;
  };

  resetBrands = () => {
    const parsed = JSON.parse(this.brandBaseline) as { b: Brand[]; a: string };
    this.brands = parsed.b.map((b) => ({ id: b.id, name: b.name, config: { ...b.config } }));
    this.activeBrandId = parsed.a;
    this.brandMenuOpen = false;
    this.brandEditingName = false;
  };

  saveBrands = async () => {
    if (this.brandSaving || !this.brandDirty) return;
    this.brandSaving = true;
    try {
      const res = await adminApi.saveBrands(
        this.brands.map((b) => ({ id: b.id, name: b.name, config: this.persistedConfig(b.config) })),
        this.activeBrandId
      );
      runInAction(() => {
        this.applyBrands(res.brands, res.active_brand_id);
        this.brandEditingName = false;
      });
    } catch (e) {
      runInAction(() => {
        this.convError = e instanceof ApiError ? e.message : "Could not save appearance.";
      });
    } finally {
      runInAction(() => {
        this.brandSaving = false;
      });
    }
  };

  // ─────────────────────────── settings: channels (§6.2) ───────────────────
  copyForwardingAddress = () => {
    if (!this.emailChannel) return;
    try {
      void navigator.clipboard.writeText(this.emailChannel.forwardingAddress);
      runInAction(() => {
        this.channelCopied = true;
      });
      setTimeout(() => runInAction(() => {
        this.channelCopied = false;
      }), 1500);
    } catch {
      /* clipboard unavailable */
    }
  };
  toggleAutoReply = (v: boolean) => this.patchChannel({ autoReply: v }, { auto_reply: v });
  toggleSpamFilter = (v: boolean) => this.patchChannel({ spamFilter: v }, { spam_filter: v });

  // Optimistically apply a toggle, rolling back if the PATCH fails.
  private patchChannel = async (
    local: Partial<EmailChannel>,
    patch: { auto_reply?: boolean; spam_filter?: boolean }
  ) => {
    const ch = this.emailChannel;
    if (!ch) return;
    const prev = { ...ch };
    runInAction(() => Object.assign(ch, local));
    try {
      await adminApi.updateEmailChannel(patch);
    } catch {
      runInAction(() => Object.assign(ch, prev));
    }
  };

  openKeyDialog = async () => {
    try {
      const created = await adminApi.createApiKey("Server key");
      runInAction(() => {
        this.revealedKey = created.key;
        this.keyDialog = true;
      });
      await this.reloadKeys();
    } catch (e) {
      runInAction(() => {
        this.convError = e instanceof ApiError ? e.message : "Could not create key.";
      });
    }
  };
  closeKeyDialog = () => {
    this.keyDialog = false;
    this.revealedKey = null;
  };
  copyKey = () => {
    try {
      if (this.revealedKey) navigator.clipboard.writeText(this.revealedKey);
    } catch {
      /* clipboard unavailable */
    }
  };
  private reloadKeys = async () => {
    const keys = await adminApi.listApiKeys();
    runInAction(() => {
      this.keys = keys.filter((k) => !k.revoked_at).map(mapApiKey);
    });
  };
  revokeKey = async (id: string) => {
    try {
      await adminApi.revokeApiKey(id);
      runInAction(() => {
        this.keys = this.keys.filter((k) => k.id !== id);
      });
    } catch {
      /* leave the key in place on failure */
    }
  };

  toggleWebhook = async (id: string, v: boolean) => {
    const prev = this.webhooks.find((w) => w.id === id)?.active;
    runInAction(() => {
      const w = this.webhooks.find((x) => x.id === id);
      if (w) w.active = v;
    });
    try {
      await adminApi.setWebhookActive(id, v);
    } catch {
      runInAction(() => {
        const w = this.webhooks.find((x) => x.id === id);
        if (w && prev !== undefined) w.active = prev;
      });
    }
  };
  deleteWebhook = async (id: string) => {
    try {
      await adminApi.deleteWebhook(id);
      runInAction(() => {
        this.webhooks = this.webhooks.filter((w) => w.id !== id);
      });
    } catch {
      /* keep the row on failure */
    }
  };

  setRole = async (id: string, role: PlatformRole) => {
    const prev = this.team.find((t) => t.id === id)?.role;
    runInAction(() => {
      const t = this.team.find((x) => x.id === id);
      if (t) t.role = role;
    });
    try {
      await adminApi.setTeamRole(id, role);
    } catch {
      runInAction(() => {
        const t = this.team.find((x) => x.id === id);
        if (t && prev) t.role = prev;
      });
    }
  };

  // setPeerAccess toggles an operator's peer-conversation access (Settings → Team,
  // per-user). Flipping your own off live-hides the peer nav and bounces you back
  // to the inbox if you're viewing it.
  setPeerAccess = async (id: string, value: boolean) => {
    const prev = this.team.find((t) => t.id === id)?.peerAccess;
    runInAction(() => {
      const t = this.team.find((x) => x.id === id);
      if (t) t.peerAccess = value;
      if (id === this.meId) {
        this.peerAccess = value;
        if (!value && this.inboxView === "peer") this.inboxView = "inbox";
      }
    });
    try {
      await adminApi.setTeamPeerAccess(id, value);
    } catch {
      runInAction(() => {
        const t = this.team.find((x) => x.id === id);
        if (t && prev !== undefined) t.peerAccess = prev;
        if (id === this.meId && prev !== undefined) this.peerAccess = prev;
      });
    }
  };
}
