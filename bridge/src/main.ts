import { randomBytes } from 'node:crypto';
import path from 'node:path';
import { connect, createHttp, principal as getPrincipal, realtimeToken, requesterAssertion } from '@sild/client';
import { Centrifuge } from 'centrifuge';
import type { Assert } from './adapter/client.ts';
import { DEFAULT_FILE_LIMITS, FileStore } from './adapter/files.ts';
import { ConfigError, loadConfig, type Config } from './config.ts';
import type { InfoBroker } from './contracts.ts';
import { createConfluenceTools } from './runner/confluence.ts';
import { internalNetworkProblem } from './runner/docker.ts';
import { startModelProxy } from './runner/proxy.ts';
import { ClaudeSessionRunner } from './runner/session.ts';
import { checkRouterAuth, loadDataConfig, makeBroker, type DataConfig } from './data.ts';
import { SlotManager } from './slots/manager.ts';
import { NullBroker } from './slots/null-broker.ts';
import { JsonStore } from './state.ts';
import type { Log } from './slots/types.ts';

export type BrokerFactory = (ctx: {
  files: FileStore;
  assert: Assert;
  // Where broker and adapter state that outlives the process lives.
  stateRoot: string;
  modelBaseUrl: string;
  modelToken: string;
  log: Log;
}) => InfoBroker | Promise<InfoBroker>;

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
  const files = new FileStore(cfg.workRoot, DEFAULT_FILE_LIMITS);
  const assert = async (conv: string, body: string) => (await requesterAssertion(sild, conv, body)).assertion;
  const stateRoot = path.join(cfg.workRoot, '.bridge');
  const broker = await makeBroker({ files, assert, stateRoot, modelBaseUrl: `http://${cfg.proxy.host}:${proxy.port}`, modelToken: proxyToken, log });
  const runner = new ClaudeSessionRunner({
    workRoot: cfg.workRoot,
    srcDir: cfg.srcDir,
    mode: cfg.runnerMode,
    image: cfg.runnerImage,
    network: cfg.runnerNetwork,
    model: cfg.model,
    effort: cfg.effort,
    maxTurns: cfg.maxTurns,
    maxBudgetUsd: cfg.maxBudgetUsd,
    baseUrl: cfg.proxy.runnerUrl ?? `http://${proxyHost}:${proxy.port}`,
    proxyToken,
    confluence,
    instructions: () => broker.instructions(),
    catalog: () => broker.catalog(),
    log,
  });

  const slots = new SlotManager(sild, me, runner, broker, files, {
    queue: cfg.queue,
    slots: cfg.slots,
    idleReleaseMs: cfg.idleReleaseMs,
    maxClaimMs: cfg.maxClaimMs,
    turnTimeoutMs: cfg.turnTimeoutMs,
    pollMs: cfg.pollMs,
    log,
    state: new JsonStore(path.join(stateRoot, 'steps')),
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
    if (data) checkRouterAuth(cfg, data);
  } catch (e) {
    if (!(e instanceof ConfigError)) throw e;
    console.error(e.message);
    process.exit(1);
  }
  const egress = cfg.runnerMode === 'docker' && cfg.runnerEgress === 'internal' ? await internalNetworkProblem(cfg.runnerNetwork!) : null;
  if (egress) {
    console.error(`invalid configuration:\n  RUNNER_NETWORK: ${egress}`);
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
