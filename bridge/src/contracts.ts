// Types shared by the bridge's two halves: the Sild side (sild/, slots/,
// runner/) and the data side (adapter/, broker/, router/).

// One catalog entry an adapter offers (adapter contract GET /catalog).
export type Source = {
  source: string;
  description: string;
  params: Record<string, unknown>; // JSON Schema (draft-07) for the request params
  max_wait_seconds: number;
};

export type DeliveredFile = {
  // Absolute path under /work/<conv>/data/, named by the bridge, never by the adapter.
  path: string;
  name: string; // the adapter's name, display only
  mime: string;
  size: number;
};

// What the info broker reports. The Sild side turns these into steps-part
// updates and session wake-ups; the data side never talks to Sild or the session.
export type BrokerEvent =
  // Again for the same id: a parked request is being retried.
  | { type: 'requested'; conv: string; requestId: string; label: string; source: string; params: Record<string, unknown> }
  // Waits for the requester to approve data access; retried once they do.
  | { type: 'parked'; conv: string; requestId: string; reason: string }
  | { type: 'delivered'; conv: string; requestId: string; summary: string; files: DeliveredFile[] }
  | { type: 'not_available'; conv: string; requestId: string; reason: string };

// The sources the session may ask for, and how much one conversation may ask.
export type DataCatalog = { sources: Source[]; perConversation: number; distinctIds: number };

// The integrator's text for the assistant (adapter contract GET /instructions); `from` is the adapter's host.
export type IntegratorInstructions = { text: string; version?: string; from: string };

export interface InfoBroker {
  // Router pass over requester-authored text. Returns the text the session may
  // see (phones/emails replaced by params); fetch outcomes arrive as events.
  onRequesterText(conv: string, requester: string, text: string): Promise<{ text: string }>;
  // The session's request_info tool. Returns at once; never waits for data.
  request(conv: string, requester: string, need: string, why: string): Promise<{ requestId: string } | { notAvailable: string }>;
  subscribe(cb: (e: BrokerEvent) => void): () => void;
  // Conversation closed: stop waiting on its requests, delete its files.
  forget(conv: string): Promise<void>;
  // Null when the integrator has none or they cannot be had. Never throws.
  instructions(): Promise<IntegratorInstructions | null>;
  // No sources when there are none or the catalog cannot be had. Never throws.
  catalog(): Promise<DataCatalog>;
  // The requester approved data access: retry what was parked for it. Resolves to how many.
  approved(conv: string): Promise<number>;
}
