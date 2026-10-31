import { createServer, type IncomingMessage, type ServerResponse } from 'node:http';
import type { AddressInfo } from 'node:net';

export type Seen = { method: string; url: string; headers: IncomingMessage['headers']; body: string };
export type Handler = (req: Seen, res: ServerResponse) => void;

// A local HTTP server that records every request and answers with `handler`.
export async function serve(handler: Handler): Promise<{ url: string; seen: Seen[]; close(): Promise<void> }> {
  const seen: Seen[] = [];
  const server = createServer(async (req, res) => {
    const chunks: Buffer[] = [];
    for await (const c of req) chunks.push(c as Buffer);
    const s = { method: req.method ?? '', url: req.url ?? '', headers: req.headers, body: Buffer.concat(chunks).toString() };
    seen.push(s);
    handler(s, res);
  });
  await new Promise<void>((r) => server.listen(0, '127.0.0.1', r));
  return {
    url: `http://127.0.0.1:${(server.address() as AddressInfo).port}`,
    seen,
    close: async () => {
      server.closeAllConnections();
      await new Promise<void>((r) => server.close(() => r()));
    },
  };
}

export function json(res: ServerResponse, status: number, body?: unknown, headers: Record<string, string> = {}): void {
  if (body === undefined) {
    res.writeHead(status, headers).end();
    return;
  }
  res.writeHead(status, { 'content-type': 'application/json', ...headers }).end(JSON.stringify(body));
}
