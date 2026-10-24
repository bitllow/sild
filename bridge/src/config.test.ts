import assert from 'node:assert/strict';
import { mkdtempSync, rmSync, writeFileSync } from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { after, test } from 'node:test';
import { ConfigError, loadConfig } from './config.ts';

const dir = mkdtempSync(path.join(os.tmpdir(), 'sild-config-'));
after(() => rmSync(dir, { recursive: true, force: true }));

const base = { SILD_URL: 'https://chat.example.com', SILD_SERVICE_KEY: 'sild_live_x', ANTHROPIC_API_KEY: 'sk-x' };

test('defaults', () => {
  const c = loadConfig(base);
  assert.equal(c.queue, 'triage');
  assert.equal(c.slots, 1);
  assert.equal(c.idleReleaseMs, 180_000);
  assert.equal(c.runnerMode, 'docker');
  assert.equal(c.runnerImage, 'sild-runner:2.1.296');
  assert.equal(c.model, 'claude-sonnet-5-5');
  assert.equal(c.effort, undefined);
  assert.equal(c.sildWsUrl, 'wss://chat.example.com/v1/ws');
  assert.deepEqual(c.modelAuth, { kind: 'api_key', key: 'sk-x' });
  assert.equal(c.proxy.host, '127.0.0.1');
});

test('overrides', () => {
  const c = loadConfig({
    ...base, SLOTS: '3', IDLE_RELEASE_SECONDS: '60', TURN_TIMEOUT_SECONDS: '90', MAX_BUDGET_USD: '0.5',
    RUNNER_MODE: 'local', SILD_WS_URL: 'ws://ws.local/v1/ws', BRIDGE_QUEUE: 'ops-help',
  });
  assert.equal(c.slots, 3);
  assert.equal(c.idleReleaseMs, 60_000);
  assert.equal(c.turnTimeoutMs, 90_000);
  assert.equal(c.maxBudgetUsd, 0.5);
  assert.equal(c.runnerMode, 'local');
  assert.equal(c.sildWsUrl, 'ws://ws.local/v1/ws');
  assert.equal(c.queue, 'ops-help');
});

test('oauth token comes from a file', () => {
  const file = path.join(dir, 'token');
  writeFileSync(file, 'oat-123\n');
  const { ANTHROPIC_API_KEY: _, ...rest } = base;
  assert.deepEqual(loadConfig({ ...rest, CLAUDE_CODE_OAUTH_TOKEN_FILE: file }).modelAuth, { kind: 'oauth', token: 'oat-123' });
});

test('every problem is reported at once', () => {
  assert.throws(
    () => loadConfig({ SLOTS: '0', RUNNER_MODE: 'vm', ANTHROPIC_API_KEY: 'a', CLAUDE_CODE_OAUTH_TOKEN_FILE: '/x' }),
    (e: unknown) => e instanceof ConfigError
      && ['SILD_URL', 'SILD_SERVICE_KEY', 'SLOTS', 'RUNNER_MODE', 'not both'].every((s) => e.message.includes(s)),
  );
});

test('confluence is off unless its URL is set', () => {
  assert.equal(loadConfig(base).confluence, undefined);
  assert.throws(() => loadConfig({ ...base, CONFLUENCE_READ_TOOLS: 'search' }), /CONFLUENCE_MCP_URL is required when CONFLUENCE_READ_TOOLS is set/);
});

test('confluence reads its token from a file and sends it as a bearer token by default', () => {
  const file = path.join(dir, 'confluence-token');
  writeFileSync(file, 'ct-1\n');
  const c = loadConfig({
    ...base, CONFLUENCE_MCP_URL: 'https://mcp.example.com/v1/mcp', CONFLUENCE_TOKEN_FILE: file,
    CONFLUENCE_READ_TOOLS: ' search , getPage,,',
  });
  assert.deepEqual(c.confluence, {
    url: 'https://mcp.example.com/v1/mcp', token: 'ct-1', authHeader: 'Authorization', authScheme: 'Bearer',
    readTools: ['search', 'getPage'], timeoutMs: 30_000,
  });
});

test('a custom confluence header carries the bare token unless a scheme is set', () => {
  const env = { ...base, CONFLUENCE_MCP_URL: 'https://m.example.com', CONFLUENCE_TOKEN: 't', CONFLUENCE_READ_TOOLS: 'search' };
  const custom = loadConfig({ ...env, CONFLUENCE_AUTH_HEADER: 'X-Api-Key' }).confluence;
  assert.equal(custom?.authHeader, 'X-Api-Key');
  assert.equal(custom?.authScheme, undefined);
  assert.equal(loadConfig({ ...env, CONFLUENCE_AUTH_SCHEME: 'Basic' }).confluence?.authScheme, 'Basic');
  assert.equal(loadConfig({ ...env, CONFLUENCE_AUTH_SCHEME: 'none' }).confluence?.authScheme, undefined);
});

test('confluence needs a token and an explicit tool list', () => {
  assert.throws(
    () => loadConfig({ ...base, CONFLUENCE_MCP_URL: 'ftp://x' }),
    (e: unknown) => e instanceof ConfigError
      && ['CONFLUENCE_MCP_URL must be', 'CONFLUENCE_READ_TOOLS is required', 'CONFLUENCE_TOKEN or CONFLUENCE_TOKEN_FILE is required'].every((s) => e.message.includes(s)),
  );
});

test('EFFORT is passed through, and only a known level', () => {
  assert.equal(loadConfig({ ...base, EFFORT: 'high' }).effort, 'high');
  assert.throws(() => loadConfig({ ...base, EFFORT: 'extreme' }), /EFFORT must be one of low, medium, high, xhigh, max/);
});
