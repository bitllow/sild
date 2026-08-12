// Typed bindings for the Sild admin API (§4.3). Shapes mirror internal/views
// and internal/api/admin.go on the Go side.

import { api, collectAll } from "./client";

export type ApiAssignmentStatus = "queued" | "assigned" | "closed";
export type ApiConvStatus = "open" | "closed";
export type ApiSenderKind = "user" | "agent" | "bot" | "system";
export type ApiVisibility = "participants" | "internal";
export type ApiChannel = "app" | "email";
export type ApiMemberKind = "user" | "agent" | "bot" | "email";
// Conversation roles are tenant-defined free strings ("rider"/"driver"/"support"/
// "client"/…). The UI derives all labels and colors from whatever roles appear.
export type ApiConvRole = string;
export type ApiPlatformRole = "owner" | "admin" | "agent" | "translator";

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
  /** Display name, inline because every surface renders it. The rest of the
   *  profile is the contacts resource — ask for it with expand=contacts.metadata. */
  name?: string;
  joined_at: string;
  external_user_id?: string;
  internal_actor_id?: string;
}

// One entry of the `contacts` expansion block. Distinct from ApiContact below,
// which is the directory projection with its aggregates.
export interface ApiExpandedContact {
  external_user_id: string;
  name?: string;
  metadata?: Record<string, unknown> | null;
}

// profilesOf indexes an expansion block by person, so a member view can be
// rendered with the profile the same response already carried.
export function profilesOf(block?: ApiExpandedContact[] | null): Record<string, Record<string, unknown>> {
  const out: Record<string, Record<string, unknown>> = {};
  for (const c of block || []) {
    if (c.metadata) out[c.external_user_id] = c.metadata;
  }
  return out;
}

