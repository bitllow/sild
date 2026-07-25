// Typed bindings for the Sild admin API (§4.3). Shapes mirror internal/views
// and internal/api/admin.go on the Go side.

import { api } from "./client";

export type ApiAssignmentStatus = "queued" | "assigned" | "closed";
export type ApiConvStatus = "open" | "closed";
export type ApiSenderKind = "user" | "agent" | "bot" | "system";
export type ApiVisibility = "participants" | "internal";
export type ApiChannel = "app" | "email";
export type ApiMemberKind = "user" | "agent" | "bot" | "email";
// Conversation roles are tenant-defined free strings ("rider"/"driver"/"support"/
// "client"/…). The UI derives all labels and colors from whatever roles appear.
export type ApiConvRole = string;
export type ApiPlatformRole = "owner" | "admin" | "agent";

export interface ApiAssignment {
  id: string;
  conversation_id: string;
  status: ApiAssignmentStatus;
  created_at: string;
  assignee_actor_id?: string;
  closed_at?: string | null;
}

export interface ApiMember {
  member_kind: ApiMemberKind;
  conv_role: ApiConvRole;
  metadata: Record<string, string> | null;
  joined_at: string;
  external_user_id?: string;
  internal_actor_id?: string;
}

export interface ApiConversation {
  id: string;
  status: ApiConvStatus;
  reference: string;
  metadata: Record<string, unknown> | null;
  created_at: string;
  members: ApiMember[];
  assignment?: ApiAssignment;
  subject?: string; // email channel only — original email subject
}

export interface ApiAttachment {
  object_key: string;
  disposition: "inline" | "attachment";
  mime_type: string;
  size_bytes: number;
  filename: string;
  url?: string;
}

export interface ApiMessage {
  id: string;
  conversation_id: string;
  sender_kind: ApiSenderKind;
  visibility: ApiVisibility;
  channel: ApiChannel;
  body: string;
  created_at: string;
  external_user_id?: string;
  internal_actor_id?: string;
  client_msg_id?: string;
  /** Agent's display name for agent/system-authored messages (from the server). */
  author_name?: string;
  attachments: ApiAttachment[];
}

export interface ApiUploadGrant {
  object_key: string;
  upload_url: string;
  expires_at: string;
}

export interface AttachmentRef {
  object_key: string;
  disposition: "inline" | "attachment";
}

export interface ApiMessagesPage {
  items: ApiMessage[];
  next_cursor?: string | null;
  has_more?: boolean;
}

// One inbox queue row: the conversation (members + last message preview + last
// activity) WITHOUT history — opening it fetches the thread lazily (§4.3).
// A row from GET /v1/conversations. It IS a conversation — the assignment is a
// nested field, not a wrapper — so the queue, the peer list, contact history and
// search all render the same shape.
export interface ApiQueueConversation extends ApiConversation {
  last_activity: string;
  last_message?: { body: string; created_at: string | null };
  /** support (queued for an agent) or peer (direct chat, no assignment). */
  kind?: "support" | "peer";
  agent_name?: string;
  unread_count?: number;
  /** Search annotations — present only on rows returned for a ?q= query. */
  snippet?: string;
  matched_fields?: string[];
}

export interface ApiQueuePage {
  items: ApiQueueConversation[];
  next_cursor: string | null;
  has_more: boolean;
  // Inbox scope counters, emitted only for the support queue. you = assigned to
  // me, unassigned = queued, closed = closed CONVERSATIONS (the inbox's notion
  // of closed is the conversation, not the assignment).
  counts?: {
    open: number;
    you: number;
    unassigned: number;
    closed: number;
  };
}

// last_activity (default), created = date started, waiting_since = queued-since.
export type QueueSort = "last_activity" | "created" | "waiting_since";
export type QueueOrder = "asc" | "desc";


export interface ApiKeyRecord {
  id: string;
  label: string;
  prefix: string;
  created_at: string;
  revoked_at?: string | null;
}

export interface ApiKeyCreated {
  id: string;
  key: string;
  label: string;
  prefix: string;
}

export interface ApiWebhook {
  id: string;
  url: string;
  events: string[];
  active: boolean;
  created_at: string;
}


