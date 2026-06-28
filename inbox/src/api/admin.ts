// Typed bindings for the Sild admin API (§4.3). Shapes mirror internal/views
// and internal/api/admin.go on the Go side.

import { api } from "./client";

export type ApiAssignmentStatus = "queued" | "assigned" | "closed";
export type ApiConvStatus = "open" | "closed";
export type ApiSenderKind = "user" | "agent" | "bot" | "system";
export type ApiVisibility = "participants" | "internal";
export type ApiChannel = "app" | "email";
export type ApiMemberKind = "user" | "agent" | "bot" | "email";
export type ApiConvRole = "dispatcher" | "client" | "driver" | "agent";
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
  attachments: unknown[];
}

export interface ApiMessagesPage {
  messages: ApiMessage[];
  has_more?: boolean;
}

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

export interface ApiSearchHit {
  conversation_id: string;
  snippet?: string;
  score?: number;
}

export interface ApiTeamMember {
  id: string;
  email: string;
  platform_role: ApiPlatformRole;
  has_password: boolean;
  created_at: string;
}

// ── Auth ──────────────────────────────────────────────────────────────────
export const adminApi = {
  loginPassword: (email: string, password: string) =>
    api.post<{ status: string; expires_at: string }>("/admin/auth/password", { email, password }),
  logout: () => api.post<void>("/admin/auth/logout"),
  googleLoginUrl: () => "/v1/admin/auth/google",

  // ── Inbox queue ───────────────────────────────────────────────────────
  listAssignments: (params?: { status?: ApiAssignmentStatus; assignee?: string }) => {
    const q = new URLSearchParams();
    if (params?.status) q.set("status", params.status);
    if (params?.assignee) q.set("assignee", params.assignee);
    const qs = q.toString();
    return api.get<ApiAssignment[]>(`/admin/assignments${qs ? `?${qs}` : ""}`);
  },
  getConversation: (id: string) => api.get<ApiConversation>(`/conversations/${id}`),
  listMessages: (id: string) => api.get<ApiMessagesPage>(`/conversations/${id}/messages?limit=100`),
  postMessage: (id: string, body: string, visibility: ApiVisibility = "participants") =>
    api.post<ApiMessage>(`/conversations/${id}/messages`, { body, visibility }),
  claimAssignment: (id: string) => api.post<ApiAssignment>(`/admin/assignments/${id}/claim`),
  closeConversation: (id: string) => api.post<{ status: string }>(`/conversations/${id}/close`),

  // ── Search (§4.3) — mixed tokens: field:value filters + free keywords ──
  search: (q: string) =>
    api.get<{ conversations: ApiSearchHit[] }>(`/admin/search?q=${encodeURIComponent(q)}`),

  // ── Settings: API keys ────────────────────────────────────────────────
  listApiKeys: () => api.get<ApiKeyRecord[]>("/admin/api-keys"),
  createApiKey: (label: string) => api.post<ApiKeyCreated>("/admin/api-keys", { label }),
  revokeApiKey: (id: string) => api.del<void>(`/admin/api-keys/${id}`),

  // ── Settings: webhooks ────────────────────────────────────────────────
  listWebhooks: () => api.get<ApiWebhook[]>("/admin/webhooks"),
  setWebhookActive: (id: string, active: boolean) =>
    api.patch<void>(`/admin/webhooks/${id}`, { active }),
  deleteWebhook: (id: string) => api.del<void>(`/admin/webhooks/${id}`),

  // ── Settings: team ────────────────────────────────────────────────────
  listTeam: () => api.get<ApiTeamMember[]>("/admin/team"),
  setTeamRole: (id: string, role: ApiPlatformRole) =>
    api.patch<void>(`/admin/team/${id}`, { platform_role: role }),
};
