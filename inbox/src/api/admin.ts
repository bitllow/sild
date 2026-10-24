// Typed bindings for the admin-only Sild API (§4.3). Shapes mirror
// internal/api/admin.go on the Go side; the shared wire shapes and endpoints
// come from @sild/client.

import {
  collectAll,
  listConversations,
  type Brand,
  type Brands,
  type ConversationQuery,
  type ConversationRow,
  type ExpandedContact,
  type Page,
  type PlatformRole,
  type RoleAssignment,
  type RoleScope,
} from "@sild/client";
import { http, WITH_PROFILES } from "./client";

const enc = encodeURIComponent;

// last_activity (default), created = date started, waiting_since = queued-since.
export type QueueSort = NonNullable<ConversationQuery["sort"]>;
export type QueueOrder = NonNullable<ConversationQuery["order"]>;

export interface ApiKeyRecord {
  id: string;
  label: string;
  prefix: string;
  created_at: string;
  revoked_at?: string | null;
  /** The service account the key signs in as; null for a server key. */
  actor_id?: string | null;
}

export interface ApiKeyCreated {
  id: string;
  key: string;
  label: string;
  prefix: string;
  actor_id?: string | null;
}

export interface ApiWebhook {
  id: string;
  url: string;
  events: string[];
  active: boolean;
  created_at: string;
}

export interface ApiQueue {
  id: string;
  key: string;
  label: string;
  requeue_on_reply: boolean;
  webhooks: boolean;
  /** The support queue: always present, not editable. */
  built_in: boolean;
}

export type QueueSettings = Pick<ApiQueue, "label" | "requeue_on_reply" | "webhooks">;

export interface ApiTeamMember {
  id: string;
  /** A service account has no login; its email field carries its name. */
  kind?: "human" | "service";
  email: string;
  first_name?: string;
  last_name?: string;
  /** Every role this member holds, each with its scope. */
  assignments: RoleAssignment[];
  has_password: boolean;
  /** The member's identity at the integrator, for embedded sign-in. */
  external_sub?: string | null;
  created_at: string;
}

