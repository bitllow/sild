import {
  deleteSession as sdkDeleteSession, query as sdkQuery,
  type SDKMessage, type SDKUserMessage,
} from '@anthropic-ai/claude-agent-sdk';
import { randomUUID } from 'node:crypto';
import { access, mkdir, readdir, readFile, rm, writeFile } from 'node:fs/promises';
import path from 'node:path';
import type { IntegratorInstructions } from '../contracts.ts';
import type { Log, Ops, SessionEvents, SessionHandle, SessionRunner } from '../slots/types.ts';
import { brief } from './brief.ts';
import { CONFLUENCE_SERVER, type ConfluenceTools } from './confluence.ts';
import { dockerSpawner } from './docker.ts';
import { sessionOptions } from './lockdown.ts';
import { OPS_SERVER, OPS_TOOLS, opsServer } from './tools.ts';
import { errText } from '../util.ts';

export type ClaudeRunnerConfig = {
  workRoot: string;
  srcDir: string;
  mode: 'docker' | 'local';
  image: string;
  network?: string;
  model: string;
  maxTurns: number;
  maxBudgetUsd: number;
  // The model proxy as the CLI reaches it, and the bearer it accepts.
  baseUrl: string;
  proxyToken: string;
  log: Log;
  confluence?: ConfluenceTools | null;
  // The integrator's assistant instructions; none when absent.
  instructions?: () => Promise<IntegratorInstructions | null>;
  query?: typeof sdkQuery;
  deleteSession?: typeof sdkDeleteSession;
};

// Conversation ids become directory names.
const CONV_ID = /^[A-Za-z0-9][A-Za-z0-9_-]{0,63}$/;

export type ConvPaths = { work: string; data: string; config: string; idFile: string };

export class ClaudeSessionRunner implements SessionRunner {
  private readonly cfg: ClaudeRunnerConfig;
  private deletes: Promise<void> = Promise.resolve();

  constructor(cfg: ClaudeRunnerConfig) {
    this.cfg = cfg;
  }

  // The session id file sits outside every directory mounted into the runner.
  paths(conv: string): ConvPaths {
    if (!CONV_ID.test(conv)) throw new Error(`unsafe conversation id: ${JSON.stringify(conv)}`);
    const work = path.join(this.cfg.workRoot, conv);
    return {
      work,
      data: path.join(work, 'data'),
      config: path.join(this.cfg.workRoot, '.cfg', conv),
      idFile: path.join(this.cfg.workRoot, '.bridge', 'sessions', conv),
    };
  }

  async open(conv: string, ops: Ops, events: SessionEvents): Promise<SessionHandle> {
    const p = this.paths(conv);
    await Promise.all([
      mkdir(p.data, { recursive: true }),
      mkdir(p.config, { recursive: true }),
      mkdir(path.dirname(p.idFile), { recursive: true }),
    ]);
    let sessionId = await readId(p.idFile);
    // Without its transcript a session cannot resume; start a fresh one.
    const resume = sessionId !== '' && await hasTranscript(p.config, sessionId);
    if (!resume) {
      sessionId = randomUUID();
      await writeFile(p.idFile, sessionId);
    }

    const env: Record<string, string> = {
      ANTHROPIC_BASE_URL: this.cfg.baseUrl,
      ANTHROPIC_AUTH_TOKEN: this.cfg.proxyToken,
      API_TIMEOUT_MS: '120000',
    };
    if (this.cfg.mode === 'local') env.HOME = p.config;

    // Composed on every open, but where the CLI records a session's prompt a resumed session keeps the one it started with.
    const [kb, integrator] = await Promise.all([
      this.cfg.confluence?.session() ?? null,
      this.cfg.instructions?.() ?? null,
    ]);
    const inbox = new Inbox();
    const q = (this.cfg.query ?? sdkQuery)({
      prompt: inbox,
      options: sessionOptions({
        workDir: p.work,
        srcDir: this.cfg.srcDir,
        configDir: p.config,
        sessionId,
        resume,
        systemPrompt: brief({ srcDir: this.cfg.srcDir, workDir: p.work, dataDir: p.data, confluence: kb !== null, integrator }),
        mcpServers: { [OPS_SERVER]: opsServer(ops), ...(kb && { [CONFLUENCE_SERVER]: kb.server }) },
        mcpTools: [...OPS_TOOLS, ...(kb?.tools ?? [])],
        env,
        model: this.cfg.model,
        maxTurns: this.cfg.maxTurns,
        maxBudgetUsd: this.cfg.maxBudgetUsd,
        onDecision: (tool, _input, d) => {
          if (!d.allow) this.cfg.log('tool denied', { conv, tool, reason: d.reason });
        },
        spawn: this.cfg.mode === 'docker'
          ? dockerSpawner({
              image: this.cfg.image,
              network: this.cfg.network,
              mounts: [
                { path: this.cfg.srcDir, readOnly: true },
                { path: p.work, readOnly: false },
                { path: p.config, readOnly: false },
              ],
            })
          : undefined,
      }),
    });
    void consume(q, events);
    this.cfg.log('session open', { conv, sessionId, resume, instructions: integrator && (integrator.version ?? 'unversioned') });

    return {
      push: (text) => inbox.push(text),
      interrupt: async () => {
        await q.interrupt();
      },
      close: async () => {
        inbox.close();
        q.close();
      },
    };
  }

