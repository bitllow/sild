import type { Http } from './http.ts';
import { catchUp } from './list.ts';
import type {
  ActiveBrand,
  Assignment,
  CloseReason,
  Conversation,
  ConversationPage,
  Message,
  NewConversation,
  NewMessage,
  Page,
  Part,
  Principal,
  RealtimeToken,
  TranslationBundle,
  TranslationManifest,
  UploadGrant,
} from './wire.ts';

const enc = encodeURIComponent;

// GET /v1/conversations filters (spec §4.2), named as on the wire.
export type ConversationQuery = {
  kind?: 'support' | 'peer';
  // Absent means the support queue.
  queue?: string;
  status?: 'open' | 'closed';
  assignment_status?: 'queued' | 'assigned' | 'closed';
  assignee?: string;
  participant?: string;
  role?: string;
  q?: string;
  sort?: 'last_activity' | 'created' | 'waiting_since';
  order?: 'asc' | 'desc';
  expand?: string;
  limit?: number;
  cursor?: string | null;
};

export const principal = (http: Http) => http.get<Principal>('/v1/principal');

// The connection token for an operator or service account; a user connects with their JWT.
export const realtimeToken = (http: Http) => http.get<RealtimeToken>('/v1/realtime/token');

export const listConversations = (http: Http, query: ConversationQuery = {}) =>
  http.get<ConversationPage>('/v1/conversations', { query });

export const getConversation = (http: Http, id: string, opts: { expand?: string } = {}) =>
  http.get<Conversation>(`/v1/conversations/${enc(id)}`, { query: opts });

export const createConversation = (http: Http, body: NewConversation = {}) =>
  http.post<Conversation>('/v1/conversations', body);

export const closeConversation = (http: Http, id: string, reason?: CloseReason) =>
  http.post<{ status: string }>(`/v1/conversations/${enc(id)}/close`, reason ? { reason } : {});

// Pages backward from the newest message; ?since= runs the other way.
export const listMessages = (http: Http, conv: string, opts: { cursor?: string | null; limit?: number } = {}) =>
  http.get<Page<Message>>(`/v1/conversations/${enc(conv)}/messages`, {
    query: { limit: opts.limit ?? 100, cursor: opts.cursor },
  });

// One page of the sync read; messagesSince drains it.
export const messagesSincePage = (http: Http, conv: string, since: string, limit = 100) =>
  http.get<Page<Message>>(`/v1/conversations/${enc(conv)}/messages`, { query: { since, limit } });

export const messagesSince = (http: Http, conv: string, since: string) =>
  catchUp((after) => messagesSincePage(http, conv, after), since);

export const sendMessage = (http: Http, conv: string, msg: NewMessage) =>
  http.post<Message>(`/v1/conversations/${enc(conv)}/messages`, msg);

export const patchMessage = (http: Http, conv: string, id: string, parts: Part[]) =>
  http.patch<Message>(`/v1/conversations/${enc(conv)}/messages/${enc(id)}`, { parts });

export const typing = (http: Http, conv: string) => http.post<void>(`/v1/conversations/${enc(conv)}/typing`);

export const markRead = (http: Http, conv: string, lastReadMessageId: string) =>
  http.post<void>(`/v1/conversations/${enc(conv)}/read`, { last_read_message_id: lastReadMessageId });

// "me" is the only assignee the API accepts.
export const claimAssignment = (http: Http, id: string) =>
  http.patch<Assignment>(`/v1/assignments/${enc(id)}`, { assignee_actor_id: 'me' });

export const closeAssignment = (http: Http, id: string) =>
  http.patch<Assignment>(`/v1/assignments/${enc(id)}`, { status: 'closed' });

// Opens a new queued assignment on the conversation.
export const openAssignment = (http: Http, conv: string) =>
  http.post<Assignment>(`/v1/conversations/${enc(conv)}/assignments`);

// The whole profile (metadata) and/or the reader's locale; an omitted field is left alone.
export const putMyContact = (http: Http, body: { metadata?: Record<string, unknown>; locale?: string }) =>
  http.put<void>('/v1/contacts/me', body);

export const issueUpload = (http: Http, file: { mime_type: string; size_bytes: number; filename: string }) =>
  http.post<UploadGrant>('/v1/uploads', file);

// The local dev backend signs URLs on its configured public origin; route them
// through this client's base instead so uploads work from any host.
export function uploadUrl(http: Http, grant: UploadGrant): { url: string; local: boolean } {
  const at = grant.upload_url.indexOf('/v1/uploads/local/');
  return at < 0 ? { url: grant.upload_url, local: false } : { url: http.baseUrl + grant.upload_url.slice(at), local: true };
}

// With appId the read is anonymous: styling a launcher must not mint a user.
export const activeBrand = (http: Http, appId?: string) =>
  appId
    ? http.get<ActiveBrand>('/v1/brands/active', { query: { app_id: appId }, anonymous: true })
    : http.get<ActiveBrand>('/v1/brands/active');

// null when the held etag still matches.
export async function translationManifest(
  http: Http,
  project: string,
  etag?: string,
): Promise<{ manifest: TranslationManifest; etag: string | null } | null> {
  const res = await http.exchange<TranslationManifest>('GET', '/v1/translations/manifest', {
    query: { project },
    headers: etag ? { 'if-none-match': etag } : undefined,
  });
  return res.status === 304 ? null : { manifest: res.data, etag: res.etag };
}

export const translationBundle = (http: Http, project: string, locale: string, version: number) =>
  http.get<TranslationBundle>('/v1/translations/bundle', { query: { project, locale, version } });
