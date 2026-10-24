// Wire types of the adapter contract (bridge/adapter/openapi.yaml).
import type { Source } from '../../contracts.ts';

export type { Source };

export const LIMITS = {
  summaryBytes: 1024,
  inlineFileBytes: 1024 * 1024,
  // Signed requests older or newer than this are refused.
  clockSkewSeconds: 300,
  maxWaitSeconds: 30,
  instructionsBytes: 8 * 1024,
} as const;

export type Requester = { id: string; display?: string };

export type InfoRequest = {
  id: string;
  requester: Requester;
  conversation_ref: string;
  source: string;
  params: Record<string, unknown>;
  reason: string;
};

// GET /instructions: the integrator's text for the assistant, appended after Sild's own brief.
export type Instructions = { text: string; version?: string };

export type DeliveryState = 'delivered' | 'declined' | 'expired';

export type WireFile = {
  name: string;
  mime: string;
  size: number;
  sha256: string; // hex, over the raw bytes
  inline?: string; // base64
  url?: string;
};

export type Delivery = {
  id: string; // the request id this delivery answers
  state: DeliveryState;
  summary: string;
  files: WireFile[];
  // Why a decline happened, for the bridge to act on; see DECLINE_CODE.
  code?: string;
};

export const DECLINE_CODE = /^[a-z0-9_.-]{1,64}$/;
// The requester must approve data access at the integrator first; the bridge retries once they do.
export const STEP_UP_REQUIRED = 'step_up_required';

// Payload of the compact JWS that carries a delivery.
export type DeliveryClaims = {
  request_id: string;
  conversation_ref: string;
  iat: number;
  exp: number;
  delivery: Delivery;
};

export const SOURCE_NAME = /^[a-z][a-z0-9_]*(\.[a-z][a-z0-9_]*)*$/;
export const REQUEST_ID = /^[A-Za-z0-9_-]{8,64}$/;
