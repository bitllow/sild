import { readFileSync } from 'node:fs';
import { wsUrl } from '@sild/client';
import type { ConfluenceConfig } from './runner/confluence.ts';
import type { ModelAuth } from './runner/proxy.ts';
import { errText } from './util.ts';

export type Config = {
  sildUrl: string;
  sildWsUrl: string;
  serviceKey: string;
  queue: string;
  slots: number;
  idleReleaseMs: number;
  maxClaimMs: number;
  turnTimeoutMs: number;
  pollMs: number;
  maxTurns: number;
  maxBudgetUsd: number;
  workRoot: string;
  srcDir: string;
  runnerMode: 'docker' | 'local';
  runnerImage: string;
  runnerNetwork?: string;
  model: string;
  modelAuth: ModelAuth;
  proxy: { host: string; port: number; runnerUrl?: string };
  confluence?: ConfluenceConfig;
};

export class ConfigError extends Error {}

export function loadConfig(env: NodeJS.ProcessEnv = process.env): Config {
  const problems: string[] = [];
  const str = (name: string, fallback?: string): string => {
    const v = env[name]?.trim();
    if (v) return v;
    if (fallback === undefined) problems.push(`${name} is required`);
    return fallback ?? '';
  };
  const num = (name: string, fallback: number, min: number): number => {
    const raw = env[name]?.trim();
    if (!raw) return fallback;
    const n = Number(raw);
    if (!Number.isFinite(n) || n < min) {
      problems.push(`${name} must be a number ≥ ${min}`);
      return fallback;
    }
    return n;
  };
  const int = (name: string, fallback: number, min: number): number => {
    const n = num(name, fallback, min);
    if (!Number.isInteger(n)) problems.push(`${name} must be a whole number`);
    return n;
  };

  const sildUrl = str('SILD_URL');
  if (sildUrl && !/^https?:\/\//.test(sildUrl)) problems.push('SILD_URL must be an http(s) URL');
  const mode = str('RUNNER_MODE', 'docker');
  if (mode !== 'docker' && mode !== 'local') problems.push('RUNNER_MODE must be docker or local');
  const queue = str('BRIDGE_QUEUE', 'triage');
  if (!/^[a-z0-9][a-z0-9_-]{0,31}$/.test(queue)) problems.push('BRIDGE_QUEUE is not a valid queue key');

  const modelAuth = loadModelAuth(env, problems);
  const cfg: Config = {
    sildUrl,
    sildWsUrl: env.SILD_WS_URL?.trim() || (/^https?:\/\//.test(sildUrl) ? wsUrl(sildUrl) : ''),
    serviceKey: str('SILD_SERVICE_KEY'),
    queue,
    slots: int('SLOTS', 1, 1),
    idleReleaseMs: num('IDLE_RELEASE_SECONDS', 180, 1) * 1000,
    maxClaimMs: num('MAX_CLAIM_SECONDS', 1800, 1) * 1000,
    turnTimeoutMs: num('TURN_TIMEOUT_SECONDS', 300, 1) * 1000,
    pollMs: num('POLL_SECONDS', 30, 1) * 1000,
    maxTurns: int('MAX_TURNS', 40, 1),
    maxBudgetUsd: num('MAX_BUDGET_USD', 2, 0.01),
    workRoot: str('WORK_ROOT', '/work'),
    srcDir: str('SRC_DIR', '/src'),
    runnerMode: mode as Config['runnerMode'],
    runnerImage: str('RUNNER_IMAGE', 'sild-runner:2.1.296'),
    runnerNetwork: env.RUNNER_NETWORK?.trim() || undefined,
    model: str('MODEL', 'claude-sonnet-5-5'),
    modelAuth: modelAuth ?? { kind: 'api_key', key: '' },
    proxy: {
      host: str('MODEL_PROXY_HOST', '127.0.0.1'),
      port: int('MODEL_PROXY_PORT', 0, 0),
      runnerUrl: env.MODEL_PROXY_URL?.trim() || undefined,
    },
    confluence: loadConfluence(env, problems, num),
  };
  if (problems.length) throw new ConfigError(`invalid configuration:\n  ${problems.join('\n  ')}`);
  return cfg;
}

function loadConfluence(
  env: NodeJS.ProcessEnv, problems: string[], num: (name: string, fallback: number, min: number) => number,
): ConfluenceConfig | undefined {
  const url = env.CONFLUENCE_MCP_URL?.trim();
  if (!url) {
    const stray = Object.keys(env).filter((k) => k.startsWith('CONFLUENCE_') && env[k]?.trim());
    if (stray.length) problems.push(`CONFLUENCE_MCP_URL is required when ${stray.join(', ')} is set`);
    return undefined;
  }
  if (!/^https?:\/\//.test(url)) problems.push('CONFLUENCE_MCP_URL must be an http(s) URL');
  const readTools = (env.CONFLUENCE_READ_TOOLS ?? '').split(',').map((t) => t.trim()).filter(Boolean);
  if (!readTools.length) problems.push('CONFLUENCE_READ_TOOLS is required with CONFLUENCE_MCP_URL');
  const authHeader = env.CONFLUENCE_AUTH_HEADER?.trim() || 'Authorization';
  const scheme = env.CONFLUENCE_AUTH_SCHEME?.trim();
  return {
    url,
    token: loadSecret(env, problems, 'CONFLUENCE_TOKEN', { needed: ' with CONFLUENCE_MCP_URL' })?.value ?? '',
    authHeader,
    authScheme: scheme === 'none' ? undefined : scheme || (authHeader.toLowerCase() === 'authorization' ? 'Bearer' : undefined),
    readTools,
    timeoutMs: num('CONFLUENCE_TIMEOUT_SECONDS', 30, 1) * 1000,
  };
}

function loadModelAuth(env: NodeJS.ProcessEnv, problems: string[]): ModelAuth | undefined {
  const s = loadSecret(env, problems, 'ANTHROPIC_API_KEY', { file: 'CLAUDE_CODE_OAUTH_TOKEN_FILE' });
  if (!s) return undefined;
  return s.fromFile ? { kind: 'oauth', token: s.value } : { kind: 'api_key', key: s.value };
}

// A secret set inline in `name` or read from the file named by `file`: exactly one, never empty.
export function loadSecret(
  env: NodeJS.ProcessEnv, problems: string[], name: string, opts: { file?: string; needed?: string } = {},
): { value: string; fromFile: boolean } | undefined {
  const fileVar = opts.file ?? `${name}_FILE`;
  const inline = env[name]?.trim();
  const file = env[fileVar]?.trim();
  if (inline && file) {
    problems.push(`set ${name} or ${fileVar}, not both`);
    return undefined;
  }
  if (inline) return { value: inline, fromFile: false };
  if (!file) {
    problems.push(`${name} or ${fileVar} is required${opts.needed ?? ''}`);
    return undefined;
  }
  try {
    const value = readFileSync(file, 'utf8').trim();
    if (value) return { value, fromFile: true };
    problems.push(`${fileVar} is empty`);
  } catch (e) {
    problems.push(`${fileVar} cannot be read: ${errText(e)}`);
  }
  return undefined;
}