export interface ApiConversation {
  contacts?: ApiExpandedContact[];
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
// A row from GET /v1/conversations — the assignment is a nested field, so the
// queue, peer list, contact history and search share one shape.
export interface ApiQueueConversation extends ApiConversation {
  last_activity: string;
  last_message?: { body: string; created_at: string | null };
  /** support (queued for an agent) or peer (direct chat, no assignment). */
  kind?: "support" | "peer";
  agent_name?: string;
  unread_count?: number;
  /** The matching fragment, present only on rows returned for a ?q= query. */
  snippet?: string;
}

export interface ApiQueuePage extends ApiPage<ApiQueueConversation> {
  contacts?: ApiExpandedContact[];
  // Support queue only. closed counts closed CONVERSATIONS, not assignments.
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
  /** What the key is held to. Empty means tenant-wide. */
  projects?: string[] | null;
  locales?: string[] | null;
  publish?: boolean;
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


/** The limits one assignment carries. Which of them a role has is the role's
 *  own declaration, served by /v1/roles. */
export interface ApiRoleScope {
  peer?: boolean;
  projects?: string[];
  locales?: string[];
  publish?: boolean;
}

export interface ApiRoleAssignment {
  role: ApiPlatformRole;
  scope: ApiRoleScope;
}

export interface ApiTeamMember {
  id: string;
  email: string;
  first_name?: string;
  last_name?: string;
  /** Every role this member holds, each with its scope. */
  assignments: ApiRoleAssignment[];
  has_password: boolean;
  created_at: string;
}

/** A role as the Team screen renders it: what it is for, and what it scopes. */
export interface ApiRoleDefinition {
  role: ApiPlatformRole;
  label: string;
  description: string;
  dimensions: {
    key: keyof ApiRoleScope;
    kind: "set" | "toggle";
    label: string;
    help: string;
    source?: string;
  }[];
}

/**
 * The list envelope every collection endpoint returns. `next_cursor` is null
 * exactly when `has_more` is false, and is opaque — round-trip it verbatim.
 */
export interface ApiPage<T> {
  items: T[];
  next_cursor: string | null;
  has_more: boolean;
}

/** Filters for GET /v1/conversations — the one conversation list. */
export interface ConversationParams {
  kind?: "support" | "peer";
  /** Conversation lifecycle — the inbox's notion of "closed". */
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
 * `grants` carry the SCOPE of each action, not just its name: two agents can both
 * hold conversations.list and only the scope tells them apart.
 */
export interface ApiPrincipal {
  kind: "admin" | "user" | "apikey";
  tenant_id: string;
  subject?: {
    id: string;
    /** The caller's own assignments, scope included — a translator is refused
     *  /v1/team and has no other way to see what they were granted. */
    assignments?: ApiRoleAssignment[];
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

// Push setup (§5.5). The credential itself is write-only — a read gives the
// project identity so you can confirm which project is wired up, never the key.
export interface ApiPushChannel {
  project_id: string;
  client_email: string;
  verified: boolean;
  updated_at: string;
  include_sender: boolean;
  include_body: boolean;
  sender_source: "brand" | "agent";
}

export interface PushSettingsPatch {
  include_sender: boolean;
  include_body: boolean;
  sender_source: "brand" | "agent";
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

// ── Translations ───────────────────────────────────────────────────────────
// A project's strings in one locale, plus the releases that publish them.
export interface ApiTranslationProject {
  id: string;
  slug: string;
  name: string;
  fallback_locale: string;
  auto_publish: boolean;
  /** Locales the tenant has turned on, a subset of available_locales. */
  locales: string[];
  platform: boolean;
  available_locales: string[];
  current_version: number | null;
  /** How many keys the project declares, and the share of them translated per locale. */
  keys: number;
  completion: Record<string, number>;
  namespaces: string[] | null;
}

export interface TranslationProjectSettings {
  name?: string;
  fallback_locale: string;
  auto_publish: boolean;
  locales: string[];
}

export type ApiTranslationState = "default" | "custom" | "needs_review";
/** The state filter's values — `missing` is a query, never a returned state. */
export type TranslationStateFilter = "" | "custom" | "missing" | "needs_review";

export interface ApiTranslationKey {
  key: string;
  /** The key's grouping prefix, derived from everything before its first dot. */
  namespace: string;
  /** English source text. */
  source: string;
  /** Draft text for the requested locale: the override if one exists, else the default. */
  value: string;
  state: ApiTranslationState;
  /** Set on a plural's category sibling: the key it belongs to, and which form it is. */
  plural_base: string;
  plural_category: string;
  /** The `{name}`s this text will have filled in. */
  placeholders: string[] | null;
}

export interface TranslationKeyParams {
  locale: string;
  state?: TranslationStateFilter;
  namespace?: string;
  q?: string;
  limit?: number;
  cursor?: string | null;
}

export interface ApiTranslationRelease {
  version: number;
  published_at: string;
  published_by?: string;
}

export interface ApiTranslationPublished {
  version: number;
  published_at: string;
  locales: string[];
}

/** A file shape an import accepts; `sheet` is export-only. */
export type TranslationFormat =
  | "json"
  | "csv"
  | "sheet"
  | "android"
  | "ios"
  | "ios-plurals"
  | "kotlin"
  | "swift"
  | "typescript";

export interface ApiImportRow {
  key: string;
  status: "new" | "changed" | "skipped";
  value: string;
  reason: string;
}

/** What an import did, or — with dry_run — what it would do. */
export interface ApiImportReport {
  project: string;
  locale: string;
  format: string;
  dry_run: boolean;
  new: number;
  changed: number;
  skipped: number;
  keys_created: number;
  rows: ApiImportRow[];
}

export interface ApiDraftRow {
  locale: string;
  key: string;
  /** What the current release serves; empty for a key that is new since it. */
  live: string;
  draft: string;
}

/** What publishing would change — the preview before approving a release. */
export interface ApiDraftDiff {
  project: string;
  version: number | null;
  next_version: number;
  rows: ApiDraftRow[];
}

// ── Auth ──────────────────────────────────────────────────────────────────
export const adminApi = {
  loginPassword: (email: string, password: string) =>
    api.post<{ status: string; expires_at: string }>("/admin/auth/password", { email, password }),
  logout: () => api.post<void>("/admin/auth/logout"),
  googleLoginUrl: () => "/v1/admin/auth/google",

  // ── Conversations ─────────────────────────────────────────────────────
  // One endpoint backs the queue, the peer inbox, contact history and search.
  // Drains the cursor: the Details-panel history and the contact filter render a
  // whole list, with no scroll affordance to continue from.
  listAllConversations: async (params?: ConversationParams) => {
    // collectAll keeps only the items, so the per-page contacts blocks are
    // re-read here and merged into one lookup for the whole drained list.
    const pages: ApiExpandedContact[] = [];
    const items = await collectAll<ApiQueueConversation>(async (cursor) => {
      const page = await adminApi.listConversations({ ...params, cursor });
      pages.push(...(page.contacts || []));
      return page;
    });
    return { items, contacts: pages };
  },
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
    // The panels render whole profiles, so the blob is asked for by name; the
    // page carries one block instead of a fetch per row.
    q.set("expand", "contacts.metadata");
    const qs = q.toString();
    return api.get<ApiQueuePage>(`/conversations${qs ? `?${qs}` : ""}`);
  },
  getConversation: (id: string) =>
    api.get<ApiConversation>(`/conversations/${id}?expand=contacts.metadata`),
  // cursor pages BACKWARD (older); ?since= below is the opposite direction.
  listMessages: (id: string, cursor?: string) =>
    api.get<ApiMessagesPage>(
      `/conversations/${id}/messages?limit=100` +
        (cursor ? `&cursor=${encodeURIComponent(cursor)}` : "")
    ),
  // ?since= is a sync read, not a page: oldest-first, next_cursor always null, and
  // continuation is `since=<last id received>` while has_more holds.
  catchUpMessages: (id: string, since: string, limit = 100) =>
    api.get<ApiMessagesPage>(
      `/conversations/${id}/messages?since=${encodeURIComponent(since)}&limit=${limit}`
    ),
  postMessage: (
    id: string,
    body: string,
    visibility: ApiVisibility = "participants",
    attachments: AttachmentRef[] = []
  ) => api.post<ApiMessage>(`/conversations/${id}/messages`, { body, visibility, attachments }),
  // Issue a signed direct-to-bucket PUT URL for an attachment (§11).
  issueUpload: (mimeType: string, sizeBytes: number, filename: string) =>
    api.post<ApiUploadGrant>("/uploads", { mime_type: mimeType, size_bytes: sizeBytes, filename }),
  // "me" is the only accepted assignee, so this cannot become reassignment.
  claimAssignment: (id: string) =>
    api.patch<ApiAssignment>(`/assignments/${id}`, { assignee_actor_id: "me" }),
  closeAssignment: (id: string) =>
    api.patch<ApiAssignment>(`/assignments/${id}`, { status: "closed" }),
  closeConversation: (id: string) => api.post<{ status: string }>(`/conversations/${id}/close`),

  // ── Contacts (people, not threads) ────────────────────────────────────
  listContacts: (q?: string) =>
    api.get<ApiPage<ApiContact>>(`/contacts${q ? `?q=${encodeURIComponent(q)}` : ""}`),

  // ── Settings: API keys ────────────────────────────────────────────────
  // Settings screens render a whole collection, so they drain the cursor.
  listApiKeys: () =>
    collectAll<ApiKeyRecord>((cursor) =>
      api.get<ApiPage<ApiKeyRecord>>(`/api-keys${cursor ? `?cursor=${encodeURIComponent(cursor)}` : ""}`)
    ),
  /** A scope holds the key to those translation projects and languages — a build
   *  token. Omit it for a tenant-wide key. */
  createApiKey: (label: string, scope?: { projects?: string[]; locales?: string[]; publish?: boolean }) =>
    api.post<ApiKeyCreated>("/api-keys", { label, ...(scope ?? {}) }),
  revokeApiKey: (id: string) => api.del<void>(`/api-keys/${id}`),

  // ── Settings: webhooks ────────────────────────────────────────────────
  listWebhooks: () =>
    collectAll<ApiWebhook>((cursor) =>
      api.get<ApiPage<ApiWebhook>>(`/webhooks${cursor ? `?cursor=${encodeURIComponent(cursor)}` : ""}`)
    ),
  setWebhookActive: (id: string, active: boolean) =>
    api.patch<void>(`/webhooks/${id}`, { active }),
  deleteWebhook: (id: string) => api.del<void>(`/webhooks/${id}`),

  // ── Signed-in principal ───────────────────────────────────────────────
  // Grants carry each action's scope, so the peer surface renders from the
  // decision the backend enforces.
  me: () => api.get<ApiPrincipal>("/principal"),

  // ── Settings: team ────────────────────────────────────────────────────
  listTeam: () =>
    collectAll<ApiTeamMember>((cursor) =>
      api.get<ApiPage<ApiTeamMember>>(`/team${cursor ? `?cursor=${encodeURIComponent(cursor)}` : ""}`)
    ),
  listRoles: () => api.get<{ roles: ApiRoleDefinition[] }>("/roles"),
  inviteMember: (member: {
    email: string;
    first_name: string;
    last_name: string;
    role: ApiPlatformRole;
    scope: ApiRoleScope;
  }) => api.post<ApiTeamMember>("/team", member),
  assignRole: (id: string, role: ApiPlatformRole, scope: ApiRoleScope) =>
    api.post<void>(`/team/${id}/roles`, { role, scope }),
  setRoleScope: (id: string, role: ApiPlatformRole, scope: ApiRoleScope) =>
    api.put<void>(`/team/${id}/roles/${role}`, { scope }),
  removeRole: (id: string, role: ApiPlatformRole) => api.del<void>(`/team/${id}/roles/${role}`),

  // ── Settings: channels (§6.2) ─────────────────────────────────────────
  // Read returns the version; the write quotes it back, so a concurrent edit
  // fails loudly (412) instead of being overwritten.
  getEmailChannel: () => api.getVersioned<ApiEmailChannel>("/channels/email"),
  updateEmailChannel: (patch: EmailChannelPatch, etag: string) =>
    api.patchIfMatch<ApiEmailChannel>("/channels/email", patch as Record<string, unknown>, etag),

  // ── Settings: push (§5.5) ─────────────────────────────────────────────
  getPushChannel: () => api.get<ApiPushChannel>("/channels/push"),
  updatePushSettings: (patch: PushSettingsPatch) =>
    api.patch<void>("/channels/push", patch as unknown as Record<string, unknown>),
  // The credential is checked against the provider before it is stored, so a
  // rejection here means the credential, not the network.
  setPushCredential: (credential: unknown) =>
    api.put<void>("/channels/push/credential", { credential }),
  deletePushCredential: () => api.del<void>("/channels/push/credential"),
  testPushSend: (token: string) => api.post<void>("/channels/push/test", { token }),

  // ── Translations ──────────────────────────────────────────────────────
  listTranslationProjects: () =>
    collectAll<ApiTranslationProject>((cursor) =>
      api.get<ApiPage<ApiTranslationProject>>(
        `/translations/projects${cursor ? `?cursor=${encodeURIComponent(cursor)}` : ""}`
      )
    ),
  createTranslationProject: (id: string, name: string) =>
    api.post<ApiTranslationProject>("/translations/projects", { id, name }),
  deleteTranslationProject: (project: string) =>
    api.del<void>(`/translations/projects/${encodeURIComponent(project)}`),

  // A key is the tenant's declaration; the values under it are its translations.
  declareTranslationKey: (project: string, key: string, source: string, plurals?: Record<string, string>) =>
    api.post<void>(`/translations/projects/${encodeURIComponent(project)}/declarations`, {
      key,
      source,
      ...(plurals ? { plurals } : {}),
    }),
  undeclareTranslationKey: (project: string, key: string) =>
    api.del<void>(
      `/translations/projects/${encodeURIComponent(project)}/declarations/${encodeURIComponent(key)}`
    ),

  // Replaces the project's settings whole, so every field is sent every time.
  saveTranslationProject: (project: string, settings: TranslationProjectSettings) =>
    api.put<void>(`/translations/projects/${encodeURIComponent(project)}`, settings as unknown as Record<string, unknown>),

  // Filtering and search are server-side, on the same cursor as every other list.
  listTranslationKeys: (project: string, params: TranslationKeyParams) => {
    const q = new URLSearchParams({ locale: params.locale });
    if (params.state) q.set("state", params.state);
    if (params.namespace) q.set("namespace", params.namespace);
    if (params.q) q.set("q", params.q);
    if (params.limit) q.set("limit", String(params.limit));
    if (params.cursor) q.set("cursor", params.cursor);
    return api.get<ApiPage<ApiTranslationKey>>(
      `/translations/projects/${encodeURIComponent(project)}/keys?${q.toString()}`
    );
  },
  setTranslationValue: (project: string, key: string, locale: string, value: string) =>
    api.put<void>(
      `/translations/projects/${encodeURIComponent(project)}/keys/${encodeURIComponent(key)}`,
      { locale, value }
    ),
  resetTranslationValue: (project: string, key: string, locale: string) =>
    api.del<void>(
      `/translations/projects/${encodeURIComponent(project)}/keys/${encodeURIComponent(key)}?locale=${encodeURIComponent(locale)}`
    ),

  publishTranslationRelease: (project: string) =>
    api.post<ApiTranslationPublished>(`/translations/projects/${encodeURIComponent(project)}/releases`),
  listTranslationReleases: (project: string, cursor?: string | null) =>
    api.get<ApiPage<ApiTranslationRelease>>(
      `/translations/projects/${encodeURIComponent(project)}/releases${cursor ? `?cursor=${encodeURIComponent(cursor)}` : ""}`
    ),
  rollbackTranslationRelease: (project: string, version: number) =>
    api.post<ApiTranslationPublished>(
      `/translations/projects/${encodeURIComponent(project)}/releases/${version}/rollback`
    ),

  /** What a publish would change, per locale. */
  translationDrafts: (project: string) =>
    api.get<ApiDraftDiff>(`/translations/projects/${encodeURIComponent(project)}/drafts`),

  /** The body IS the file. `dryRun` reports without writing; the same call without
   *  it writes exactly what the report said. */
  importTranslations: (
    project: string,
    opts: { locale: string; format: TranslationFormat; dryRun?: boolean; createKeys?: boolean },
    file: string
  ) => {
    const q = new URLSearchParams({ locale: opts.locale, format: opts.format });
    if (opts.dryRun) q.set("dry_run", "1");
    if (opts.createKeys) q.set("create_keys", "1");
    return api.postFile<ApiImportReport>(
      `/translations/projects/${encodeURIComponent(project)}/import?${q}`,
      file,
      opts.format === "json" ? "application/json" : "text/plain"
    );
  },

  /** Where a browser downloads an export from. Same-origin, so the session cookie
   *  rides along and no token has to be handled here. */
  translationExportUrl: (project: string, locale: string, format: TranslationFormat) =>
    `/v1/translations/projects/${encodeURIComponent(project)}/export?` +
    new URLSearchParams({ locale, format }),

  // ── Settings: appearance (§8) — brands saved as one staged set ─────────
  getBrands: () => api.getVersioned<ApiBrands>("/brands"),
  saveBrands: (brands: ApiBrand[], activeBrandId: string, etag: string) =>
    api.putIfMatch<ApiBrands>("/brands", { brands, active_brand_id: activeBrandId }, etag),
};
