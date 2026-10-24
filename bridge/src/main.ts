import { randomBytes } from 'node:crypto';
import { connect, createHttp, principal as getPrincipal, realtimeToken, type Http } from '@sild/client';
import { Centrifuge } from 'centrifuge';
import { ConfigError, loadConfig, type Config } from './config.ts';
import type { InfoBroker } from './contracts.ts';
import { createConfluenceTools } from './runner/confluence.ts';
import { startModelProxy } from './runner/proxy.ts';
import { ClaudeSessionRunner } from './runner/session.ts';
import { loadDataConfig, makeBroker, type DataConfig } from './data.ts';
import { SlotManager } from './slots/manager.ts';
import { NullBroker } from './slots/null-broker.ts';
import type { Log } from './slots/types.ts';

export type BrokerFactory = (ctx: { config: Config; sild: Http; modelBaseUrl: string; modelToken: string; log: Log }) => InfoBroker | Promise<InfoBroker>;

export type Bridge = { stop(): Promise<void> };

export const log: Log = (msg, fields) => {
  console.log(JSON.stringify({ ts: new Date().toISOString(), msg, ...fields }));
};

export async function startBridge(cfg: Config, makeBroker: BrokerFactory = () => new NullBroker()): Promise<Bridge> {
  const sild = createHttp({
    baseUrl: cfg.sildUrl,
    credential: { kind: 'bearer', token: cfg.serviceKey },
    timeoutMs: 15_000,
  });
  const principal = await getPrincipal(sild);
  const me = principal.subject?.id;
  if (principal.kind !== 'admin' || !me) throw new Error('SILD_SERVICE_KEY must be a service account key');

  const proxyToken = randomBytes(32).toString('base64url');
  const proxy = await startModelProxy({ auth: cfg.modelAuth, token: proxyToken, host: cfg.proxy.host, port: cfg.proxy.port, log });
  const proxyHost = cfg.runnerMode === 'docker' ? 'host.docker.internal' : '127.0.0.1';
  const confluence = createConfluenceTools(cfg.confluence, log);
  const broker = await makeBroker({ config: cfg, sild, modelBaseUrl: `http://127.0.0.1:${proxy.port}`, modelToken: proxyToken, log });
  const runner = new ClaudeSessionRunner({
    workRoot: cfg.workRoot,
    srcDir: cfg.srcDir,
    mode: cfg.runnerMode,
    image: cfg.runnerImage,
    network: cfg.runnerNetwork,
    model: cfg.model,
    maxTurns: cfg.maxTurns,
    maxBudgetUsd: cfg.maxBudgetUsd,
    baseUrl: cfg.proxy.runnerUrl ?? `http://${proxyHost}:${proxy.port}`,
    proxyToken,
    confluence,
    instructions: () => broker.instructions(),
    log,
  });

  const slots = new SlotManager(sild, me, runner, broker, {
    queue: cfg.queue,
    slots: cfg.slots,
    idleReleaseMs: cfg.idleReleaseMs,
    maxClaimMs: cfg.maxClaimMs,
    turnTimeoutMs: cfg.turnTimeoutMs,
    pollMs: cfg.pollMs,
    log,
  });
  slots.start();
  const realtime = connect({
    Centrifuge,
    url: cfg.sildWsUrl,
    getToken: async () => (await realtimeToken(sild)).token,
    onConnected: () => slots.onConnected(),
    onEvent: (channel, env) => slots.onEvent(channel, env),
    log,
  });
  log('bridge started', { me, tenant: principal.tenant_id, queue: cfg.queue, slots: cfg.slots, mode: cfg.runnerMode });

  return {
    stop: async () => {
      realtime.close();
      await slots.stop();
      await proxy.close();
      await confluence?.close();
    },
  };
}

if (import.meta.main) {
  let cfg: Config;
  let data: DataConfig | null;
  try {
    cfg = loadConfig();
    data = loadDataConfig();
  } catch (e) {
    if (!(e instanceof ConfigError)) throw e;
    console.error(e.message);
    process.exit(1);
  }
  const bridge = await startBridge(cfg, data ? (ctx) => makeBroker(data, ctx) : undefined);
  const shutdown = () => {
    log('bridge stopping');
    bridge.stop().then(() => process.exit(0), () => process.exit(1));
  };
  process.once('SIGINT', shutdown);
  process.once('SIGTERM', shutdown);
}
