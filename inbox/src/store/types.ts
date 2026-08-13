import type { MessageAttachment, Presence } from "@/components/ds";
import type {
  ApiBrandConfig,
  ApiPlatformRole,
  ApiRoleAssignment,
  ApiRoleDefinition,
  ApiRoleScope,
} from "@/api/admin";

export type UiStatus = "queued" | "assigned" | "closed";
export type Channel = "app" | "email";

/** A file uploaded and ready to attach to the next outgoing message. */
export interface PendingAttachment {
  objectKey: string;
  disposition: "inline" | "attachment";
  mimeType: string;
  filename: string;
}

export interface Member {
  name: string;
  role: string;
  meta: Record<string, string>;
  extId?: string;
}

export interface Message {
  id: string;
  dir?: "in" | "out";
  internal?: boolean;
  system?: boolean;
  author?: string;
  time?: string;
  body: string;
  channel?: Channel;
  read?: string;
  attachments?: MessageAttachment[];
}

export interface Conversation {
  id: string;
  name: string;
  presence: Presence | null;
  channel: Channel;
  reference: string;
  /** Email subject (email channel only) — shown in the header instead of the
   *  opaque conversation id. Undefined for app conversations. */
  subject?: string;
  /** Derived UI status: closed if the conversation is closed, else the
   *  assignment status. Drives the status pill / claim / close / composer. */
  status: UiStatus;
  convClosed: boolean;
  assignmentId?: string;
  assignmentStatus?: UiStatus;
  unread: number;
  time: string;
  /** ISO timestamps for the three inbox sort keys (all sortable strings):
   *  lastActivity = newest message (or creation time); dateStarted = conversation
   *  creation; waitingSince = current assignment/queue-entry time. Carried per row
   *  so realtime arrivals re-sort client-side without a refetch. */
  lastActivity: string;
  dateStarted: string;
  waitingSince: string;
  preview: string;
  members: Member[];
  messages: Message[];
  /** Cursor for the next page of OLDER messages; null when the thread is whole. */
  olderCursor?: string | null;
}

export interface ApiKey {
  id: string;
  label: string;
  masked: string;
  created: string;
  /** What the key reaches, in words — "everything" for an unscoped one. */
  reach: string;
}

export interface Webhook {
  id: string;
  url: string;
  events: string[];
  active: boolean;
}

export type PlatformRole = ApiPlatformRole;

/** The limits one assignment carries, and the role catalogue that describes
 *  them. Identical on the wire, so the API types are these types. */
export type RoleScope = ApiRoleScope;
export type RoleAssignment = ApiRoleAssignment;
export type RoleDefinition = ApiRoleDefinition;
export type RoleDimension = ApiRoleDefinition["dimensions"][number];

export interface TeamMember {
  id: string;
  name: string;
  email: string;
  assignments: RoleAssignment[];
}

/** The email support channel as the Channels settings render it (§6.2). */
export interface EmailChannel {
  forwardingAddress: string;
  inboundDomain: string;
  verified: boolean;
  autoReply: boolean;
  spamFilter: boolean;
  fromName: string;
  fromAddress: string;
}

/** One brand — a named messenger look edited in Settings → Appearance (§8). */
export type BrandConfig = ApiBrandConfig;
export interface Brand {
  id: string;
  name: string;
  config: BrandConfig;
}

export type InboxView = "inbox" | "peer" | "settings" | "translations";
export type SettingsTab = "installation" | "channels" | "appearance" | "keys" | "webhooks" | "team";
export type InboxFilter = "you" | "unassigned" | "all";
export type SessionState = "loading" | "authed" | "anon";
