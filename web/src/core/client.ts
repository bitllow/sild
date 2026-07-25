import { Centrifuge } from "centrifuge";
import type {
  BrandResponse,
  ConnectionState,
  PendingAttachment,
  SildConfig,
  WidgetConversation,
  WidgetMessage,
  WidgetState,
} from "./types";

// WidgetClient is the surface the <App> renders against — implemented by the
// live SildClient and the no-network PreviewClient, so the same component drives
// production and the Appearance preview.
export interface WidgetClient {
  state: WidgetState;
  subscribe(fn: () => void): () => void;
  start(conversationId?: string): void | Promise<void>;
  openConversation(id: string): void | Promise<void>;
  openSupportRequest(): void | Promise<string>;
  send(text: string, attachments?: PendingAttachment[]): void | Promise<void>;
  backToList(): void;
  upload(file: File): Promise<PendingAttachment>;
  /** Toggle the reply-notification sound (shared across the home + thread headers). */
  toggleSound(): void;
  /** Force the realtime socket to reconnect so the server re-derives this user's
   *  channel subscriptions — needed after opening a conversation created after the
   *  socket connected (its conv:<id> channel isn't in the current subscription set). */
  reconnect(): void;
}

interface ApiAttachment {
  object_key: string;
  disposition: "inline" | "attachment";
  mime_type: string;
  size_bytes: number;
  filename: string;
  url?: string;
}

interface ApiMessage {
  id: string;
  conversation_id: string;
  sender_kind: "user" | "agent" | "bot" | "system";
  visibility: "participants" | "internal";
  body: string;
  created_at: string;
  external_user_id?: string;
  internal_actor_id?: string;
  /** The agent's display name for agent-authored messages (from the server). */
  author_name?: string;
  attachments?: ApiAttachment[];
}

// Minimal member shape from GET /me/conversations (§4.2 conversation view).
interface ApiConvMember {
  member_kind: "user" | "agent" | "bot" | "email";
  conv_role: string;
  external_user_id?: string;
  internal_actor_id?: string;
  metadata?: Record<string, unknown> | null;
}

interface Envelope {
  type: string;
  conversation_id?: string;
  data: unknown;
}

function clock(iso: string): string {
  const d = new Date(iso);
  if (isNaN(d.getTime())) return "";
  let h = d.getHours();
  const m = String(d.getMinutes()).padStart(2, "0");
  const ap = h >= 12 ? "PM" : "AM";
  h = h % 12 || 12;
  return `${h}:${m} ${ap}`;
}

function uuid(): string {
  try {
    return crypto.randomUUID();
  } catch {
    return "m_" + Math.random().toString(36).slice(2) + Date.now().toString(36);
  }
}

// A single shared AudioContext, reused across chimes and resumed on the first
// user gesture — browsers start it suspended (autoplay policy), and realtime
// events aren't gestures, so a fresh per-chime context would stay silent.
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

// Bind once at startup: an AudioContext only resumes on a user gesture, and an
// incoming reply isn't one — so prime it on the visitor's earlier interactions
// (opening the launcher, typing) and it's running by the time a reply chimes.
function installAudioUnlock(): void {
  if (audioUnlockBound || typeof window === "undefined") return;
  audioUnlockBound = true;
  const unlock = () => void ensureAudioContext()?.resume().catch(() => {});
  window.addEventListener("pointerdown", unlock);
  window.addEventListener("keydown", unlock);
}

