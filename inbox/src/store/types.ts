import type { MessageAttachment, Presence } from "@/components/ds";
import type { ApiBrandConfig } from "@/api/admin";

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
}

export interface Webhook {
  id: string;
  url: string;
  events: string[];
  active: boolean;
}

export type PlatformRole = "owner" | "admin" | "agent" | "translator";

/** The limits one assignment carries. Which of them a role has is the role's
 *  own declaration, served by the backend. */
export interface RoleScope {
  peer?: boolean;
  projects?: string[];
  locales?: string[];
  publish?: boolean;
}

/** One role a member holds, with that role's scope. */
export interface RoleAssignment {
  role: PlatformRole;
  scope: RoleScope;
}

export interface TeamMember {
  id: string;
  name: string;
  email: string;
  assignments: RoleAssignment[];
}

/** One limit a role's scope may carry, as the backend describes it. */
export interface RoleDimension {
  key: keyof RoleScope;
  kind: "set" | "toggle";
  label: string;
  help: string;
  /** Where the options of a set come from — the screen fetches them itself. */
  source?: string;
}

/** A role as the Team screen renders it: what it is for, and what it scopes. */
export interface RoleDefinition {
  role: PlatformRole;
  label: string;
  description: string;
  dimensions: RoleDimension[];
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
