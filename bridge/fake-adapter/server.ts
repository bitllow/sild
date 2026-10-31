// Fake provider adapter serving the design's scenarios; prints its public key for the bridge:
//   ADAPTER_SECRET=… SILD_TENANT=… SILD_JWKS_URL=… [SILD_KEYS] [ADAPTER_PRIVATE_KEY] [ELEVATION_URL] node fake-adapter/server.ts
import { createServer, type Server } from 'node:http';
import type { AddressInfo } from 'node:net';
import {
  MIN_SECRET_LENGTH, STEP_UP_REQUIRED, createAdapter, generateKeyPair, loadPrivateKey, publicKeyOf, type Adapter, type Final, type Handler, type SildTrust,
} from '../src/adapter/kit/index.ts';
import { CATALOG, INSTRUCTIONS, handle } from './scenarios.ts';

export type FakeAdapter = { url: string; publicKey: string; adapter: Adapter; server: Server; close: () => Promise<void> };

// Until when (unix seconds) the requester approved data access for this conversation, or null.
export type Elevation = (user: string, conv: string) => Promise<number | null>;

export async function startFakeAdapter(opts: {
  secret: string; sild: SildTrust; elevated?: Elevation; privateKey?: string; port?: number; host?: string; publicUrl?: string;
}): Promise<FakeAdapter> {
  const privateKey = loadPrivateKey(opts.privateKey ?? generateKeyPair().privateKey);
  const host = opts.host ?? '127.0.0.1';
  let publicUrl = opts.publicUrl ?? '';
  const adapter = createAdapter({
    secret: opts.secret,
    privateKey,
    catalog: CATALOG,
    handle: opts.elevated ? stepUp(opts.elevated, handle) : handle,
    sild: opts.sild,
    instructions: () => INSTRUCTIONS,
    get publicUrl() {
      return publicUrl;
    },
  });
  const server = createServer(adapter.handler);
  await new Promise<void>((resolve) => server.listen(opts.port ?? 0, host, resolve));
  const { port } = server.address() as AddressInfo;
  publicUrl ||= `http://${host}:${port}`;
  return {
    url: publicUrl,
    publicKey: publicKeyOf(privateKey),
    adapter,
    server,
    close: () =>
      new Promise((resolve) => {
        adapter.close();
        server.closeAllConnections();
        server.close(() => resolve());
      }),
  };
}

function stepUp(elevated: Elevation, next: Handler): Handler {
  return async (req) => {
    const until = await elevated(req.requester.id, req.conversation_ref).catch(() => null);
    if (!until) return { type: 'decline', code: STEP_UP_REQUIRED, reason: 'Approve data access to continue' };
    const stamp = (f: Final): Final => (f.type === 'deliver' ? { ...f, approvedUntil: until } : f);
    const outcome = await next(req);
    if (outcome.type === 'async') return { type: 'async', result: outcome.result.then(stamp) };
    return outcome.type === 'deliver' ? stamp(outcome) : outcome;
  };
}

// Asks the integrator, as a real adapter would read its own elevation store.
function elevationAt(url: string): Elevation {
  return async (user, conv) => {
    const res = await fetch(`${url}?${new URLSearchParams({ user, conv })}`, { signal: AbortSignal.timeout(3_000) });
    const e = res.ok ? ((await res.json()) as { elevated?: boolean; until?: number }) : {};
    return e.elevated && typeof e.until === 'number' ? e.until : null;
  };
}

if (import.meta.main) {
  const secret = process.env.ADAPTER_SECRET;
  if (!secret || secret.length < MIN_SECRET_LENGTH) {
    console.error(`ADAPTER_SECRET is required, at least ${MIN_SECRET_LENGTH} characters`);
    process.exit(1);
  }
  const tenant = process.env.SILD_TENANT?.trim();
  const jwksUrl = process.env.SILD_JWKS_URL?.trim() || undefined;
  const keys = process.env.SILD_KEYS ? JSON.parse(process.env.SILD_KEYS) : undefined;
  if (!tenant || (!jwksUrl && !keys)) {
    console.error('SILD_TENANT and SILD_JWKS_URL or SILD_KEYS are required');
    process.exit(1);
  }
  const fake = await startFakeAdapter({
    secret,
    sild: { tenant, jwksUrl, keys },
    elevated: process.env.ELEVATION_URL ? elevationAt(process.env.ELEVATION_URL) : undefined,
    privateKey: process.env.ADAPTER_PRIVATE_KEY || undefined,
    port: Number(process.env.PORT ?? 8790),
    host: process.env.HOST ?? '127.0.0.1',
    publicUrl: process.env.PUBLIC_URL || undefined,
  });
  console.log(`fake adapter listening on ${fake.url}`);
  console.log(`ADAPTER_PUBLIC_KEY=${fake.publicKey}`);
  console.log(`sources: ${CATALOG.map((s) => s.source).join(', ')}`);
}
