import { createSdkMcpServer, tool, type McpSdkServerConfigWithInstance } from '@anthropic-ai/claude-agent-sdk';
import { z } from 'zod';
import type { Ops } from '../slots/types.ts';
import { errText } from '../util.ts';

export const OPS_SERVER = 'ops';
export const OPS_TOOL_NAMES = ['reply', 'ask_for_details', 'propose_fix', 'request_info'];
export const OPS_TOOLS = OPS_TOOL_NAMES.map((t) => `mcp__${OPS_SERVER}__${t}`);

type Result = { content: { type: 'text'; text: string }[]; isError?: boolean };

async function run(fn: () => Promise<string>): Promise<Result> {
  try {
    return { content: [{ type: 'text', text: await fn() }] };
  } catch (e) {
    return { content: [{ type: 'text', text: `failed: ${errText(e)}` }], isError: true };
  }
}

// onAnswered fires once an answer reached the requester; the session ends the turn on it.
export function opsTools(ops: Ops, onAnswered: () => void = () => {}) {
  const answer = (fn: () => Promise<string>) => run(async () => {
    const r = await fn();
    onAnswered();
    return r;
  });
  return [
    tool('reply', 'Send the requester a message: an answer or a status update. Ends your turn.',
      { text: z.string().min(1).max(4000) },
      async ({ text }) => answer(() => ops.reply(text))),
    tool('ask_for_details', 'Ask the requester for something only they know. Ends your turn.',
      { text: z.string().min(1).max(4000) },
      async ({ text }) => answer(() => ops.reply(text))),
    tool('propose_fix', 'Propose a diagnosis: the likely cause, numbered steps to fix it, and the evidence behind it. Ends your turn.',
      {
        cause: z.string().min(1).max(1000),
        fix: z.array(z.string().min(1).max(500)).min(1).max(10),
        evidence: z.array(z.object({
          finding: z.string().min(1).max(500).describe('What you found, in plain words'),
          source: z.string().min(1).max(100).describe('Where it came from, as the requester would say it'),
          ref: z.string().min(1).max(500).optional().describe('Exact file path or request id; operators only'),
        })).min(1).max(10),
      },
      async ({ cause, fix, evidence }) => answer(() => ops.proposeFix(cause, fix, evidence))),
    tool('request_info', 'Ask the application for data you cannot read yourself. Returns at once; data arrives later or never.',
      { need: z.string().min(1).max(500), why: z.string().min(1).max(500) },
      async ({ need, why }) => run(() => ops.requestInfo(need, why))),
  ] as const;
}

export function opsServer(ops: Ops, onAnswered?: () => void): McpSdkServerConfigWithInstance {
  return createSdkMcpServer({ name: OPS_SERVER, version: '1.0.0', tools: [...opsTools(ops, onAnswered)] });
}