/** A role as the Team screen renders it: what it is for, and what it scopes. */
export interface ApiRoleDefinition {
  role: PlatformRole;
  label: string;
  description: string;
  dimensions: {
    key: keyof RoleScope;
    kind: "set" | "toggle";
    label: string;
    help: string;
    source?: string;
  }[];
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

// Embedded sign-in config. The validator token is write-only: a read only says
// whether one is stored.
export interface ApiIdentityProvider {
  embed_key: string;
  validator_url: string;
  has_validator_token: boolean;
  allowed_origins: string[];
  embedded_only: boolean;
  updated_at: string;
}

export interface IdentityProviderInput {
  validator_url: string;
  /** Omit to keep the stored token; only allowed while the URL is unchanged. */
  validator_token?: string;
  allowed_origins: string[];
  embedded_only: boolean;
}

export interface ApiEmbeddedSession {
  token: string;
  expires_at: string;
  external_sub: string;
  admin_id: string;
}

// ── Auth ──────────────────────────────────────────────────────────────────
export const adminApi = {
  loginPassword: (email: string, password: string) =>
    http.post<{ status: string; expires_at: string }>("/v1/admin/auth/password", { email, password }),
  logout: () => http.post<void>("/v1/admin/auth/logout", {}),
  googleLoginUrl: () => "/v1/admin/auth/google",
  // The embedded inbox trades the host's user token for a session bearer; no
  // cookie rides either call.
  exchangeEmbedded: (embedKey: string, token: string) =>
    http.post<ApiEmbeddedSession>("/v1/admin/auth/embedded", { embed_key: embedKey, token }, { anonymous: true }),
  embedOrigins: (embedKey: string) =>
    http.get<{ allowed_origins: string[] }>(`/v1/admin/auth/embedded/${enc(embedKey)}`, { anonymous: true }),
  // Names the bearer explicitly: it may no longer be the client's credential,
  // and keepalive lets the call outlive an unloading frame.
  endEmbedded: (token: string) =>
    http.post<void>("/v1/admin/auth/logout", {}, {
      anonymous: true,
      headers: { authorization: `Bearer ${token}` },
      keepalive: true,
    }),

  // ── Conversations ─────────────────────────────────────────────────────
  // Drains the cursor: the Details-panel history and the contact filter render a
  // whole list, with no scroll affordance to continue from.
  listAllConversations: async (query: ConversationQuery) => {
    // collectAll keeps only the items, so the per-page contacts blocks are
    // re-read here and merged into one lookup for the whole drained list.
    const contacts: ExpandedContact[] = [];
    const items = await collectAll<ConversationRow>(async (cursor) => {
      const page = await listConversations(http, { ...query, expand: WITH_PROFILES, cursor });
      contacts.push(...(page.contacts || []));
      return page;
    });
    return { items, contacts };
  },

  // ── Contacts (people, not threads) ────────────────────────────────────
  listContacts: (q?: string) => http.get<Page<ApiContact>>("/v1/contacts", { query: { q } }),

  // ── Settings: API keys ────────────────────────────────────────────────
  // Settings screens render a whole collection, so they drain the cursor.
  listApiKeys: () => collectAll<ApiKeyRecord>((cursor) => http.get("/v1/api-keys", { query: { cursor } })),
  createApiKey: (label: string) => http.post<ApiKeyCreated>("/v1/api-keys", { label }),
  revokeApiKey: (id: string) => http.del<void>(`/v1/api-keys/${id}`),

  // ── Settings: webhooks ────────────────────────────────────────────────
  listWebhooks: () => collectAll<ApiWebhook>((cursor) => http.get("/v1/webhooks", { query: { cursor } })),
  setWebhookActive: (id: string, active: boolean) => http.patch<void>(`/v1/webhooks/${id}`, { active }),
  deleteWebhook: (id: string) => http.del<void>(`/v1/webhooks/${id}`),

  // ── Settings: team ────────────────────────────────────────────────────
  listTeam: () => collectAll<ApiTeamMember>((cursor) => http.get("/v1/team", { query: { cursor } })),
  listRoles: () => http.get<{ roles: ApiRoleDefinition[] }>("/v1/roles"),
  inviteMember: (member: {
    email: string;
    first_name: string;
    last_name: string;
    role: PlatformRole;
    scope: RoleScope;
  }) => http.post<ApiTeamMember>("/v1/team", member),
  assignRole: (id: string, role: PlatformRole, scope: RoleScope) =>
    http.post<void>(`/v1/team/${id}/roles`, { role, scope }),
  setRoleScope: (id: string, role: PlatformRole, scope: RoleScope) =>
    http.put<void>(`/v1/team/${id}/roles/${role}`, { scope }),
  removeRole: (id: string, role: PlatformRole) => http.del<void>(`/v1/team/${id}/roles/${role}`),
  // Empty unlinks. 409 external_sub_taken when another member holds it.
  setExternalSub: (id: string, externalSub: string) =>
    http.put<void>(`/v1/team/${id}/external-sub`, { external_sub: externalSub }),

  // ── Settings: embedded sign-in ────────────────────────────────────────
  getIdentityProvider: () => http.get<ApiIdentityProvider>("/v1/identity-provider"),
  putIdentityProvider: (input: IdentityProviderInput) => http.put<ApiIdentityProvider>("/v1/identity-provider", input),
  deleteIdentityProvider: () => http.del<void>("/v1/identity-provider"),

  // ── Settings: service accounts and queues ─────────────────────────────
  createServiceAccount: (account: {
    name: string;
    display_name: string;
    role: PlatformRole;
    scope: RoleScope;
  }) => http.post<ApiTeamMember>("/v1/service-accounts", account),
  createServiceKey: (id: string, label: string) =>
    http.post<ApiKeyCreated>(`/v1/service-accounts/${id}/keys`, { label }),
  listQueues: () => collectAll<ApiQueue>((cursor) => http.get("/v1/queues", { query: { cursor } })),
  // Creates the queue when the key is new; replaces its settings otherwise.
  putQueue: (key: string, settings: QueueSettings) => http.put<ApiQueue>(`/v1/queues/${enc(key)}`, settings),

  // ── Settings: channels (§6.2) ─────────────────────────────────────────
  // Read returns the version; the write quotes it back, so a concurrent edit
  // fails loudly (412) instead of being overwritten.
  getEmailChannel: () => http.exchange<ApiEmailChannel>("GET", "/v1/channels/email"),
  updateEmailChannel: (patch: EmailChannelPatch, etag: string) =>
    http.exchange<ApiEmailChannel>("PATCH", "/v1/channels/email", { body: patch, headers: { "if-match": etag } }),

  // ── Settings: push (§5.5) ─────────────────────────────────────────────
  getPushChannel: () => http.get<ApiPushChannel>("/v1/channels/push"),
  updatePushSettings: (patch: PushSettingsPatch) => http.patch<void>("/v1/channels/push", patch),
  // The credential is checked against the provider before it is stored, so a
  // rejection here means the credential, not the network.
  setPushCredential: (credential: unknown) => http.put<void>("/v1/channels/push/credential", { credential }),
  deletePushCredential: () => http.del<void>("/v1/channels/push/credential"),
  testPushSend: (token: string) => http.post<void>("/v1/channels/push/test", { token }),

  // ── Translations ──────────────────────────────────────────────────────
  listTranslationProjects: () =>
    collectAll<ApiTranslationProject>((cursor) => http.get("/v1/translations/projects", { query: { cursor } })),
  createTranslationProject: (id: string, name: string) =>
    http.post<ApiTranslationProject>("/v1/translations/projects", { id, name }),
  deleteTranslationProject: (project: string) => http.del<void>(`/v1/translations/projects/${enc(project)}`),

  // A key is the tenant's declaration; the values under it are its translations.
  declareTranslationKey: (project: string, key: string, source: string) =>
    http.post<void>(`/v1/translations/projects/${enc(project)}/declarations`, { key, source }),
  undeclareTranslationKey: (project: string, key: string) =>
    http.del<void>(`/v1/translations/projects/${enc(project)}/declarations/${enc(key)}`),

  // Replaces the project's settings whole, so every field is sent every time.
  saveTranslationProject: (project: string, settings: TranslationProjectSettings) =>
    http.put<void>(`/v1/translations/projects/${enc(project)}`, settings),

  // Filtering and search are server-side, on the same cursor as every other list.
  listTranslationKeys: (project: string, params: TranslationKeyParams) =>
    http.get<Page<ApiTranslationKey>>(`/v1/translations/projects/${enc(project)}/keys`, { query: { ...params } }),
  setTranslationValue: (project: string, key: string, locale: string, value: string) =>
    http.put<void>(`/v1/translations/projects/${enc(project)}/keys/${enc(key)}`, { locale, value }),
  resetTranslationValue: (project: string, key: string, locale: string) =>
    http.del<void>(`/v1/translations/projects/${enc(project)}/keys/${enc(key)}`, undefined, { query: { locale } }),

  publishTranslationRelease: (project: string) =>
    http.post<ApiTranslationPublished>(`/v1/translations/projects/${enc(project)}/releases`, {}),
  listTranslationReleases: (project: string, cursor?: string | null) =>
    http.get<Page<ApiTranslationRelease>>(`/v1/translations/projects/${enc(project)}/releases`, { query: { cursor } }),
  rollbackTranslationRelease: (project: string, version: number) =>
    http.post<ApiTranslationPublished>(`/v1/translations/projects/${enc(project)}/releases/${version}/rollback`, {}),

  // ── Settings: appearance (§8) — brands saved as one staged set ─────────
  getBrands: () => http.exchange<Brands>("GET", "/v1/brands"),
  saveBrands: (brands: Brand[], activeBrandId: string, etag: string) =>
    http.exchange<Brands>("PUT", "/v1/brands", {
      body: { brands, active_brand_id: activeBrandId },
      headers: { "if-match": etag },
    }),
};