// chime plays a short ascending two-tone notification. Tones schedule off
// ctx.currentTime, so resume a suspended context FIRST and play in the callback
// (a suspended context advances no time → scheduling into it is silent). No-op
// where audio is unavailable (SSR, no Web Audio).
function chime(): void {
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

// The most recent agent's display name in a thread, for the header + bubbles.
function agentNameOf(msgs: WidgetMessage[]): string | undefined {
  for (let i = msgs.length - 1; i >= 0; i--) {
    if (msgs[i].direction === "in" && !msgs[i].system && msgs[i].author) return msgs[i].author;
  }
  return undefined;
}

function initialSoundOn(): boolean {
  if (typeof window === "undefined") return true;
  try {
    return window.localStorage.getItem("sild_widget_sound") !== "off";
  } catch {
    return true;
  }
}

// mapMessage renders an API message for the thread. Direction is by author:
//  - system → system line
//  - agent/bot → incoming (author = the agent's name)
//  - user → the visitor's OWN message is outgoing; any OTHER user (the driver in
//    a peer chat) is incoming, its author resolved from the conversation members.
// selfId distinguishes own vs other user messages (peer conversations have two
// user parties); without it, every user message is treated as the visitor's own
// (the support-only case, where the visitor is the only user).
function mapMessage(m: ApiMessage, selfId?: string, names?: Record<string, string>): WidgetMessage {
  const system = m.sender_kind === "system";
  const isAgent = m.sender_kind === "agent" || m.sender_kind === "bot" || !!m.internal_actor_id;
  const mine = m.sender_kind === "user" && (!selfId || m.external_user_id === selfId);
  let author: string | undefined;
  if (!system && !mine) {
    author = isAgent
      ? m.author_name || "Support"
      : (m.external_user_id && names?.[m.external_user_id]) || m.author_name || m.external_user_id || "User";
  }
  return {
    id: m.id,
    direction: mine ? "out" : "in",
    system,
    author,
    time: clock(m.created_at),
    body: m.body,
    attachments: (m.attachments || []).map((a) => ({
      url: a.url,
      disposition: a.disposition,
      mimeType: a.mime_type,
      filename: a.filename,
      sizeBytes: a.size_bytes,
    })),
  };
}

/** Framework-agnostic Sild client (§4.2 REST + §5 realtime over SSE). The widget
 *  and a future @sild/react both render this; it owns no DOM. */
export class SildClient implements WidgetClient {
  private base: string;
  private tokenProvider: () => Promise<string> | string;
  private metadata: Record<string, unknown>;
  private selfId?: string;
  private activeNames: Record<string, string> = {};
  private token: string | null = null;
  private cf: Centrifuge | null = null;
  private listeners = new Set<() => void>();

  state: WidgetState = {
    ready: false,
    error: null,
    connection: "idle",
    conversations: [],
    activeId: null,
    messages: [],
    loadingThread: false,
    soundOn: initialSoundOn(),
  };

  constructor(cfg: SildConfig) {
    this.base = (cfg.baseUrl || "").replace(/\/$/, "");
    this.tokenProvider = cfg.tokenProvider;
    this.metadata = cfg.metadata || {};
    this.selfId = cfg.userId;
    installAudioUnlock(); // prime reply-notification audio on the first gesture
  }

  toggleSound() {
    const soundOn = !this.state.soundOn;
    try {
      window.localStorage.setItem("sild_widget_sound", soundOn ? "on" : "off");
    } catch {
      /* storage unavailable */
    }
    this.patch({ soundOn });
    if (soundOn) chime(); // confirm audibly on unmute
  }

  subscribe(fn: () => void): () => void {
    this.listeners.add(fn);
    return () => this.listeners.delete(fn);
  }
  private emit() {
    for (const l of this.listeners) l();
  }
  private patch(p: Partial<WidgetState>) {
    this.state = { ...this.state, ...p };
    this.emit();
  }

  private async getToken(force = false): Promise<string> {
    if (this.token && !force) return this.token;
    this.token = await this.tokenProvider();
    return this.token;
  }

  private async api<T>(method: string, path: string, body?: unknown): Promise<T> {
    const send = async () => {
      const token = await this.getToken();
      return fetch(this.base + "/v1" + path, {
        method,
        headers: {
          Authorization: `Bearer ${token}`,
          ...(body !== undefined ? { "Content-Type": "application/json" } : {}),
        },
        body: body !== undefined ? JSON.stringify(body) : undefined,
      });
    };
    let res = await send();
    if (res.status === 401) {
      await this.getToken(true); // expired/rotated — refresh once
      res = await send();
    }
    if (res.status === 204) return undefined as T;
    const text = await res.text();
    const data = text ? JSON.parse(text) : null;
    if (!res.ok) throw new Error(data?.error?.message || res.statusText);
    return data as T;
  }

  // fetchBrand loads the active brand (name + config) for an already-authenticated
  // surface (native SDK). The web widget uses fetchPublicBrand at load instead, to
  // avoid minting a token / creating a user just to style the launcher.
  async fetchBrand(): Promise<BrandResponse> {
    return this.api<BrandResponse>("GET", "/me/brand");
  }

  // fetchPublicBrand loads branding unauthenticated, keyed by the host-embedded
  // app id (= tenant id). No Authorization header → no token, no user record.
  async fetchPublicBrand(appId?: string): Promise<BrandResponse> {
    const q = appId ? `?app_id=${encodeURIComponent(appId)}` : "";
    const res = await fetch(this.base + "/v1/public/brand" + q);
    if (!res.ok) throw new Error("brand fetch failed");
    return (await res.json()) as BrandResponse;
  }

  // ── lifecycle ──────────────────────────────────────────────────────────
  async start(conversationId?: string) {
    this.patch({ connection: "connecting", error: null });
    try {
      await this.getToken();
      this.connectRealtime();
      // The list surface needs it, and openConversation reads the target's row from it.
      await this.loadConversations();
      if (conversationId) {
        await this.openConversation(conversationId);
      }
      this.patch({ ready: true });
    } catch (e) {
      this.patch({ error: e instanceof Error ? e.message : "Failed to connect", ready: true });
    }
  }

  private connectRealtime() {
    if (this.cf) return;
    // SSE transport (§5: "SSE is available for the web widget") — proxy-friendly,
    // no WebSocket upgrade needed. Channels are attached server-side.
    this.cf = new Centrifuge([{ transport: "sse", endpoint: this.base + "/v1/ws/sse" }], {
      getToken: async () => this.getToken(true),
    });
    this.cf.on("connected", () => this.patch({ connection: "connected" }));
    this.cf.on("connecting", () => this.patch({ connection: "connecting" }));
    this.cf.on("disconnected", () => this.patch({ connection: "disconnected" }));
    this.cf.on("publication", (ctx) => this.onEvent(ctx.data as Envelope));
    this.cf.connect();
  }

  private onEvent(env: Envelope) {
    if (env.type === "message.created" && env.conversation_id === this.state.activeId) {
      const msg = mapMessage(env.data as ApiMessage, this.selfId, this.activeNames);
      if (this.state.messages.some((m) => m.id === msg.id)) return; // dedupe own echo
      const patch: Partial<WidgetState> = { messages: [...this.state.messages, msg] };
      if (msg.direction === "in" && !msg.system && msg.author) patch.agentName = msg.author;
      this.patch(patch);
      // An agent reply arrived while the thread is open — chime if not muted.
      if (msg.direction === "in" && !msg.system && this.state.soundOn) chime();
    } else if (env.type === "conversation.closed" && env.conversation_id === this.state.activeId) {
      const conv = this.state.conversations.find((c) => c.id === env.conversation_id);
      if (conv) conv.closed = true;
      this.emit();
    }
  }

  // ── data ───────────────────────────────────────────────────────────────
  async loadConversations() {
    const list = await this.api<Array<Record<string, unknown>>>("GET", "/me/conversations");
    const convs: WidgetConversation[] = (list || []).map((c) => {
      const last = (c.last_message || {}) as { body?: string; created_at?: string };
      const assignment = c.assignment as { status?: string } | undefined;
      const members = (c.members as ApiConvMember[]) || [];
      const agentName = (c.agent_name as string) || undefined;
      // Peer conversation: a direct chat with no support agent (no assignment, no
      // agent participant). Render without agent framing.
      const peer = !assignment && !agentName && !members.some((m) => m.member_kind === "agent");
      const names: Record<string, string> = {};
      for (const m of members) {
        if (m.external_user_id) names[m.external_user_id] = String(m.metadata?.name || m.external_user_id);
      }
      // The other party in a peer chat = the (non-agent) member that isn't me.
      const other = members.find((m) => m.member_kind !== "agent" && m.external_user_id && m.external_user_id !== this.selfId);
      const otherName = other ? String(other.metadata?.name || other.external_user_id) : undefined;
      const reference = (c.reference as string) || "";
      return {
        id: String(c.id),
        preview: last.body || "No messages yet",
        time: last.created_at ? clock(last.created_at) : "",
        closed: (c.status as string) === "closed" || assignment?.status === "closed",
        agentName,
        peer,
        names,
        title: peer ? otherName || "Direct chat" : agentName,
        subtitle: peer ? "Direct chat" + (reference ? ` · ${reference}` : "") : undefined,
      };
    });
    // Seed a header fallback name from any conversation that already has an agent.
    const named = convs.find((c) => c.agentName)?.agentName;
    this.patch({ conversations: convs, ...(named ? { agentName: named } : {}) });
  }

  async openConversation(id: string) {
    this.patch({ activeId: id, loadingThread: true, messages: [] });
    // The row carries the peer flag, title/subtitle, closed state and member names, so
    // fetch the list when it's missing — the normal case for a late open(id).
    if (!this.state.conversations.some((c) => c.id === id)) {
      await this.loadConversations().catch(() => {});
    }
    // Resolve author names from the loaded conversation members so a peer thread can
    // label the other party's messages.
    this.activeNames = this.state.conversations.find((c) => c.id === id)?.names || {};
    try {
      const page = await this.api<{ messages: ApiMessage[] }>(
        "GET",
        `/conversations/${id}/messages?limit=100`
      );
      const messages = (page.messages || [])
        .slice()
        .sort((a, b) => a.created_at.localeCompare(b.created_at))
        .map((m) => mapMessage(m, this.selfId, this.activeNames));
      this.patch({ messages, loadingThread: false, agentName: agentNameOf(messages) });
    } catch (e) {
      this.patch({ loadingThread: false, error: e instanceof Error ? e.message : "Failed to load" });
    }
  }

  async openSupportRequest() {
    const conv = await this.api<{ id: string }>("POST", "/me/support-requests", { metadata: this.metadata });
    await this.loadConversations();
    await this.openConversation(conv.id);
    // The socket connected before this conversation existed, so its server-side
    // subscriptions don't cover it yet — reconnect to re-derive membership and
    // pick up conv:<id> (§5.2 mid-connection subscription change).
    this.reconnect();
    return conv.id;
  }

  reconnect() {
    if (!this.cf) return;
    try {
      this.cf.disconnect();
      this.cf.connect();
    } catch {
      /* ignore */
    }
  }

  backToList() {
    this.patch({ activeId: null, messages: [] });
    void this.loadConversations();
  }

  // upload sends a file direct to the bucket via a signed PUT (§11) and returns a
  // reference to attach to a message. Images default to inline (rendered in the
  // thread); everything else is an attachment (listed below the message).
  async upload(file: File): Promise<PendingAttachment> {
    const mime = file.type || "application/octet-stream";
    const grant = await this.api<{ object_key: string; upload_url: string }>("POST", "/uploads", {
      mime_type: mime,
      size_bytes: file.size,
      filename: file.name,
    });
    // The local dev backend returns an absolute URL on its configured public
    // origin; rewrite it to the widget's own base so uploads work from any host
    // (LAN/phone). Real cloud signed URLs have no local route and are used as-is.
    const marker = "/v1/uploads/local/";
    const i = grant.upload_url.indexOf(marker);
    const putUrl = i >= 0 ? this.base + grant.upload_url.slice(i) : grant.upload_url;
    const res = await fetch(putUrl, { method: "PUT", body: file, headers: { "Content-Type": mime } });
    if (!res.ok) throw new Error("upload failed");
    return {
      objectKey: grant.object_key,
      disposition: mime.startsWith("image/") ? "inline" : "attachment",
      mimeType: mime,
      filename: file.name,
    };
  }

  async send(text: string, attachments: PendingAttachment[] = []) {
    const id = this.state.activeId;
    const body = text.trim();
    if (!id || (!body && attachments.length === 0)) return;
    try {
      const msg = await this.api<ApiMessage>("POST", `/conversations/${id}/messages`, {
        body,
        client_msg_id: uuid(),
        attachments: attachments.map((a) => ({ object_key: a.objectKey, disposition: a.disposition })),
      });
      const mapped = mapMessage(msg, this.selfId, this.activeNames);
      if (!this.state.messages.some((m) => m.id === mapped.id)) {
        this.patch({ messages: [...this.state.messages, mapped] });
      }
    } catch (e) {
      this.patch({ error: e instanceof Error ? e.message : "Failed to send" });
    }
  }

  destroy() {
    try {
      this.cf?.disconnect();
    } catch {
      /* ignore */
    }
    this.cf = null;
    this.listeners.clear();
  }

  get connection(): ConnectionState {
    return this.state.connection;
  }
}

// PreviewClient backs the Appearance live preview: a static, no-network client
// seeded with a sample thread so the real <App> renders realistic content
// (bubbles, header, composer) without touching the backend.
export class PreviewClient implements WidgetClient {
  private listeners = new Set<() => void>();

  state: WidgetState = {
    ready: true,
    error: null,
    connection: "connected",
    conversations: [],
    activeId: "preview",
    loadingThread: false,
    soundOn: true,
    agentName: "Eva",
    messages: [
      { id: "p1", direction: "in", author: "Eva", body: "Hi! How can we help with your trip today?", time: "" },
      { id: "p2", direction: "out", body: "How do I change my pickup address?", time: "" },
      { id: "p3", direction: "in", author: "Eva", body: "Open your trip, tap the pickup pin, and drag it to a new spot.", time: "" },
    ],
  };

  subscribe(fn: () => void): () => void {
    this.listeners.add(fn);
    return () => this.listeners.delete(fn);
  }
  start(): void {}
  openConversation(): void {}
  openSupportRequest(): void {}
  send(): void {}
  backToList(): void {}
  reconnect(): void {}
  toggleSound(): void {
    this.state = { ...this.state, soundOn: !this.state.soundOn };
    for (const l of this.listeners) l();
  }
  async upload(file: File): Promise<PendingAttachment> {
    return { objectKey: "", disposition: "attachment", mimeType: file.type, filename: file.name };
  }
}
