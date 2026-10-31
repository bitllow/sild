// Egress-only realtime client (§5). Connects to the backend's Centrifuge node
// with a short-lived agent token (minted over REST with the session's credential),
// and surfaces server-side subscription publications. Channels are attached
// server-side — the client declares none.

import { Centrifuge } from "centrifuge";
import { ApiError, connect, realtimeToken, type Envelope, type Realtime, type RealtimeState } from "@sild/client";
import { http, isEmbedded } from "./client";

export type { Envelope, Realtime, RealtimeState };

export interface RealtimeHandlers {
  onEvent: (channel: string, env: Envelope) => void;
  onState?: (state: RealtimeState) => void;
  onConnected?: () => void;
  // The session behind the token is gone; the socket has stopped reconnecting.
  onSignedOut?: () => void;
}

// The browser connects straight to the backend node (a cross-origin WebSocket,
// allowed by the node's CheckOrigin); the cookie can't ride it, so auth is the
// agent token. Override the endpoint with NEXT_PUBLIC_SILD_WS_URL; unset, it is
// port 8080 on the page's own host, so another device on the network works too.
function wsUrl(): string {
  if (process.env.NEXT_PUBLIC_SILD_WS_URL) return process.env.NEXT_PUBLIC_SILD_WS_URL;
  const { protocol, hostname } = window.location;
  return `${protocol === "https:" ? "wss" : "ws"}://${hostname}:8080/v1/ws`;
}

// An embedded bearer is renewed by the host, so only a cookie session ends here.
const sessionEnded = (e: unknown) => e instanceof ApiError && e.status === 401 && !isEmbedded();

export function createRealtime(handlers: RealtimeHandlers): Realtime {
  return connect({
    Centrifuge,
    url: wsUrl(),
    getToken: async () => {
      try {
        return (await realtimeToken(http)).token;
      } catch (e) {
        if (sessionEnded(e)) handlers.onSignedOut?.();
        throw e;
      }
    },
    isUnauthorized: sessionEnded,
    onState: handlers.onState,
    onConnected: handlers.onConnected,
    onEvent: handlers.onEvent,
  });
}
