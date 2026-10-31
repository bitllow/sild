import type { Options } from '@anthropic-ai/claude-agent-sdk';
import { Client } from '@modelcontextprotocol/sdk/client/index.js';
import { InMemoryTransport } from '@modelcontextprotocol/sdk/inMemory.js';
import { McpServer } from '@modelcontextprotocol/sdk/server/mcp.js';
import { StreamableHTTPServerTransport } from '@modelcontextprotocol/sdk/server/streamableHttp.js';
import type { CallToolResult } from '@modelcontextprotocol/sdk/types.js';
import assert from 'node:assert/strict';
import { mkdtempSync, rmSync } from 'node:fs';
import { createServer, type IncomingHttpHeaders, type Server } from 'node:http';
import type { AddressInfo } from 'node:net';
import os from 'node:os';
import path from 'node:path';
import { after, test } from 'node:test';
import { z } from 'zod';
import { createConfluenceTools, type ConfluenceConfig, type ConfluenceSession } from './confluence.ts';
import { ClaudeSessionRunner, type ClaudeRunnerConfig } from './session.ts';

const seen: { headers: IncomingHttpHeaders[]; calls: string[] } = { headers: [], calls: [] };

function remote(): McpServer {
  const s = new McpServer({ name: 'fake-confluence', version: '1.0.0' });
  s.registerTool('search', {
    description: 'Search pages',
    inputSchema: { query: z.string(), size: z.number().optional() },
  }, async ({ query, size }) => {
    seen.calls.push(`search:${query}`);
    return { content: [{ type: 'text', text: size ? 'x'.repeat(size) : `found ${query}` }] };
  });
  s.registerTool('create_page', { description: 'Create a page', inputSchema: { title: z.string() } }, async ({ title }) => {
    seen.calls.push(`create:${title}`);
    return { content: [{ type: 'text', text: 'created' }] };
  });
  s.registerTool('boom', { description: 'Fails' }, async () => {
    throw new Error('remote exploded');
  });
  s.registerTool('hang', { description: 'Never answers' }, () => new Promise<CallToolResult>(() => {}));
  return s;
}

function listen(port = 0): Promise<Server> {
  const http = createServer(async (req, res) => {
    seen.headers.push(req.headers);
    const transport = new StreamableHTTPServerTransport({ sessionIdGenerator: undefined });
    res.on('close', () => void transport.close());
    await remote().connect(transport);
    await transport.handleRequest(req, res);
  });
  return new Promise((resolve) => http.listen(port, '127.0.0.1', () => resolve(http)));
}

function stop(http: Server): Promise<void> {
  http.closeAllConnections();
  return new Promise((resolve) => http.close(() => resolve()));
}

const http = await listen();
const url = `http://127.0.0.1:${(http.address() as AddressInfo).port}/mcp`;
after(() => stop(http));

const base: ConfluenceConfig = {
  url, token: 't0k', authHeader: 'Authorization', authScheme: 'Bearer',
  readTools: ['search', 'boom', 'hang', 'not_on_remote'], timeoutMs: 2000,
};

async function attach(kb: ConfluenceSession | null): Promise<Client> {
  assert.ok(kb);
  const [a, b] = InMemoryTransport.createLinkedPair();
  await kb.server.instance.connect(a);
  const c = new Client({ name: 'cli', version: '1' });
  await c.connect(b);
  return c;
}

const text = (r: unknown): string => ((r as CallToolResult).content[0] as { text: string }).text;

test('unconfigured gives no tools', () => {
  assert.equal(createConfluenceTools(undefined, () => {}), null);
});

test('only allowlisted remote tools are exposed, with their input schema', async () => {
  const logs: unknown[] = [];
  const kb = createConfluenceTools(base, (msg, f) => logs.push([msg, f]))!;
  after(() => kb.close());
  const s = await kb.session();
  assert.deepEqual(s?.tools, ['mcp__confluence__search', 'mcp__confluence__boom', 'mcp__confluence__hang']);
  const { tools } = await (await attach(s)).listTools();
  assert.deepEqual(tools.map((t) => t.name), ['search', 'boom', 'hang']);
  assert.deepEqual(Object.keys(tools[0].inputSchema.properties ?? {}), ['query', 'size']);
  assert.deepEqual(logs, [['confluence tools not offered by the remote', { missing: ['not_on_remote'] }]]);
});

test('calls forward with the token header and return the remote text', async () => {
  const kb = createConfluenceTools(base, () => {})!;
  after(() => kb.close());
  seen.headers.length = 0;
  const c = await attach(await kb.session());
  const r = await c.callTool({ name: 'search', arguments: { query: 'refunds' } });
  assert.equal(text(r), 'found refunds');
  assert.ok(!r.isError);
  assert.ok(seen.headers.length > 0);
  for (const h of seen.headers) assert.equal(h.authorization, 'Bearer t0k');
});

