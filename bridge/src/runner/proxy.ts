import { createHash, timingSafeEqual } from 'node:crypto';
import { createServer, request as httpRequest, type IncomingHttpHeaders } from 'node:http';
import { request as httpsRequest } from 'node:https';
import type { AddressInfo } from 'node:net';

export type ModelAuth = { kind: 'api_key'; key: string } | { kind: 'oauth'; token: string };

export type ProxyOptions = {
  auth: ModelAuth;
  // The bearer every caller must send; it is stripped before the real credential goes on.
  token: string;
  upstream?: string;
  host?: string;
  port?: number;
  log?: (msg: string, fields?: Record<string, unknown>) => void;
};

export type ModelProxy = { port: number; close(): Promise<void> };

const OAUTH_BETA = 'oauth-2025-04-20';
const STRIPPED = ['host', 'x-api-key', 'authorization', 'connection', 'keep-alive', 'proxy-authorization', 'upgrade'];

// The CLI gets this proxy's URL and token, never the credential; that is added here, on the bridge's event loop.
export async function startModelProxy(opts: ProxyOptions): Promise<ModelProxy> {
  const upstream = new URL(opts.upstream ?? 'https://api.anthropic.com');
  const send = upstream.protocol === 'http:' ? httpRequest : httpsRequest;
  const log = opts.log ?? (() => {});

  if (!opts.token) throw new Error('model proxy needs a token');
  const server = createServer((req, res) => {
    const pathname = (req.url ?? '').split('?')[0];
    if (req.method !== 'POST' || !(pathname === '/v1/messages' || pathname.startsWith('/v1/messages/'))) {
      res.writeHead(404).end();
      return;
    }
    if (!sameToken(bearer(req.headers.authorization), opts.token)) {
      res.writeHead(401).end();
      return;
    }
    const up = send({
      protocol: upstream.protocol,
      hostname: upstream.hostname,
      port: upstream.port || undefined,
      path: req.url,
      method: req.method,
      headers: withAuth(req.headers, opts.auth),
    }, (r) => {
      res.writeHead(r.statusCode ?? 502, r.headers);
      r.pipe(res);
    });
    up.on('error', (e) => {
      log('model proxy upstream error', { error: e.message });
      if (!res.headersSent) res.writeHead(502);
      res.end();
    });
    res.on('close', () => up.destroy());
    req.pipe(up);
  });

  await new Promise<void>((resolve, reject) => {
    server.once('error', reject);
    server.listen(opts.port ?? 0, opts.host ?? '127.0.0.1', resolve);
  });
  return {
    port: (server.address() as AddressInfo).port,
    close: () => new Promise((resolve) => {
      server.closeAllConnections();
      server.close(() => resolve());
    }),
  };
}

function withAuth(incoming: IncomingHttpHeaders, auth: ModelAuth): IncomingHttpHeaders {
  const headers = { ...incoming };
  for (const h of STRIPPED) delete headers[h];
  if (auth.kind === 'api_key') {
    headers['x-api-key'] = auth.key;
    return headers;
  }
  headers.authorization = `Bearer ${auth.token}`;
  const beta = String(headers['anthropic-beta'] ?? '').split(',').map((s) => s.trim()).filter(Boolean);
  if (!beta.includes(OAUTH_BETA)) beta.push(OAUTH_BETA);
  headers['anthropic-beta'] = beta.join(',');
  return headers;
}

function bearer(header: string | undefined): string {
  const m = /^Bearer (.+)$/i.exec(header ?? '');
  return m ? m[1].trim() : '';
}

// Hashing first gives equal-length buffers, so the compare leaks neither content nor length.
function sameToken(got: string, want: string): boolean {
  const h = (s: string) => createHash('sha256').update(s).digest();
  return got !== '' && timingSafeEqual(h(got), h(want));
}
