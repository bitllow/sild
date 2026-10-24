import path from 'node:path';
import { AdapterClient, type Assert } from './adapter/client.ts';
import type { FileStore } from './adapter/files.ts';
import { createBroker, DEFAULT_BROKER_LIMITS } from './broker/broker.ts';
import { ConfigError, loadSecret, type Config } from './config.ts';
import type { InfoBroker } from './contracts.ts';
import { AnthropicRouter, DEFAULT_ROUTER_MODEL } from './router/anthropic.ts';
import { FakeRouter } from './router/fake.ts';
import type { Router } from './router/router.ts';
import type { Log } from './slots/types.ts';
import { JsonStore } from './state.ts';

export type DataConfig = {
  adapterUrl: string;
  adapterSecret: string;
  adapterPublicKey: string;
  allowInsecureLocal: boolean;
  router: 'anthropic' | 'fake';
  routerModel: string;
  // Unset: the router reaches the model through the bridge's proxy and its credential.
  routerApiKey?: string;
  // Different ids one conversation may request data about.
  maxRequestIds: number;
};

// Without ADAPTER_URL the bridge runs with no data provider.
export function loadDataConfig(env: NodeJS.ProcessEnv = process.env): DataConfig | null {
  const url = env.ADAPTER_URL?.trim();
  if (!url) return null;
  const problems: string[] = [];
  const needed = ' with ADAPTER_URL';
  const secret = loadSecret(env, problems, 'ADAPTER_SECRET', { needed })?.value ?? '';
  const publicKey = loadSecret(env, problems, 'ADAPTER_PUBLIC_KEY', { needed })?.value ?? '';
  const router = env.ROUTER?.trim() || 'anthropic';
  if (router !== 'anthropic' && router !== 'fake') problems.push('ROUTER must be anthropic or fake');
  const routerKey = env.ROUTER_API_KEY?.trim() || env.ROUTER_API_KEY_FILE?.trim()
    ? loadSecret(env, problems, 'ROUTER_API_KEY')?.value
    : undefined;
  const maxIds = Number(env.MAX_REQUEST_IDS?.trim() || DEFAULT_BROKER_LIMITS.distinctIds);
  if (!Number.isInteger(maxIds) || maxIds < 1) problems.push('MAX_REQUEST_IDS must be a whole number ≥ 1');
  if (problems.length) throw new ConfigError(`invalid data provider configuration:\n  ${problems.join('\n  ')}`);
  return {
    adapterUrl: url,
    adapterSecret: secret,
    adapterPublicKey: publicKey,
    allowInsecureLocal: env.ADAPTER_ALLOW_INSECURE_LOCAL === '1',
    router: router as DataConfig['router'],
    routerModel: env.ROUTER_MODEL?.trim() || DEFAULT_ROUTER_MODEL,
    maxRequestIds: maxIds,
    ...(routerKey && { routerApiKey: routerKey }),
  };
}

// A subscription token serves only Claude Code, so a model router needs a key of its own.
export function checkRouterAuth(cfg: Config, data: DataConfig): void {
  if (data.router === 'anthropic' && !data.routerApiKey && cfg.modelAuth.kind === 'oauth') {
    throw new ConfigError('ROUTER=anthropic needs ROUTER_API_KEY when the model credential is a subscription token');
  }
}

// Without its own key the router reaches the model through the bridge's proxy, holding only the proxy token.
export function makeBroker(
  data: DataConfig,
  ctx: { config: Config; files: FileStore; assert: Assert; stateRoot: string; modelBaseUrl: string; modelToken: string; log: Log },
): InfoBroker {
  const log = (msg: string) => ctx.log(msg);
  const adapter = new AdapterClient({
    url: data.adapterUrl,
    secret: data.adapterSecret,
    publicKey: data.adapterPublicKey,
    files: ctx.files,
    assert: ctx.assert,
    state: new JsonStore(path.join(ctx.stateRoot, 'adapter')),
    allowInsecureLocal: data.allowInsecureLocal,
    log,
  });
  const router: Router = data.router === 'fake'
    ? new FakeRouter()
    : new AnthropicRouter(data.routerApiKey
      ? { apiKey: data.routerApiKey, model: data.routerModel }
      : { baseURL: ctx.modelBaseUrl, authToken: ctx.modelToken, model: data.routerModel });
  return createBroker({ adapter, router, log, limits: { distinctIds: data.maxRequestIds }, state: new JsonStore(path.join(ctx.stateRoot, 'broker')) });
}
