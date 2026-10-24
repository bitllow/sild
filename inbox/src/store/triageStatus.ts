import type { Conversation, Member, Message, Part, Step } from "@sild/client";
import type { Status } from "@/components/ds";

export type TriageStatus =
  | "new"
  | "waiting"
  | "diagnosing"
  | "needs_details"
  | "fix_proposed"
  | "fixed"
  | "closed";

export type TriageConversationLike = Pick<Conversation, "status" | "close_reason" | "assignment" | "members">;

/** Pill colour + label per status, as the design maps them. */
export const TRIAGE_PILL: Record<TriageStatus, { pill: Status; label: string }> = {
  new: { pill: "assigned", label: "New" },
  waiting: { pill: "queued", label: "Waiting" },
  diagnosing: { pill: "assigned", label: "Diagnosing" },
  needs_details: { pill: "queued", label: "Needs details" },
  fix_proposed: { pill: "open", label: "Fix proposed" },
  fixed: { pill: "closed", label: "Fixed" },
  closed: { pill: "closed", label: "Closed" },
};

export const isTriageActive = (s: TriageStatus) => s !== "fixed" && s !== "closed";

/** The person who opened the request: the conversation's client member. */
export function requesterOf(members: Member[]): Member | undefined {
  return members.find((m) => m.conv_role === "client");
}

export function isFromRequester(m: Message, requester: Member | undefined): boolean {
  if (!requester) return m.sender_kind === "user";
  if (requester.internal_actor_id) return m.internal_actor_id === requester.internal_actor_id;
  return !!requester.external_user_id && m.external_user_id === requester.external_user_id;
}

export type DiagnosisPart = Extract<Part, { type: "diagnosis" }>;
export type StepsPart = Extract<Part, { type: "steps" }>;

export function diagnosisOf(m: Message): DiagnosisPart | undefined {
  return m.parts?.find((p): p is DiagnosisPart => p.type === "diagnosis");
}

export function stepsOf(m: Message): StepsPart[] {
  return (m.parts ?? []).filter((p): p is StepsPart => p.type === "steps");
}

/** Data requests parked until the requester approves data access at the integrator. */
export function awaitingApproval(messages: Message[]): Step[] {
  // Steps unchanged since the latest approval wait for the assistant to retry, as the server counts them.
  const at = (iso: string) => new Date(iso).getTime();
  const approvedAt = Math.max(0, ...messages.filter((m) => m.parts?.some((p) => p.type === "approval")).map((m) => at(m.created_at)));
  return messages
    .filter((m) => Math.max(at(m.created_at), m.updated_at ? at(m.updated_at) : 0) > approvedAt)
    .flatMap(stepsOf)
    .flatMap((p) => p.steps)
    .filter((s) => s.state === "needs_approval");
}

/** One data request as operators read it: source plus its parameters. */
export function describeRequest(internal: NonNullable<Step["internal"]>): string {
  const params = Object.entries(internal.params ?? {})
    .map(([k, v]) => `${k}=${typeof v === "string" ? v : JSON.stringify(v)}`)
    .join(", ");
  return params ? `${internal.source} · ${params}` : internal.source;
}

// A steps-only message is the assistant's progress on a turn, not its answer.
function isAnswer(m: Message): boolean {
  return m.sender_kind === "bot" && (!!m.body.trim() || !!diagnosisOf(m));
}

/** The derived status of a triage conversation (no column backs it). */
export function triageStatus(c: TriageConversationLike, messages: Message[]): TriageStatus {
  if (c.status === "closed") return c.close_reason === "resolved" ? "fixed" : "closed";
  const requester = requesterOf(c.members);
  const turns = messages.filter((m) => m.sender_kind !== "system" && m.visibility !== "internal");
  if (!turns.some((m) => isFromRequester(m, requester))) return "new";
  if (c.assignment?.status === "queued") return "waiting";
  let last: Message | undefined;
  for (let i = turns.length - 1; i >= 0 && !last; i--) {
    if (isFromRequester(turns[i], requester) || isAnswer(turns[i])) last = turns[i];
  }
  if (!last || isFromRequester(last, requester)) {
    // Unassigned with the requester's turn pending: a reply requeues it.
    return c.assignment?.status === "assigned" ? "diagnosing" : "waiting";
  }
  return diagnosisOf(last) ? "fix_proposed" : "needs_details";
}
