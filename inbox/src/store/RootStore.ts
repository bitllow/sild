import { makeAutoObservable, runInAction } from "mobx";
import type { Centrifuge } from "centrifuge";
import { adminApi, type ApiAssignment, type ApiMessage } from "@/api/admin";
import { ApiError } from "@/api/client";
import { createRealtime, type RealtimeEnvelope, type RealtimeState } from "@/api/realtime";
import {
  buildConversation,
  mapApiKey,
  mapRealtimeMessage,
  mapTeamMember,
  mapWebhook,
  relativeTime,
} from "./map";
import type {
  ApiKey,
  Conversation,
  InboxFilter,
  InboxView,
  PlatformRole,
  SessionState,
  SettingsTab,
  TeamMember,
  Webhook,
} from "./types";

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
  panelOpen = true;
  composer = "";
  internal = false;
  loadingConvs = false;
  convError: string | null = null;
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
  settingsLoaded = false;
  keyDialog = false;
  revealedKey: string | null = null;

  // --- realtime (§5) ---
  rtState: RealtimeState = "disconnected";
  typingConvId: string | null = null;
  private rt: Centrifuge | null = null;
  private typingTimer: ReturnType<typeof setTimeout> | null = null;
  private safetyTimer: ReturnType<typeof setInterval> | null = null;

  constructor() {
    makeAutoObservable(this);
  }

  // ─────────────────────────── session ───────────────────────────
  bootstrap = async () => {
    try {
      await this.loadConversations();
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
    });
  };

  // ─────────────────────────── inbox load ───────────────────────────
  loadConversations = async () => {
    runInAction(() => {
      this.loadingConvs = true;
      this.convError = null;
    });
    const assignments = await adminApi.listAssignments();
    // one assignment per conversation (keep the first seen)
    const seen = new Set<string>();
    const unique: ApiAssignment[] = [];
    for (const a of assignments) {
      if (seen.has(a.conversation_id)) continue;
      seen.add(a.conversation_id);
      unique.push(a);
    }
    const built = await Promise.all(
      unique.map(async (a) => {
        const [conv, page] = await Promise.all([
          adminApi.getConversation(a.conversation_id),
          adminApi.listMessages(a.conversation_id),
        ]);
        return buildConversation(conv, page, a);
      })
    );
    runInAction(() => {
      this.convs = built;
      this.loadingConvs = false;
      if (!this.activeId || !built.some((c) => c.id === this.activeId)) {
        this.activeId = built[0]?.id ?? null;
      }
    });
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
        const i = this.convs.findIndex((c) => c.id === id);
        if (i >= 0) this.convs[i] = rebuilt;
      });
    } catch {
      /* transient; next tick retries */
    }
  };

  // ─────────────────────────── realtime (§5) ───────────────────────────
  connectRealtime = () => {
    if (this.rt || typeof window === "undefined") return;
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
  private syncQueue = async () => {
    const before = this.convs.map((c) => c.id).sort().join(",");
    await this.loadConversations().catch(() => {});
    const after = this.convs.map((c) => c.id).sort().join(",");
    if (before !== after) this.reconnectRealtime();
  };

  private handleEvent = (channel: string, env: RealtimeEnvelope) => {
    if (channel.startsWith("agents:")) {
      void this.syncQueue(); // tenant-wide queue change (new/updated request)
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
        if (cid !== this.activeId && msg.dir === "in") conv.unread += 1;
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
    const conv = this.convs.find((c) => c.id === id);
    if (conv) conv.unread = 0;
    void this.refreshActiveMessages();
  };
  setFilter = (f: InboxFilter) => {
    this.filter = f;
  };
  newRequest = () => {
    this.filter = "unassigned";
  };
  togglePanel = () => {
    this.panelOpen = !this.panelOpen;
  };

  get filteredConvs(): Conversation[] {
    return this.convs
      .filter((c) =>
        this.filter === "all"
          ? true
          : this.filter === "unassigned"
            ? c.status === "queued"
            : this.filter === "closed"
              ? c.status === "closed"
              : c.status !== "closed"
      )
      // Default ordering: most recently active first. filter() already returned a
      // fresh array, so sorting in place doesn't touch the observable source.
      .sort((a, b) => b.lastActivity.localeCompare(a.lastActivity));
  }

  get active(): Conversation | null {
    return (
      this.convs.find((c) => c.id === this.activeId) ||
      this.searchResults?.find((c) => c.id === this.activeId) ||
      this.convs[0] ||
      null
    );
  }

  // The list shown in the left column: search results when a query is active,
  // otherwise the filtered assignment queue.
  get listConvs(): Conversation[] {
    return this.searchResults !== null ? this.searchResults : this.filteredConvs;
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
      const { conversations } = await adminApi.search(q);
      const built = await Promise.all(
        conversations.map(async (hit) => {
          const [conv, page] = await Promise.all([
            adminApi.getConversation(hit.conversation_id),
            adminApi.listMessages(hit.conversation_id),
          ]);
          const c = buildConversation(conv, page);
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
    if (!conv || !text || this.sending) return;
    const internal = this.internal;
    this.sending = true;
    try {
      await adminApi.postMessage(conv.id, text, internal ? "internal" : "participants");
      runInAction(() => {
        this.composer = "";
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
      const [keys, webhooks, team] = await Promise.all([
        adminApi.listApiKeys(),
        adminApi.listWebhooks(),
        adminApi.listTeam(),
      ]);
      runInAction(() => {
        this.keys = keys.filter((k) => !k.revoked_at).map(mapApiKey);
        this.webhooks = webhooks.map(mapWebhook);
        this.team = team.map(mapTeamMember);
        this.settingsLoaded = true;
      });
    } catch {
      runInAction(() => {
        this.settingsLoaded = true;
      });
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
}
