import type { Options, Query } from '@anthropic-ai/claude-agent-sdk';
import assert from 'node:assert/strict';
import { existsSync, mkdirSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { after, test } from 'node:test';
import type { Ops, SessionEvents } from '../slots/types.ts';
import { ClaudeSessionRunner, type ClaudeRunnerConfig } from './session.ts';
import { OPS_TOOLS } from './tools.ts';

const root = mkdtempSync(path.join(os.tmpdir(), 'sild-runner-'));
after(() => rmSync(root, { recursive: true, force: true }));

const ops: Ops = {
  reply: async () => 'sent',
  proposeFix: async () => 'sent', requestInfo: async () => 'requested r1',
};

function harness(over: Partial<ClaudeRunnerConfig> = {}, script: unknown[] = []) {
  const calls: Options[] = [];
  const control = { interrupts: 0, closes: 0 };
  const query = ((params: { options?: Options }) => {
    calls.push(params.options!);
    const gen = (async function* () {
      for (const m of script) yield m;
    })();
    return Object.assign(gen, {
      interrupt: async () => void control.interrupts++,
      close: () => void control.closes++,
    }) as unknown as Query;
  }) as ClaudeRunnerConfig['query'];
  const workRoot = mkdtempSync(path.join(root, 'work-'));
  const runner = new ClaudeSessionRunner({
    workRoot, srcDir: '/src', mode: 'docker', image: 'sild-runner:test', model: 'm',
    maxTurns: 5, maxBudgetUsd: 1, baseUrl: 'http://host.docker.internal:9999', proxyToken: 'proxy-tok', log: () => {},
    query, ...over,
  });
  return { runner, calls, control, workRoot };
}

const noEvents: SessionEvents = { activity() {}, turnEnd() {}, exit() {} };

test('a new session runs locked down, with the proxy URL and token as its only model settings', async () => {
  const { runner, calls, workRoot } = harness();
  await runner.open('c_1', ops, noEvents);
  const o = calls[0];
  const p = runner.paths('c_1');
  assert.equal(o.cwd, path.join(workRoot, 'c_1'));
  assert.equal(o.resume, undefined);
  assert.equal(o.sessionId, readFileSync(p.idFile, 'utf8'));
  assert.equal(typeof o.spawnClaudeCodeProcess, 'function');
  assert.equal(o.env?.ANTHROPIC_BASE_URL, 'http://host.docker.internal:9999');
  assert.equal(o.env?.ANTHROPIC_AUTH_TOKEN, 'proxy-tok');
  assert.equal(o.env?.CLAUDE_CONFIG_DIR, p.config);
  for (const k of Object.keys(o.env ?? {})) assert.doesNotMatch(k, /API_KEY|OAUTH/);
  assert.deepEqual(Object.keys(o.mcpServers ?? {}), ['ops']);
  for (const t of OPS_TOOLS) assert.ok(o.allowedTools?.includes(t), t);
  assert.ok(existsSync(p.data));
  assert.match(String(o.systemPrompt), new RegExp(p.data));
});

test('a session with a transcript on disk resumes; without one it starts fresh', async () => {
  const { runner, calls } = harness();
  await runner.open('c_2', ops, noEvents);
  const id = calls[0].sessionId!;
  await runner.open('c_2', ops, noEvents);
  assert.equal(calls[1].resume, undefined, 'no transcript yet');
  const fresh = calls[1].sessionId!;
  assert.notEqual(fresh, id);

  const project = path.join(runner.paths('c_2').config, 'projects', '-work-c_2');
  mkdirSync(project, { recursive: true });
  writeFileSync(path.join(project, `${fresh}.jsonl`), '{}\n');
  await runner.open('c_2', ops, noEvents);
  assert.equal(calls[2].resume, fresh);
});

test('the integrator instructions reach the brief; without them there is no section', async () => {
  const { runner, calls } = harness({ instructions: async () => ({ text: 'Acme is a CRM.', from: 'adapter.acme.example' }) });
  await runner.open('c_6', ops, noEvents);
  assert.match(String(calls[0].systemPrompt), /## Integrator instructions \(from adapter\.acme\.example\)\n\n<integrator_instructions>\nAcme is a CRM\.\n/);

  const bare = harness({ instructions: async () => null });
  await bare.runner.open('c_7', ops, noEvents);
  assert.doesNotMatch(String(bare.calls[0].systemPrompt), /integrator/i);
});

test('local mode spawns no container and keeps HOME in the config dir', async () => {
  const { runner, calls } = harness({ mode: 'local' });
  await runner.open('c_3', ops, noEvents);
  assert.equal(calls[0].spawnClaudeCodeProcess, undefined);
  assert.equal(calls[0].env?.HOME, runner.paths('c_3').config);
});

test('session output maps to activity, one turnEnd per result, then exit', async () => {
  const script = [
    { type: 'system', subtype: 'init' },
    { type: 'assistant', message: { content: [] } },
    { type: 'user', message: { content: [] } },
    { type: 'result', subtype: 'success' },
    { type: 'result', subtype: 'error_max_turns' },
  ];
  const { runner, control } = harness({}, script);
  const seen: string[] = [];
  let resolveExit!: () => void;
  const exited = new Promise<void>((r) => (resolveExit = r));
  const handle = await runner.open('c_4', ops, {
    activity: () => seen.push('activity'),
    turnEnd: (o) => seen.push(`end:${o}`),
    exit: (e) => {
      seen.push(`exit:${e ?? ''}`);
      resolveExit();
    },
  });
  await exited;
  assert.deepEqual(seen, ['activity', 'activity', 'end:success', 'end:error_max_turns', 'exit:']);
  await handle.interrupt();
  await handle.close();
  assert.deepEqual(control, { interrupts: 1, closes: 1 });
});

test('destroy deletes the transcript through the conversation config dir and removes every directory', async () => {
  const deleted: { id: string; dir?: string; configDir?: string }[] = [];
  const { runner } = harness({
    deleteSession: async (id, opts) => void deleted.push({ id, dir: opts?.dir, configDir: process.env.CLAUDE_CONFIG_DIR }),
  });
  await runner.open('c_5', ops, noEvents);
  const p = runner.paths('c_5');
  const id = readFileSync(p.idFile, 'utf8');
  assert.deepEqual(await runner.known(), ['c_5']);

  await runner.destroy('c_5');
  assert.deepEqual(deleted, [{ id, dir: p.work, configDir: p.config }]);
  for (const d of [p.work, p.config, p.idFile]) assert.equal(existsSync(d), false, d);
  assert.deepEqual(await runner.known(), []);
});

test('conversation ids that are not plain names are refused', () => {
  const { runner } = harness();
  for (const bad of ['../x', '.cfg', 'a/b', '', '.bridge']) assert.throws(() => runner.paths(bad), /unsafe/);
});
