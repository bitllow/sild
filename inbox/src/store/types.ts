import type { Presence } from "@/components/ds";

export type UiStatus = "queued" | "assigned" | "closed";
export type Channel = "app" | "email";

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
}

export interface Conversation {
  id: string;
  name: string;
  presence: Presence | null;
  channel: Channel;
  reference: string;
  /** Derived UI status: closed if the conversation is closed, else the
   *  assignment status. Drives the status pill / claim / close / composer. */
  status: UiStatus;
  convClosed: boolean;
  assignmentId?: string;
  assignmentStatus?: UiStatus;
  unread: number;
  time: string;
  /** ISO timestamp of the last activity (newest message, or creation time if no
   *  messages). Sortable — drives the default desc-by-activity ordering. */
  lastActivity: string;
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

export type InboxView = "inbox" | "settings";
export type SettingsTab = "keys" | "webhooks" | "team";
export type InboxFilter = "you" | "unassigned" | "closed" | "all";
export type SessionState = "loading" | "authed" | "anon";
