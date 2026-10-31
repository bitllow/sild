import { ApiError } from './http.ts';
import type { Envelope } from './wire.ts';

// The slice of the centrifuge client this module drives. The caller passes the
// constructor in: a linked package cannot resolve the consumer's centrifuge.
export type CentrifugeOptions = {
  getToken?: (ctx: unknown) => Promise<string>;
  minReconnectDelay?: number;
  maxReconnectDelay?: number;
  [option: string]: unknown;
};

export type CentrifugeClient = {
  on(event: 'connecting' | 'disconnected', cb: (ctx: { code: number; reason: string }) => void): unknown;
  on(event: 'connected', cb: (ctx: { transport: string }) => void): unknown;
  on(event: 'error', cb: (ctx: { type: string; error: { code: number; message: string } }) => void): unknown;
  on(event: 'publication', cb: (ctx: { channel: string; data: unknown }) => void): unknown;
  connect(): void;
  disconnect(): void;
};

export type CentrifugeConstructor = {
  new (endpoint: string | { transport: 'websocket' | 'sse'; endpoint: string }[], options?: CentrifugeOptions): CentrifugeClient;
  UnauthorizedError: new (message: string) => Error;
};

export type RealtimeState = 'connecting' | 'connected' | 'disconnected';

export type RealtimeOptions = {
  Centrifuge: CentrifugeConstructor;
  // websocket: ws(s)://…/v1/ws (wsUrl); sse: …/v1/ws/sse (sseUrl).
  url: string;
  transport?: 'websocket' | 'sse';
  // Called whenever centrifuge needs a fresh connection token.
  getToken: () => Promise<string>;
  // Every (re)connect: run the REST catch-up here, the socket replays nothing.
  // `reconnect` is false on the first connect of this handle.
  onConnected?: (info: { reconnect: boolean }) => void;
  onEvent: (channel: string, env: Envelope) => void;
  onState?: (state: RealtimeState) => void;
  // Errors that stop reconnecting; by default a 401 or 403 from getToken.
  isUnauthorized?: (e: unknown) => boolean;
  log?: (msg: string, fields?: Record<string, unknown>) => void;
  // Passed through to centrifuge (e.g. emulationEndpoint, debug).
  options?: CentrifugeOptions;
};

export type Realtime = {
  close(): void;
  // Drops and reopens the connection so the server re-derives its channels.
  reconnect(): void;
};

export function wsUrl(baseUrl: string): string {
  const u = new URL('/v1/ws', baseUrl);
  u.protocol = u.protocol === 'https:' ? 'wss:' : 'ws:';
  return u.toString();
}

export function sseUrl(baseUrl: string): string {
  return baseUrl.replace(/\/+$/, '') + '/v1/ws/sse';
}

export function parseEnvelope(data: unknown): Envelope | null {
  if (typeof data !== 'object' || data === null) return null;
  const env = data as Partial<Envelope>;
  if (typeof env.type !== 'string') return null;
  return { type: env.type, conversation_id: env.conversation_id, data: env.data, ts: Number(env.ts) || 0 };
}

const unauthorized = (e: unknown) => e instanceof ApiError && (e.status === 401 || e.status === 403);

// Egress-only: channels are attached server-side, the client subscribes to nothing.
export function connect(o: RealtimeOptions): Realtime {
  const log = o.log ?? (() => {});
  const isUnauthorized = o.isUnauthorized ?? unauthorized;
  const endpoint = o.transport === 'sse' ? [{ transport: 'sse' as const, endpoint: o.url }] : o.url;
  const client = new o.Centrifuge(endpoint, {
    minReconnectDelay: 500,
    maxReconnectDelay: 30_000,
    ...o.options,
    getToken: async () => {
      try {
        return await o.getToken();
      } catch (e) {
        if (isUnauthorized(e)) throw new o.Centrifuge.UnauthorizedError(e instanceof Error ? e.message : String(e));
        throw e;
      }
    },
  });

  let connects = 0;
  client.on('connecting', () => o.onState?.('connecting'));
  client.on('connected', () => {
    log('realtime connected');
    o.onState?.('connected');
    o.onConnected?.({ reconnect: connects++ > 0 });
  });
  client.on('disconnected', (ctx) => {
    log('realtime disconnected', { code: ctx.code, reason: ctx.reason });
    o.onState?.('disconnected');
  });
  client.on('error', (ctx) => log('realtime error', { type: ctx.type, error: ctx.error?.message }));
  client.on('publication', (ctx) => {
    const env = parseEnvelope(ctx.data);
    if (env) o.onEvent(ctx.channel, env);
    else log('realtime event dropped', { channel: ctx.channel });
  });

  client.connect();
  return {
    close: () => client.disconnect(),
    reconnect: () => {
      client.disconnect();
      client.connect();
    },
  };
}