  async destroy(conv: string): Promise<void> {
    const p = this.paths(conv);
    const sessionId = await readId(p.idFile);
    if (sessionId) {
      const del = this.cfg.deleteSession ?? sdkDeleteSession;
      await this.withConfigDir(p.config, () => del(sessionId, { dir: p.work }))
        .catch((e) => this.cfg.log('deleteSession failed', { conv, error: String(e) }));
    }
    await Promise.all([
      rm(p.work, { recursive: true, force: true }),
      rm(p.config, { recursive: true, force: true }),
      rm(p.idFile, { force: true }),
    ]);
  }

  async known(): Promise<string[]> {
    try {
      return (await readdir(path.join(this.cfg.workRoot, '.bridge', 'sessions'))).filter((c) => CONV_ID.test(c));
    } catch {
      return [];
    }
  }

  // deleteSession finds transcripts through the process-wide CLAUDE_CONFIG_DIR.
  private withConfigDir(dir: string, fn: () => Promise<void>): Promise<void> {
    const run = this.deletes.then(async () => {
      const prev = process.env.CLAUDE_CONFIG_DIR;
      process.env.CLAUDE_CONFIG_DIR = dir;
      try {
        await fn();
      } finally {
        if (prev === undefined) delete process.env.CLAUDE_CONFIG_DIR;
        else process.env.CLAUDE_CONFIG_DIR = prev;
      }
    });
    this.deletes = run.catch(() => {});
    return run;
  }
}

async function consume(q: AsyncIterable<SDKMessage>, events: SessionEvents): Promise<void> {
  try {
    for await (const m of q) {
      if (m.type === 'result') events.turnEnd(m.subtype);
      else if (m.type === 'assistant' || m.type === 'user' || m.type === 'stream_event') events.activity();
    }
    events.exit();
  } catch (e) {
    events.exit(errText(e));
  }
}

async function readId(file: string): Promise<string> {
  try {
    return (await readFile(file, 'utf8')).trim();
  } catch {
    return '';
  }
}

export async function hasTranscript(configDir: string, sessionId: string): Promise<boolean> {
  const projects = path.join(configDir, 'projects');
  let dirs: string[];
  try {
    dirs = await readdir(projects);
  } catch {
    return false;
  }
  for (const d of dirs) {
    try {
      await access(path.join(projects, d, `${sessionId}.jsonl`));
      return true;
    } catch {
      // not in this project dir
    }
  }
  return false;
}

// Streaming input: messages queue while a turn runs; the CLI may merge them into one turn.
class Inbox implements AsyncIterable<SDKUserMessage> {
  private queue: SDKUserMessage[] = [];
  private wake?: () => void;
  private closed = false;

  push(text: string): void {
    if (this.closed) return;
    this.queue.push({ type: 'user', message: { role: 'user', content: text }, parent_tool_use_id: null });
    this.wake?.();
  }

  close(): void {
    this.closed = true;
    this.wake?.();
  }

  async *[Symbol.asyncIterator](): AsyncIterator<SDKUserMessage> {
    while (true) {
      if (this.queue.length) yield this.queue.shift()!;
      else if (this.closed) return;
      else await new Promise<void>((r) => (this.wake = r));
    }
  }
}