export interface ApiTeamMember {
  id: string;
  email: string;
  first_name?: string;
  last_name?: string;
  platform_role: ApiPlatformRole;
  /** Per-user access to the peer-conversations surface (Settings → Team). */
  peer_access: boolean;
  has_password: boolean;
  created_at: string;
}

/**
 * The list envelope every collection endpoint returns. One shape means one list
 * parser; `next_cursor` is null exactly when `has_more` is false, and the cursor
 * is opaque — round-trip it verbatim, never parse it.
 */
export interface ApiPage<T> {
  items: T[];
  next_cursor: string | null;
  has_more: boolean;
}

/** Filters for GET /v1/conversations — the one conversation list. */
export interface ConversationParams {
  kind?: "support" | "peer";
  /** Conversation lifecycle. Distinct from assignmentStatus: the inbox's notion
   *  of "closed" is the CONVERSATION being closed. */
  status?: "open" | "closed";
  /** Assignment state machine (queued → assigned → closed). */
  assignmentStatus?: "queued" | "assigned" | "closed";
  assignee?: string;
  participant?: string;
  role?: string;
  q?: string;
  sort?: QueueSort;
  order?: "asc" | "desc";
  limit?: number;
  cursor?: string | null;
}

/** One action the caller holds, with the scope it is held over. */
export interface ApiGrant {
  action: string;
  scope?: {
    kinds?: Array<"support" | "peer">;
    requires_assignment?: boolean;
    participant?: string;
  };
}

/**
 * GET /v1/principal — who am I, for any credential.
 *
 * `grants` carry the SCOPE of each action, not just its name: an agent with peer
 * access and one without both hold conversations.list, and only the scope tells
 * them apart. Rendering affordances from this keeps the frontend from
 * re-deriving policy from role flags.
 */
export interface ApiPrincipal {
  kind: "admin" | "user" | "apikey";
  tenant_id: string;
  subject?: {
    id: string;
    role?: ApiPlatformRole;
    email?: string;
    first_name?: string;
    last_name?: string;
  };
  grants: ApiGrant[];
}

/** A person the tenant has talked to, projected from membership rows. */
export interface ApiContact {
  external_user_id: string;
  metadata?: Record<string, unknown>;
  last_activity: string;
  /** Current conversations only — an archived-only contact is findable with 0. */
  conversation_count: number;
}

export interface ApiEmailChannel {
  channel: "email";
  forwarding_address: string;
  inbound_domain: string;
  verified: boolean;
  auto_reply: boolean;
  spam_filter: boolean;
  from_name: string;
  from_address: string;
}

export interface EmailChannelPatch {
  auto_reply?: boolean;
  spam_filter?: boolean;
  from_name?: string;
  from_address?: string;
}

