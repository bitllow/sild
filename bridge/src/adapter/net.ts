import { lookup as dnsLookup } from 'node:dns/promises';
import http from 'node:http';
import https from 'node:https';
import { BlockList, isIP, type LookupFunction } from 'node:net';

export type Resolver = (hostname: string) => Promise<{ address: string; family: number }[]>;

export const systemResolver: Resolver = (hostname) => dnsLookup(hostname, { all: true, verbatim: true });

const LOCAL_HOSTS = new Set(['localhost', '127.0.0.1', '[::1]', '::1']);

const blocked = new BlockList();
for (const [net, bits] of [
  ['0.0.0.0', 8], ['10.0.0.0', 8], ['100.64.0.0', 10], ['127.0.0.0', 8], ['169.254.0.0', 16],
  ['172.16.0.0', 12], ['192.0.0.0', 24], ['192.0.2.0', 24], ['192.168.0.0', 16], ['198.18.0.0', 15],
  ['198.51.100.0', 24], ['203.0.113.0', 24], ['224.0.0.0', 4], ['240.0.0.0', 4],
] as const) blocked.addSubnet(net, bits, 'ipv4');
for (const [net, bits] of [
  ['::', 128], ['::1', 128], ['64:ff9b::', 96], ['64:ff9b:1::', 48], ['100::', 64], ['2001::', 32],
  ['2001:db8::', 32], ['2002::', 16], ['fc00::', 7], ['fe80::', 10], ['fec0::', 10], ['ff00::', 8],
] as const) blocked.addSubnet(net, bits, 'ipv6');

// Private, loopback, link-local, CGNAT, documentation, multicast and the
// IPv6 transition ranges that can embed any of those.
export function isBlockedAddress(address: string): boolean {
  const family = isIP(address);
  if (family === 4) return blocked.check(address, 'ipv4');
  if (family !== 6) return true;
  const mapped = /^::ffff:(\d+\.\d+\.\d+\.\d+)$/i.exec(address);
  if (mapped) return blocked.check(mapped[1]!, 'ipv4');
  if (/^::ffff:/i.test(address)) return true;
  return blocked.check(address, 'ipv6');
}

export function isLocalHost(hostname: string): boolean {
  return LOCAL_HOSTS.has(hostname);
}

export type Target = { url: URL; address: string; family: number };

// Resolves once and pins the connection to the checked address, so a second
// DNS answer cannot redirect the request.
export async function resolveTarget(url: URL, allowInsecureLocal: boolean, resolve: Resolver): Promise<Target> {
  const local = allowInsecureLocal && isLocalHost(url.hostname);
  if (url.protocol !== 'https:' && !(local && url.protocol === 'http:')) throw new Error('adapter must be reached over https');
  if (url.username || url.password) throw new Error('credentials in URL are not allowed');
  const host = url.hostname.replace(/^\[|\]$/g, '');
  const addrs = isIP(host) ? [{ address: host, family: isIP(host) }] : await resolve(host);
  if (!addrs.length) throw new Error(`${host} did not resolve`);
  for (const a of addrs) {
    const loopback = /^(127\.|::ffff:127\.)/i.test(a.address) || a.address === '::1';
    if (isBlockedAddress(a.address) && !(local && loopback)) throw new Error(`${host} resolves to a non-public address`);
  }
  const first = addrs[0]!;
  return { url, address: first.address, family: first.family };
}

export type Response = { status: number; headers: http.IncomingHttpHeaders; body: Buffer };

export type SendOptions = {
  method: string;
  headers?: Record<string, string>;
  body?: Buffer | string;
  timeoutMs: number;
  maxBytes: number;
  signal?: AbortSignal;
};

// One HTTP exchange. Redirects are returned as-is, never followed.
export function send(target: Target, opts: SendOptions): Promise<Response> {
  const pinned: LookupFunction = (_host, options, cb) => {
    if (options.all) (cb as (e: null, a: { address: string; family: number }[]) => void)(null, [{ address: target.address, family: target.family }]);
    else cb(null, target.address, target.family);
  };
  const mod = target.url.protocol === 'https:' ? https : http;
  return new Promise((resolve, reject) => {
    const fail = (err: Error) => {
      clearTimeout(deadline);
      reject(err);
    };
    const req = mod.request(
      target.url,
      { method: opts.method, headers: opts.headers, lookup: pinned, agent: false, signal: opts.signal },
      (res) => {
        const declared = Number(res.headers['content-length']);
        const tooLarge = () => {
          fail(new Error('response too large'));
          req.destroy();
        };
        if (Number.isFinite(declared) && declared > opts.maxBytes) return tooLarge();
        const chunks: Buffer[] = [];
        let size = 0;
        res.on('data', (c: Buffer) => {
          size += c.length;
          if (size > opts.maxBytes) tooLarge();
          else chunks.push(c);
        });
        res.on('end', () => {
          clearTimeout(deadline);
          resolve({ status: res.statusCode ?? 0, headers: res.headers, body: Buffer.concat(chunks) });
        });
        res.on('error', fail);
      },
    );
    const deadline = setTimeout(() => req.destroy(new Error('timed out')), opts.timeoutMs);
    req.on('error', fail);
    req.end(opts.body);
  });
}