test('a custom header carries the bare token', async () => {
  const kb = createConfluenceTools({ ...base, authHeader: 'X-Api-Key', authScheme: undefined }, () => {})!;
  after(() => kb.close());
  seen.headers.length = 0;
  await kb.session();
  assert.ok(seen.headers.length > 0);
  for (const h of seen.headers) {
    assert.equal(h['x-api-key'], 't0k');
    assert.equal(h.authorization, undefined);
  }
});

test('a tool outside the allowlist is refused without reaching the remote', async () => {
  const kb = createConfluenceTools(base, () => {})!;
  after(() => kb.close());
  const c = await attach(await kb.session());
  seen.calls.length = 0;
  const r = await c.callTool({ name: 'create_page', arguments: { title: 't' } });
  assert.equal(r.isError, true);
  assert.deepEqual(seen.calls, []);
});

test('oversized output is truncated with a note', async () => {
  const kb = createConfluenceTools({ ...base, maxResultBytes: 100 }, () => {})!;
  after(() => kb.close());
  const c = await attach(await kb.session());
  const t = text(await c.callTool({ name: 'search', arguments: { query: 'q', size: 5000 } }));
  assert.ok(t.startsWith('x'.repeat(100) + '\n\n[truncated: showing 100 of 5000 bytes'), t);
});

test('remote errors and timeouts come back as tool errors', async () => {
  const kb = createConfluenceTools({ ...base, timeoutMs: 300 }, () => {})!;
  after(() => kb.close());
  const c = await attach(await kb.session());
  const boom = await c.callTool({ name: 'boom', arguments: {} });
  assert.equal(boom.isError, true);
  assert.match(text(boom), /remote exploded/);
  const hang = await c.callTool({ name: 'hang', arguments: {} });
  assert.equal(hang.isError, true);
  assert.match(text(hang), /Confluence request failed: .*timed out/i);
  assert.equal(text(await c.callTool({ name: 'search', arguments: { query: 'after' } })), 'found after');
});

test('each session gets its own server instance', async () => {
  const kb = createConfluenceTools(base, () => {})!;
  after(() => kb.close());
  const [a, b] = [await kb.session(), await kb.session()];
  assert.notEqual(a?.server.instance, b?.server.instance);
  for (const c of [await attach(a), await attach(b)]) {
    assert.equal(text(await c.callTool({ name: 'search', arguments: { query: 'q' } })), 'found q');
  }
});

test('an unreachable remote gives no tools, then reconnects once it is back', async () => {
  const spare = await listen();
  const port = (spare.address() as AddressInfo).port;
  await stop(spare);
  const logs: string[] = [];
  const kb = createConfluenceTools({ ...base, url: `http://127.0.0.1:${port}/mcp` }, (m) => logs.push(m))!;
  after(() => kb.close());
  assert.equal(await kb.session(), null);
  assert.ok(logs.includes('confluence unavailable'));

  const back = await listen(port);
  after(() => stop(back));
  const c = await attach(await kb.session());
  assert.equal(text(await c.callTool({ name: 'search', arguments: { query: 'q' } })), 'found q');
});

test('a session gets the confluence server and tools only when they are available', async () => {
  const work = mkdtempSync(path.join(os.tmpdir(), 'sild-kb-'));
  after(() => rmSync(work, { recursive: true, force: true }));
  const kb = createConfluenceTools(base, () => {})!;
  after(() => kb.close());
  const seenOpts: Options[] = [];
  let up = true;
  const runner = new ClaudeSessionRunner({
    workRoot: work, srcDir: '/src', mode: 'local', image: 'i', model: 'm', maxTurns: 1, maxBudgetUsd: 1,
    baseUrl: 'http://127.0.0.1:1', proxyToken: 't', log: () => {},
    confluence: { session: async () => (up ? kb.session() : null), close: async () => {} },
    query: ((p: { options: Options }) => {
      seenOpts.push(p.options);
      return Object.assign((async function* () {})(), { interrupt: async () => {}, close: () => {} });
    }) as unknown as ClaudeRunnerConfig['query'],
  });
  const ops = { reply: async () => '', proposeFix: async () => '', requestInfo: async () => '' };
  const events = { activity() {}, turnEnd() {}, exit() {} };

  await runner.open('c_kb', ops, events);
  up = false;
  await runner.open('c_kb', ops, events);
  const [on, off] = seenOpts;
  assert.deepEqual(Object.keys(on.mcpServers ?? {}), ['ops', 'confluence']);
  assert.ok(on.allowedTools?.includes('mcp__confluence__search'));
  assert.match(String(on.systemPrompt), /Confluence, read-only/);
  assert.deepEqual(Object.keys(off.mcpServers ?? {}), ['ops']);
  assert.ok(!off.allowedTools?.some((t) => t.startsWith('mcp__confluence__')));
  assert.doesNotMatch(String(off.systemPrompt), /Confluence, read-only/);
});