// ── Appearance: web/SDK messenger branding (§8) ─────────────────────────────
// One brand profile's config — the messenger look the widget + SDK render.
// Mirrors domain.BrandConfig on the Go side.
export interface ApiBrandConfig {
  logo: string; // bucket object key (or legacy data:/http URL)
  logoUrl?: string; // resolved signed URL for display (read-only, from server)
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

export interface ApiBrand {
  id: string;
  name: string;
  config: ApiBrandConfig;
}

export interface ApiBrands {
  brands: ApiBrand[];
  active_brand_id: string;
}

// ── Auth ──────────────────────────────────────────────────────────────────
export const adminApi = {
  loginPassword: (email: string, password: string) =>
    api.post<{ status: string; expires_at: string }>("/admin/auth/password", { email, password }),
  logout: () => api.post<void>("/admin/auth/logout"),
  googleLoginUrl: () => "/v1/admin/auth/google",

  // ── Conversations ─────────────────────────────────────────────────────
  // One endpoint backs the support queue, the peer inbox, contact history and
  // search. The credential decides the subset; these are just filters.
  listConversations: (params?: ConversationParams) => {
    const q = new URLSearchParams();
    if (params?.kind) q.set("kind", params.kind);
    if (params?.status) q.set("status", params.status);
    if (params?.assignmentStatus) q.set("assignment_status", params.assignmentStatus);
    if (params?.assignee) q.set("assignee", params.assignee);
    if (params?.participant) q.set("participant", params.participant);
    if (params?.role) q.set("role", params.role);
    if (params?.q) q.set("q", params.q);
    if (params?.sort) q.set("sort", params.sort);
    if (params?.order) q.set("order", params.order);
    if (params?.limit) q.set("limit", String(params.limit));
    if (params?.cursor) q.set("cursor", params.cursor);
    const qs = q.toString();
    return api.get<ApiQueuePage>(`/conversations${qs ? `?${qs}` : ""}`);
  },
  getConversation: (id: string) => api.get<ApiConversation>(`/conversations/${id}`),
  listMessages: (id: string) => api.get<ApiMessagesPage>(`/conversations/${id}/messages?limit=100`),
  postMessage: (
    id: string,
    body: string,
    visibility: ApiVisibility = "participants",
    attachments: AttachmentRef[] = []
  ) => api.post<ApiMessage>(`/conversations/${id}/messages`, { body, visibility, attachments }),
  // Issue a signed direct-to-bucket PUT URL for an attachment (§11).
  issueUpload: (mimeType: string, sizeBytes: number, filename: string) =>
    api.post<ApiUploadGrant>("/uploads", { mime_type: mimeType, size_bytes: sizeBytes, filename }),
  // Claiming is a partial update of the assignment; "me" is the only accepted
  // assignee, so this cannot become reassignment by accident.
  claimAssignment: (id: string) =>
    api.patch<ApiAssignment>(`/assignments/${id}`, { assignee_actor_id: "me" }),
  closeAssignment: (id: string) =>
    api.patch<ApiAssignment>(`/assignments/${id}`, { status: "closed" }),
  closeConversation: (id: string) => api.post<{ status: string }>(`/conversations/${id}/close`),

  // ── Contacts (people, not threads) ────────────────────────────────────
  listContacts: (q?: string) =>
    api.get<ApiPage<ApiContact>>(`/contacts${q ? `?q=${encodeURIComponent(q)}` : ""}`),

  // ── Settings: API keys ────────────────────────────────────────────────
  listApiKeys: () => api.get<ApiPage<ApiKeyRecord>>("/api-keys"),
  createApiKey: (label: string) => api.post<ApiKeyCreated>("/api-keys", { label }),
  revokeApiKey: (id: string) => api.del<void>(`/api-keys/${id}`),

  // ── Settings: webhooks ────────────────────────────────────────────────
  listWebhooks: () => api.get<ApiPage<ApiWebhook>>("/webhooks"),
  setWebhookActive: (id: string, active: boolean) =>
    api.patch<void>(`/webhooks/${id}`, { active }),
  deleteWebhook: (id: string) => api.del<void>(`/webhooks/${id}`),

  // ── Signed-in principal ───────────────────────────────────────────────
  // Grants carry the SCOPE each action is held over, so the peer surface is
  // rendered from the same decision the backend enforces rather than from a
  // duplicated peer_access rule.
  me: () => api.get<ApiPrincipal>("/principal"),

  // ── Settings: team ────────────────────────────────────────────────────
  listTeam: () => api.get<ApiPage<ApiTeamMember>>("/team"),
  setTeamRole: (id: string, role: ApiPlatformRole) =>
    api.patch<void>(`/team/${id}`, { platform_role: role }),
  setTeamPeerAccess: (id: string, peerAccess: boolean) =>
    api.patch<void>(`/team/${id}`, { peer_access: peerAccess }),

  // ── Settings: channels (§6.2) ─────────────────────────────────────────
  getEmailChannel: () => api.get<ApiEmailChannel>("/channels/email"),
  updateEmailChannel: (patch: EmailChannelPatch) =>
    api.patch<ApiEmailChannel>("/channels/email", patch as Record<string, unknown>),

  // ── Settings: appearance (§8) — brands saved as one staged set ─────────
  getBrands: () => api.get<ApiBrands>("/brands"),
  saveBrands: (brands: ApiBrand[], activeBrandId: string) =>
    api.put<ApiBrands>("/brands", { brands, active_brand_id: activeBrandId }),
};
