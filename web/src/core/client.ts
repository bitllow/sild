import { Centrifuge } from "centrifuge";
import type {
  ConnectionState,
  PendingAttachment,
  SildConfig,
  WidgetConversation,
  WidgetMessage,
  WidgetState,
} from "./types";

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
  attachments?: ApiAttachment[];
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

function mapMessage(m: ApiMessage): WidgetMessage {
  const system = m.sender_kind === "system";
  const out = m.sender_kind === "user";
  return {
    id: m.id,
    direction: out ? "out" : "in",
    system,
    author: system ? undefined : out ? undefined : "Support",
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
export class SildClient {
  private base: string;
  private tokenProvider: () => Promise<string> | string;
  private metadata: Record<string, unknown>;
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
  };

  constructor(cfg: SildConfig) {
    this.base = (cfg.baseUrl || "").replace(/\/$/, "");
    this.tokenProvider = cfg.tokenProvider;
    this.metadata = cfg.metadata || {};
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

  // ── lifecycle ──────────────────────────────────────────────────────────
  async start(conversationId?: string) {
    this.patch({ connection: "connecting", error: null });
    try {
      await this.getToken();
      this.connectRealtime();
      if (conversationId) {
        await this.openConversation(conversationId);
      } else {
        await this.loadConversations();
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
      const msg = mapMessage(env.data as ApiMessage);
      if (this.state.messages.some((m) => m.id === msg.id)) return; // dedupe own echo
      this.patch({ messages: [...this.state.messages, msg] });
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
      return {
        id: String(c.id),
        preview: last.body || "No messages yet",
        time: last.created_at ? clock(last.created_at) : "",
        closed: (c.status as string) === "closed" || assignment?.status === "closed",
      };
    });
    this.patch({ conversations: convs });
  }

  async openConversation(id: string) {
    this.patch({ activeId: id, loadingThread: true, messages: [] });
    try {
      const page = await this.api<{ messages: ApiMessage[] }>(
        "GET",
        `/conversations/${id}/messages?limit=100`
      );
      const messages = (page.messages || [])
        .slice()
        .sort((a, b) => a.created_at.localeCompare(b.created_at))
        .map(mapMessage);
      this.patch({ messages, loadingThread: false });
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

  private reconnect() {
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
      const mapped = mapMessage(msg);
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
