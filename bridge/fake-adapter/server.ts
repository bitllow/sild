// Fake provider adapter serving the design's scenarios.
//   ADAPTER_SECRET=dev-secret-at-least-16 PORT=8790 node fake-adapter/server.ts
// ADAPTER_PRIVATE_KEY (base64url PKCS#8 or PEM) pins a key; otherwise one is
// generated and its public half printed for the bridge config.
import { createServer, type Server } from 'node:http';
import type { AddressInfo } from 'node:net';
import { MIN_SECRET_LENGTH, createAdapter, generateKeyPair, loadPrivateKey, publicKeyOf, type Adapter } from '../src/adapter/kit/index.ts';
import { CATALOG, INSTRUCTIONS, handle } from './scenarios.ts';

export type FakeAdapter = { url: string; publicKey: string; adapter: Adapter; server: Server; close: () => Promise<void> };

export async function startFakeAdapter(opts: { secret: string; privateKey?: string; port?: number; host?: string; publicUrl?: string }): Promise<FakeAdapter> {
  const privateKey = loadPrivateKey(opts.privateKey ?? generateKeyPair().privateKey);
  const host = opts.host ?? '127.0.0.1';
  let publicUrl = opts.publicUrl ?? '';
  const adapter = createAdapter({
    secret: opts.secret,
    privateKey,
    catalog: CATALOG,
    handle,
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

if (import.meta.main) {
  const secret = process.env.ADAPTER_SECRET;
  if (!secret || secret.length < MIN_SECRET_LENGTH) {
    console.error(`ADAPTER_SECRET is required, at least ${MIN_SECRET_LENGTH} characters`);
    process.exit(1);
  }
  const fake = await startFakeAdapter({
    secret,
    privateKey: process.env.ADAPTER_PRIVATE_KEY || undefined,
    port: Number(process.env.PORT ?? 8790),
    host: process.env.HOST ?? '127.0.0.1',
    publicUrl: process.env.PUBLIC_URL || undefined,
  });
  console.log(`fake adapter listening on ${fake.url}`);
  console.log(`ADAPTER_PUBLIC_KEY=${fake.publicKey}`);
  console.log(`sources: ${CATALOG.map((s) => s.source).join(', ')}`);
}
