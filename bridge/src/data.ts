import { AdapterClient } from './adapter/client.ts';
import type { FileStore } from './adapter/files.ts';
import { createBroker } from './broker/broker.ts';
import { ConfigError, loadSecret, type Config } from './config.ts';
import type { InfoBroker } from './contracts.ts';
import { AnthropicRouter, DEFAULT_ROUTER_MODEL } from './router/anthropic.ts';
import { FakeRouter } from './router/fake.ts';
import type { Router } from './router/router.ts';
import type { Log } from './slots/types.ts';

export type DataConfig = {
  adapterUrl: string;
  adapterSecret: string;
  adapterPublicKey: string;
  allowInsecureLocal: boolean;
  router: 'anthropic' | 'fake';
  routerModel: string;
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
  if (problems.length) throw new ConfigError(`invalid data provider configuration:\n  ${problems.join('\n  ')}`);
  return {
    adapterUrl: url,
    adapterSecret: secret,
    adapterPublicKey: publicKey,
    allowInsecureLocal: env.ADAPTER_ALLOW_INSECURE_LOCAL === '1',
    router: router as DataConfig['router'],
    routerModel: env.ROUTER_MODEL?.trim() || DEFAULT_ROUTER_MODEL,
  };
}

// The router reaches the model through the bridge's own proxy, so it holds only the proxy token.
export function makeBroker(data: DataConfig, ctx: { config: Config; files: FileStore; modelBaseUrl: string; modelToken: string; log: Log }): InfoBroker {
  const log = (msg: string) => ctx.log(msg);
  const adapter = new AdapterClient({
    url: data.adapterUrl,
    secret: data.adapterSecret,
    publicKey: data.adapterPublicKey,
    files: ctx.files,
    allowInsecureLocal: data.allowInsecureLocal,
    log,
  });
  const router: Router = data.router === 'fake'
    ? new FakeRouter()
    : new AnthropicRouter({ baseURL: ctx.modelBaseUrl, authToken: ctx.modelToken, model: data.routerModel });
  return createBroker({ adapter, router, log });
}
