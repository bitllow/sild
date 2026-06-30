// Public configuration for the drop-in (spec §9).
export interface SildConfig {
  /** Mints a user JWT via the host backend (which holds the API key). Never the
   *  API key itself. A guest is the same call with a host-generated id. */
  tokenProvider: () => Promise<string> | string;
  /** Base URL of the Sild backend. Defaults to the script's origin. */
  baseUrl?: string;
  /** Open directly to this conversation. Required for guest tokens (§9); omit
   *  for an authed user to show the conversation list / open a new request. */
  conversationId?: string;
  /** Per-participant metadata attached when this user opens a support request
   *  (becomes conversation_members.metadata — shown in the inbox member panel).
   *  Host-defined and opaque, e.g. { name, email, phone, plan }. */
  metadata?: Record<string, unknown>;
  /** Launcher accent (defaults to the Sild brand blue). */
  accent?: string;
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
}
