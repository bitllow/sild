// Egress-only realtime client (§5). Connects to the backend's Centrifuge node
// with a short-lived agent token (minted via the cookie-authed REST endpoint),
// and surfaces server-side subscription publications. Channels are attached
// server-side — the client declares none.

import { Centrifuge } from "centrifuge";

export interface RealtimeEnvelope {
  type: string;
  conversation_id?: string;
  data: unknown;
  ts: number;
}

export type RealtimeState = "connecting" | "connected" | "disconnected";

export interface RealtimeHandlers {
  onEvent: (channel: string, env: RealtimeEnvelope) => void;
  onState?: (state: RealtimeState) => void;
}

// The browser connects straight to the backend node (a cross-origin WebSocket,
// allowed by the node's CheckOrigin); the cookie can't ride it, so auth is the
// agent token. Override the endpoint with NEXT_PUBLIC_SILD_WS_URL.
const WS_URL = process.env.NEXT_PUBLIC_SILD_WS_URL || "ws://localhost:8080/v1/ws";

export function createRealtime(handlers: RealtimeHandlers): Centrifuge {
  const client = new Centrifuge(WS_URL, {
    getToken: async () => {
      const r = await fetch("/v1/admin/realtime/token", { credentials: "include" });
      if (!r.ok) throw new Error("realtime token request failed");
      const d = (await r.json()) as { token: string };
      return d.token;
    },
  });

  client.on("connected", () => handlers.onState?.("connected"));
  client.on("connecting", () => handlers.onState?.("connecting"));
  client.on("disconnected", () => handlers.onState?.("disconnected"));
  // server-side subscription publications arrive on the client itself
  client.on("publication", (ctx) => {
    handlers.onEvent(ctx.channel, ctx.data as RealtimeEnvelope);
  });

  return client;
}
