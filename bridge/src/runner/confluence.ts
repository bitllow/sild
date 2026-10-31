import type { McpSdkServerConfigWithInstance } from '@anthropic-ai/claude-agent-sdk';
import { Client } from '@modelcontextprotocol/sdk/client/index.js';
import { StreamableHTTPClientTransport } from '@modelcontextprotocol/sdk/client/streamableHttp.js';
import { McpServer } from '@modelcontextprotocol/sdk/server/mcp.js';
import {
  CallToolRequestSchema, ListToolsRequestSchema, McpError, type CallToolResult, type Tool,
} from '@modelcontextprotocol/sdk/types.js';
import type { Log } from '../slots/types.ts';
import { errText, truncateUtf8 } from '../util.ts';

export const CONFLUENCE_SERVER = 'confluence';
const MAX_RESULT_BYTES = 50_000;

export type ConfluenceConfig = {
  url: string;
  token: string;
  // Header carrying the token; the value is `${scheme} ${token}`, or the bare token without a scheme.
  authHeader: string;
  authScheme?: string;
  readTools: string[];
  timeoutMs: number;
  maxResultBytes?: number;
};

export type ConfluenceSession = { server: McpSdkServerConfigWithInstance; tools: string[] };

export type ConfluenceTools = {
  // Null when the remote is unreachable or exposes none of the read tools; the session runs without them.
  session(): Promise<ConfluenceSession | null>;
  close(): Promise<void>;
};

export function createConfluenceTools(cfg: ConfluenceConfig | undefined, log: Log): ConfluenceTools | null {
  if (!cfg) return null;
  const allow = new Set(cfg.readTools);
  const maxBytes = cfg.maxResultBytes ?? MAX_RESULT_BYTES;
  const headers = { [cfg.authHeader]: cfg.authScheme ? `${cfg.authScheme} ${cfg.token}` : cfg.token };
  let client: Promise<Client> | undefined;
  let listed: Tool[] | undefined;

  const connect = (): Promise<Client> => {
    if (!client) {
      const p = (async () => {
        const c = new Client({ name: 'sild-bridge', version: '1.0.0' });
        await c.connect(new StreamableHTTPClientTransport(new URL(cfg.url), { requestInit: { headers } }));
        return c;
      })();
      client = p;
      p.catch(() => drop(p));
    }
    return client;
  };
  const drop = (p: Promise<Client>): void => {
    if (client !== p) return;
    client = undefined;
    p.then((c) => c.close()).catch(() => {});
  };

  const tools = async (): Promise<Tool[]> => {
    if (listed) return listed;
    const p = connect();
    try {
      const c = await p;
      const all: Tool[] = [];
      let cursor: string | undefined;
      do {
        const page = await c.listTools(cursor ? { cursor } : undefined, { timeout: cfg.timeoutMs });
        all.push(...page.tools);
        cursor = page.nextCursor;
      } while (cursor);
      const missing = cfg.readTools.filter((n) => !all.some((t) => t.name === n));
      if (missing.length) log('confluence tools not offered by the remote', { missing });
      listed = all.filter((t) => allow.has(t.name));
      return listed;
    } catch (e) {
      drop(p);
      throw e;
    }
  };

  const call = async (name: string, args: Record<string, unknown> | undefined, signal: AbortSignal): Promise<CallToolResult> => {
    if (!allow.has(name)) return failure(`tool ${name} is not available`);
    const p = connect();
    try {
      const res = await (await p).callTool({ name, arguments: args }, undefined, { timeout: cfg.timeoutMs, signal });
      return { content: [{ type: 'text', text: cap(toText(res as CallToolResult), maxBytes) }], isError: res.isError === true };
    } catch (e) {
      // An McpError came back over a working connection; anything else may mean it is gone.
      if (!(e instanceof McpError)) drop(p);
      log('confluence call failed', { tool: name, error: errText(e) });
      return failure(`Confluence request failed: ${errText(e)}`);
    }
  };

  return {
    async session() {
      let offered: Tool[];
      try {
        offered = await tools();
      } catch (e) {
        log('confluence unavailable', { error: errText(e) });
        return null;
      }
      if (!offered.length) return null;
      // An McpServer binds to one transport, so each session gets its own.
      const server = new McpServer({ name: CONFLUENCE_SERVER, version: '1.0.0' }, { capabilities: { tools: {} } });
      server.server.setRequestHandler(ListToolsRequestSchema, () => ({
        tools: offered.map(({ name, title, description, inputSchema, annotations }) => ({ name, title, description, inputSchema, annotations })),
      }));
      server.server.setRequestHandler(CallToolRequestSchema, (req, extra) => call(req.params.name, req.params.arguments, extra.signal));
      return {
        server: { type: 'sdk', name: CONFLUENCE_SERVER, instance: server },
        tools: offered.map((t) => `mcp__${CONFLUENCE_SERVER}__${t.name}`),
      };
    },
    async close() {
      const p = client;
      client = undefined;
      await p?.then((c) => c.close()).catch(() => {});
    },
  };
}

function toText(res: CallToolResult): string {
  const parts = (res.content ?? []).map((c) => {
    if (c.type === 'text') return c.text;
    if (c.type === 'resource' && 'text' in c.resource) return c.resource.text;
    return `[${c.type} content omitted]`;
  });
  if (!parts.length && res.structuredContent) parts.push(JSON.stringify(res.structuredContent));
  return parts.join('\n\n');
}

function cap(text: string, max: number): string {
  const bytes = Buffer.byteLength(text);
  if (bytes <= max) return text;
  return `${truncateUtf8(text, max)}\n\n[truncated: showing ${max} of ${bytes} bytes; narrow the request for the rest]`;
}

function failure(text: string): CallToolResult {
  return { content: [{ type: 'text', text }], isError: true };
}
