// BrandConfig is the messenger look the admin configures in Settings →
// Appearance and both the web widget and native SDKs render. Mirrors
// domain.BrandConfig on the Go side; the active brand's config is served at
// GET /v1/me/brand. logo/iconImg are data URLs (or asset URLs in production).
export interface BrandConfig {
  /** Bucket object key (or legacy data:/http URL) — prefer logoUrl to render. */
  logo: string;
  /** Resolved signed URL for the logo asset (read-only, from the server). */
  logoUrl?: string;
  brand: string;
  theme: "light" | "dark" | "auto";
  font: "system" | "sild" | "inter" | "figtree" | "dmsans";
  radius: "sharp" | "default" | "rounded" | "pillowy";
  launcherIcon: "chat" | "message" | "help" | "sparkle" | "custom";
  iconImg: string;
  iconImgUrl?: string;
  launcherPos: "left" | "right";
  launcherSize: "sm" | "md" | "lg";
  heading: string;
  sub: string;
  topics: string;
  showTeam: boolean;
  poweredBy: boolean;
}

// DEFAULT_BRAND is the fallback look, used until the server config loads (and if
// the fetch fails). Kept in sync with domain.DefaultBrandConfig.
export const DEFAULT_BRAND: BrandConfig = {
  logo: "",
  brand: "#2563FD",
  theme: "light",
  font: "sild",
  radius: "default",
  launcherIcon: "chat",
  iconImg: "",
  launcherPos: "right",
  launcherSize: "md",
  heading: "Hi there.",
  sub: "How can we help? We typically reply in a few minutes.",
  topics: "",
  showTeam: false,
  poweredBy: true,
};

/** The active brand as returned by GET /v1/me/brand. */
export interface BrandResponse {
  name: string;
  config: Partial<BrandConfig>;
}

/** GET /v1/translations/manifest — each locale's current published version. */
export interface TranslationManifest {
  project: string;
  fallback_locale: string;
  locales: Record<string, number>;
}

/** GET /v1/translations/bundle — every key resolved for one locale. */
export interface TranslationBundle {
  project: string;
  locale: string;
  version: number;
  strings: Record<string, string>;
}

// Public configuration for the drop-in (spec §9).
export interface SildConfig {
  /** Mints a user JWT via the host backend (which holds the API key). Never the
   *  API key itself. A guest is the same call with a host-generated id. */
  tokenProvider: () => Promise<string> | string;
  /** Base URL of the Sild backend. Defaults to the script's origin. */
  baseUrl?: string;
  /** Public app id (the tenant's id), used to fetch branding at load without
   *  authenticating — no token minted, no user record. REQUIRED: the anonymous
   *  brand read names its tenant outright rather than being inferred from how many
   *  the deployment holds. Find it in the inbox under Settings → Installation. */
  appId: string;
  /** Open directly to this conversation. Required for guest tokens (§9); omit
   *  for an authed user to show the conversation list / open a new request. */
  conversationId?: string;
  /** The end-user's own id (the token's subject). Supplied by the host, which
   *  minted the token and already knows it. Lets the widget tell the visitor's
   *  own messages from the other party's in a peer conversation (rider↔driver),
   *  where both are user-authored. Optional — support-only usage doesn't need it. */
  userId?: string;
  /** The signed-in user's profile, upserted on every start() and shared by every
   *  conversation they are in (shown in the inbox member panel). Host-defined and
   *  opaque, e.g. { name, email, phone, plan }. This is the WHOLE profile, not a
   *  patch: it replaces whatever Sild holds, and the last writer wins across a
   *  person's devices. An empty object asserts an empty profile and erases it;
   *  omitting it asserts nothing and leaves whatever Sild holds. */
  metadata?: Record<string, unknown>;
  /** Optional inline brand overrides. When omitted, the widget fetches the
   *  active brand from GET /v1/me/brand at load. */
  brand?: Partial<BrandConfig>;
  /** Render in this language instead of the visitor's browser preference. */
  locale?: string;
}

export type Direction = "in" | "out";

export type Disposition = "inline" | "attachment";

export interface WidgetAttachment {
  /** Signed GET URL for rendering/download (per-request, may be absent). */
  url?: string;
  /** inline = render in the message body (images); attachment = list below it. */
  disposition: Disposition;
  mimeType: string;
  filename: string;
  sizeBytes: number;
}

/** A file uploaded and ready to attach to the next message. */
export interface PendingAttachment {
  objectKey: string;
  disposition: Disposition;
  mimeType: string;
  filename: string;
}

export interface WidgetMessage {
  id: string;
  direction: Direction;
  system?: boolean;
  author?: string;
  time: string;
  body: string;
  attachments?: WidgetAttachment[];
}

export interface WidgetConversation {
  id: string;
  preview: string;
  time: string;
  closed: boolean;
  /** The handling agent's display name (first name), if one is assigned. */
  agentName?: string;
  /** A peer conversation: a direct chat between end-user parties (e.g. rider↔
   *  driver) with no support agent. Rendered without agent framing. */
  peer?: boolean;
  /** Row/header title — the other party for a peer chat, else the agent. */
  title?: string;
  /** The host's reference for a peer chat, e.g. "trip 9021" — shown under the title. */
  reference?: string;
  /** external_user_id → display name, for resolving message authors in the thread. */
  names?: Record<string, string>;
}

export type ConnectionState = "idle" | "connecting" | "connected" | "disconnected";

export interface WidgetState {
  ready: boolean;
  error: string | null;
  connection: ConnectionState;
  conversations: WidgetConversation[];
  activeId: string | null;
  messages: WidgetMessage[];
  loadingThread: boolean;
  /** Cursor for the next page of OLDER messages; null when the thread is whole. */
  olderCursor: string | null;
  loadingOlder: boolean;
  /** Reply-notification sound: chimes on incoming agent messages when true.
   *  Toggled from either header (home + thread), so it lives in shared state. */
  soundOn: boolean;
  /** The support agent's display name (their first name), learned from incoming
   *  messages — shown in the thread header + on incoming bubbles instead of the
   *  generic "Support". */
  agentName?: string;
}

/** The list envelope every collection endpoint returns. */
export interface ApiPage<T> {
  items: T[];
  next_cursor: string | null;
  has_more: boolean;
}
