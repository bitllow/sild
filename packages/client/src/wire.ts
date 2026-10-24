// JSON shapes as the backend renders them (backend/internal/views and the api
// handlers that extend those views). Field names are the wire's own.

export type SenderKind = 'user' | 'agent' | 'bot' | 'system';
export type Visibility = 'participants' | 'internal';
export type Channel = 'app' | 'email';
export type MemberKind = 'user' | 'agent' | 'bot' | 'email';
export type ConversationStatus = 'open' | 'closed';
export type ConversationKind = 'support' | 'peer';
export type CloseReason = 'resolved' | 'closed';
export type AssignmentStatus = 'queued' | 'assigned' | 'closed';
export type PlatformRole = 'owner' | 'admin' | 'agent' | 'translator' | 'requester';

// Message ids are prefixed ULIDs; the zero ULID sorts before every message.
export const BEFORE_FIRST_MESSAGE = 'm_00000000000000000000000000';

export type Page<T> = { items: T[]; next_cursor: string | null; has_more: boolean };

export type ErrorBody = {
  error: { code: string; message: string; request_id?: string; fields?: Record<string, string> };
};

export type Assignment = {
  id: string;
  conversation_id: string;
  status: AssignmentStatus;
  created_at: string;
  assignee_actor_id?: string;
  closed_at?: string | null;
};

export type Member = {
  member_kind: MemberKind;
  // Tenant-defined: "client", "support", "rider", "driver", …
  conv_role: string;
  joined_at: string;
  name?: string;
  external_user_id?: string;
  internal_actor_id?: string;
  // A requester's identity at the tenant's integrator; adapters authorize on it.
  external_sub?: string;
};

// One entry of the `expand=contacts.metadata` block.
export type ExpandedContact = {
  external_user_id: string;
  name?: string;
  metadata?: Record<string, unknown> | null;
};

export type Conversation = {
  id: string;
  status: ConversationStatus;
  reference: string;
  metadata: Record<string, unknown> | null;
  created_at: string;
  queue?: string;
  kind?: ConversationKind;
  close_reason?: CloseReason | null;
  members: Member[];
  assignment?: Assignment;
  // Email conversations only.
  subject?: string;
  // History is served from cold storage.
  archived?: boolean;
  contacts?: ExpandedContact[];
};

// A row of GET /v1/conversations.
export type ConversationRow = Conversation & {
  last_activity: string;
  last_message?: { body: string; created_at: string | null };
  agent_name?: string;
  unread_count?: number;
  // Present only on rows returned for a ?q= query.
  snippet?: string;
};

export type ConversationPage = Page<ConversationRow> & {
  contacts?: ExpandedContact[];
  // Operator support queue only; closed counts closed conversations.
  counts?: { open: number; you: number; unassigned: number; closed: number };
};

export type StepState = 'running' | 'done' | 'not_available';

export type Step = {
  label: string;
  result?: string;
  state: StepState;
  // Stripped for readers who may not see internal notes.
  internal?: { source: string; params?: Record<string, unknown> | null };
};

export type Part =
  | { type: 'steps'; steps: Step[] }
  | { type: 'diagnosis'; cause: string; fix: string[] };

export type Disposition = 'inline' | 'attachment';

export type Attachment = {
  object_key: string;
  disposition: Disposition;
  mime_type: string;
  size_bytes: number;
  filename: string;
  url?: string;
};

export type AttachmentRef = { object_key: string; disposition: Disposition };

export type Message = {
  id: string;
  conversation_id: string;
  sender_kind: SenderKind;
  visibility: Visibility;
  channel: Channel;
  body: string;
  created_at: string;
  external_user_id?: string;
  internal_actor_id?: string;
  client_msg_id?: string;
  // The operator's display name on agent-authored messages.
  author_name?: string;
  attachments: Attachment[];
  parts?: Part[] | null;
  updated_at?: string;
};

export type NewMessage = {
  body: string;
  // Idempotency key: a resend with the same id returns the first message.
  client_msg_id?: string;
  visibility?: Visibility;
  attachments?: AttachmentRef[];
  parts?: Part[];
};

export type NewConversation = {
  queue?: string;
  open_assignment?: boolean;
  reference?: string;
  metadata?: Record<string, unknown>;
  external_user_id?: string;
  members?: { user_id: string; conv_role: string }[];
};

export type RoleScope = {
  peer?: boolean;
  queues?: string[];
  projects?: string[];
  locales?: string[];
  publish?: boolean;
};

export type RoleAssignment = { role: PlatformRole; scope: RoleScope };

export type Grant = {
  action: string;
  scope?: {
    kinds?: ConversationKind[];
    requires_assignment?: boolean;
    participant?: string;
    queues?: string[];
    member_only?: boolean;
    own_queues?: string[];
  };
};

export type Principal = {
  kind: 'admin' | 'user' | 'apikey';
  tenant_id: string;
  subject?: {
    id: string;
    assignments?: RoleAssignment[];
    email?: string;
    first_name?: string;
    last_name?: string;
  };
  grants?: Grant[];
};

export type RealtimeToken = { token: string; expires_at: string };

export type UploadGrant = { object_key: string; upload_url: string; expires_at: string };

// Mirrors domain.BrandConfig.
export type BrandConfig = {
  // Bucket object key (or a legacy data:/http URL); render logoUrl.
  logo: string;
  logoUrl?: string;
  brand: string;
  theme: 'light' | 'dark' | 'auto';
  font: 'system' | 'sild' | 'inter' | 'figtree' | 'dmsans';
  radius: 'sharp' | 'default' | 'rounded' | 'pillowy';
  launcherIcon: 'chat' | 'message' | 'help' | 'sparkle' | 'custom';
  iconImg: string;
  iconImgUrl?: string;
  launcherPos: 'left' | 'right';
  launcherSize: 'sm' | 'md' | 'lg';
  heading: string;
  sub: string;
  topics: string;
  showTeam: boolean;
  poweredBy: boolean;
};

export type ActiveBrand = { name: string; config: Partial<BrandConfig> };
export type Brand = { id: string; name: string; config: BrandConfig };
export type Brands = { brands: Brand[]; active_brand_id: string };

export type TranslationManifest = {
  project: string;
  fallback_locale: string;
  locales: Record<string, number>;
};

export type TranslationBundle = {
  project: string;
  locale: string;
  version: number;
  strings: Record<string, string>;
};

export type EventType =
  | 'message.created'
  | 'message.updated'
  | 'message.read'
  | 'member.added'
  | 'member.removed'
  | 'assignment.updated'
  | 'conversation.closed'
  | 'typing'
  | 'contact.updated';

// `type` stays open: an older client must tolerate events added later.
export type Envelope = {
  type: EventType | (string & {});
  conversation_id?: string;
  data: unknown;
  ts: number;
};
