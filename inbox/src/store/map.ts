import type {
  ApiConversation,
  ApiEmailChannel,
  ApiKeyRecord,
  ApiMember,
  ApiMessage,
  ApiTeamMember,
  ApiWebhook,
} from "@/api/admin";
import type { ApiMessagesPage, ApiQueueItem } from "@/api/admin";
import type { ApiAssignment } from "@/api/admin";
import type { Channel, Conversation, EmailChannel, Member, Message, ApiKey, TeamMember, Webhook, UiStatus } from "./types";

export function clockTime(iso: string): string {
  const d = new Date(iso);
  if (isNaN(d.getTime())) return "";
  let h = d.getHours();
  const m = String(d.getMinutes()).padStart(2, "0");
  const ap = h >= 12 ? "PM" : "AM";
  h = h % 12 || 12;
  return `${h}:${m} ${ap}`;
}

/** Short relative age for conversation rows ("2m", "3h", "Yesterday"). */
export function relativeTime(iso: string): string {
  const d = new Date(iso);
  const now = Date.now();
  const diff = Math.max(0, now - d.getTime());
  const min = Math.floor(diff / 60000);
  if (min < 1) return "now";
  if (min < 60) return `${min}m`;
  const hr = Math.floor(min / 60);
  if (hr < 24) return `${hr}h`;
  const day = Math.floor(hr / 24);
  if (day === 1) return "Yesterday";
  if (day < 7) return `${day}d`;
  return d.toLocaleDateString(undefined, { day: "numeric", month: "short" });
}

export function shortDate(iso: string): string {
  const d = new Date(iso);
  if (isNaN(d.getTime())) return "";
  return d.toLocaleDateString(undefined, { day: "numeric", month: "short", year: "numeric" });
}

function memberDisplayName(m: ApiMember): string {
  return m.metadata?.name || m.external_user_id || m.internal_actor_id || "Member";
}

export function mapMember(m: ApiMember): Member {
  const meta: Record<string, string> = {};
  for (const [k, v] of Object.entries(m.metadata || {})) {
    if (k === "name") continue; // shown as the title
    meta[k] = String(v);
  }
  return { name: memberDisplayName(m), role: m.conv_role, meta, extId: m.external_user_id };
}

/** Map a realtime message envelope payload (same shape as ApiMessage) using the
 *  already-loaded UI conversation to resolve the author. */
export function mapRealtimeMessage(m: ApiMessage, conv: Conversation): Message {
  const isAgent = m.sender_kind === "agent" || m.sender_kind === "bot" || !!m.internal_actor_id;
  const system = m.sender_kind === "system";
  let author: string | undefined;
  if (!system) {
    author = isAgent
      ? "You"
      : conv.members.find((x) => x.extId && x.extId === m.external_user_id)?.name || m.external_user_id || "User";
  }
  return {
    id: m.id,
    dir: isAgent ? "out" : "in",
    internal: m.visibility === "internal",
    system,
    author,
    time: clockTime(m.created_at),
    body: m.body,
    channel: m.channel,
  };
}

export function mapMessage(m: ApiMessage, conv: ApiConversation): Message {
  const isAgent = m.sender_kind === "agent" || m.sender_kind === "bot" || !!m.internal_actor_id;
  const system = m.sender_kind === "system";
  let author: string | undefined;
  if (!system) {
    if (isAgent) {
      author = "You";
    } else {
      const mem = conv.members.find((x) => x.external_user_id && x.external_user_id === m.external_user_id);
      author = mem ? memberDisplayName(mem) : m.external_user_id || "User";
    }
  }
  return {
    id: m.id,
    dir: isAgent ? "out" : "in",
    internal: m.visibility === "internal",
    system,
    author,
    time: clockTime(m.created_at),
    body: m.body,
    channel: m.channel,
  };
}

function deriveStatus(conv: ApiConversation, a?: ApiAssignment): UiStatus {
  if (conv.status === "closed") return "closed";
  return (a?.status as UiStatus) || "queued";
}

// conversationShell derives the row fields shared by the queue list and the full
// fetch (identity, name, channel, status, members). Each builder fills in the
// parts that differ: history, preview, and last-activity timestamp.
function conversationShell(conv: ApiConversation, a?: ApiAssignment) {
  const client = conv.members.find((m) => m.conv_role === "client") || conv.members[0];
  const name = client ? memberDisplayName(client) : conv.reference || "Conversation";
  const channel = conv.members.some((m) => m.member_kind === "email") ? "email" : "app";
  return {
    id: conv.id,
    name,
    presence: null,
    channel: channel as Channel,
    reference: conv.reference || conv.id,
    status: deriveStatus(conv, a),
    convClosed: conv.status === "closed",
    assignmentId: a?.id,
    assignmentStatus: a?.status as UiStatus | undefined,
    unread: 0,
    dateStarted: conv.created_at,
    waitingSince: a?.created_at ?? conv.created_at,
    members: conv.members.map(mapMember),
  };
}

export function buildConversation(
  conv: ApiConversation,
  page: ApiMessagesPage,
  assignment?: ApiAssignment
): Conversation {
  const a = assignment || conv.assignment;
  const msgs = [...page.messages]
    .sort((x, y) => x.created_at.localeCompare(y.created_at))
    .map((m) => mapMessage(m, conv));
  const last = msgs.filter((m) => !m.system).slice(-1)[0] || msgs.slice(-1)[0];
  const preview = conv.status === "closed" ? "Conversation closed" : last?.body || "";
  const lastTs = page.messages.length ? page.messages[page.messages.length - 1].created_at : conv.created_at;

  return {
    ...conversationShell(conv, a),
    time: relativeTime(lastTs),
    lastActivity: lastTs,
    preview,
    messages: msgs,
  };
}

/** Build a queue row from the paginated list endpoint: members + last-message
 *  preview + last activity, but NO history (messages load on open). */
export function buildQueueRow(item: ApiQueueItem): Conversation {
  const conv = item.conversation;
  const lastTs = conv.last_activity;
  const preview = conv.status === "closed" ? "Conversation closed" : conv.last_message?.body || "";
  return {
    ...conversationShell(conv, item.assignment),
    time: relativeTime(lastTs),
    lastActivity: lastTs,
    preview,
    messages: [],
  };
}

export function mapApiKey(k: ApiKeyRecord): ApiKey {
  return {
    id: k.id,
    label: k.label || "API key",
    masked: `${k.prefix}………`,
    created: shortDate(k.created_at),
  };
}

export function mapWebhook(w: ApiWebhook): Webhook {
  return { id: w.id, url: w.url, events: w.events || [], active: w.active };
}

export function mapTeamMember(t: ApiTeamMember): TeamMember {
  const local = t.email.split("@")[0].replace(/[._-]+/g, " ");
  const name = local.replace(/\b\w/g, (c) => c.toUpperCase());
  return { id: t.id, name: name || t.email, email: t.email, role: t.platform_role };
}

export function mapEmailChannel(c: ApiEmailChannel): EmailChannel {
  return {
    forwardingAddress: c.forwarding_address,
    inboundDomain: c.inbound_domain,
    verified: c.verified,
    autoReply: c.auto_reply,
    spamFilter: c.spam_filter,
    fromName: c.from_name,
    fromAddress: c.from_address,
  };
}
