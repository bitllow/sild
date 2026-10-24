export type Log = (msg: string, fields?: Record<string, unknown>) => void;

// What the session's ops tools do. Each returns the text the model sees as the tool result.
export type Ops = {
  // Backs both reply and ask_for_details.
  reply(text: string): Promise<string>;
  proposeFix(cause: string, fix: string[], evidence: Finding[]): Promise<string>;
  requestInfo(need: string, why: string): Promise<string>;
};

export type SessionEvents = {
  // The session produced output: a turn is running.
  activity(): void;
  // A result message. Several queued inputs may end in one result.
  turnEnd(outcome: string): void;
  // The session process ended.
  exit(error?: string): void;
};

export type SessionHandle = {
  push(text: string): void;
  interrupt(): Promise<void>;
  // Ends the process; the session stays resumable.
  close(): Promise<void>;
};

export interface SessionRunner {
  // Starts the conversation's session, resuming it when one exists.
  open(conv: string, ops: Ops, events: SessionEvents): Promise<SessionHandle>;
  // Conversation closed: delete the transcript and everything on disk for it.
  destroy(conv: string): Promise<void>;
  // Conversations that still have something on disk.
  known(): Promise<string[]>;
}

// One reason behind a diagnosis; ref is for operators only.
export type Finding = { finding: string; source: string; ref?: string };

// request_info calls a session may make in one turn.
export const REQUESTS_PER_TURN = 3;
