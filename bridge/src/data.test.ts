import assert from 'node:assert/strict';
import { test } from 'node:test';
import { ConfigError, loadConfig } from './config.ts';
import { checkRouterAuth, loadDataConfig } from './data.ts';

const data = { ADAPTER_URL: 'https://erp.example.com/sild', ADAPTER_SECRET: 's', ADAPTER_PUBLIC_KEY: 'k' };
const base = { SILD_URL: 'https://chat.example.com', SILD_SERVICE_KEY: 'sild_live_x', RUNNER_EGRESS: 'open' };

test('the router uses the proxy unless it has a key of its own', () => {
  assert.equal(loadDataConfig(data)!.routerApiKey, undefined);
  assert.equal(loadDataConfig({ ...data, ROUTER_API_KEY: 'sk-router' })!.routerApiKey, 'sk-router');
});

test('a model router on a subscription token needs its own key', () => {
  const oauth = { ...loadConfig({ ...base, ANTHROPIC_API_KEY: 'x' }), modelAuth: { kind: 'oauth' as const, token: 't' } };
  assert.throws(() => checkRouterAuth(oauth, loadDataConfig(data)!), ConfigError);
  checkRouterAuth(oauth, loadDataConfig({ ...data, ROUTER_API_KEY: 'sk-router' })!);
  checkRouterAuth(oauth, loadDataConfig({ ...data, ROUTER: 'fake' })!);
  checkRouterAuth(loadConfig({ ...base, ANTHROPIC_API_KEY: 'x' }), loadDataConfig(data)!);
});
