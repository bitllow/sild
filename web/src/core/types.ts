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

// Public configuration for the drop-in (spec §9).
export interface SildConfig {
  /** Mints a user JWT via the host backend (which holds the API key). Never the
   *  API key itself. A guest is the same call with a host-generated id. */
  tokenProvider: () => Promise<string> | string;
  /** Base URL of the Sild backend. Defaults to the script's origin. */
  baseUrl?: string;
  /** Public app id (the tenant's id) used to fetch branding at load without
   *  authenticating — no token minted, no user record. Omit in single-tenant dev.
   *  Without it the launcher renders with defaults (+ any inline `brand`). */
  appId?: string;
  /** Open directly to this conversation. Required for guest tokens (§9); omit
   *  for an authed user to show the conversation list / open a new request. */
  conversationId?: string;
  /** Per-participant metadata attached when this user opens a support request
   *  (becomes conversation_members.metadata — shown in the inbox member panel).
   *  Host-defined and opaque, e.g. { name, email, phone, plan }. */
  metadata?: Record<string, unknown>;
  /** Optional inline brand overrides. When omitted, the widget fetches the
   *  active brand from GET /v1/me/brand at load. */
  brand?: Partial<BrandConfig>;
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
  /** Reply-notification sound: chimes on incoming agent messages when true.
   *  Toggled from either header (home + thread), so it lives in shared state. */
  soundOn: boolean;
  /** The support agent's display name (their first name), learned from incoming
   *  messages — shown in the thread header + on incoming bubbles instead of the
   *  generic "Support". */
  agentName?: string;
}
