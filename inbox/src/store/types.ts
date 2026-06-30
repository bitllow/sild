import type { MessageAttachment, Presence } from "@/components/ds";

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

export type PlatformRole = "owner" | "admin" | "agent";

export interface TeamMember {
  id: string;
  name: string;
  email: string;
  role: PlatformRole;
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

export type InboxView = "inbox" | "settings";
export type SettingsTab = "keys" | "webhooks" | "team" | "channels";
export type InboxFilter = "you" | "unassigned" | "closed" | "all";
export type SessionState = "loading" | "authed" | "anon";
