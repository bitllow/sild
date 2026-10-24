import type { Source } from '../contracts.ts';

export type Proposal = {
  source: string;
  params: Record<string, unknown>;
  // Plain words for the requester, e.g. "Checked his phone registration".
  label: string;
};

export type RouteInput = {
  // Requester-authored text, or the session's request_info need. Never
  // delivered data, Confluence or code.
  text: string;
  catalog: Source[];
  // Sources already delivered in this conversation.
  delivered: string[];
  max: number;
};

export type RouteOutput = { requests: Proposal[] };

export interface Router {
  route(input: RouteInput): Promise<RouteOutput>;
}
