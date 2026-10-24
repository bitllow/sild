import { spawn } from 'node:child_process';
import { randomUUID } from 'node:crypto';
import path from 'node:path';
import type { SpawnOptions, SpawnedProcess } from '@anthropic-ai/claude-agent-sdk';

export type Mount = { path: string; readOnly: boolean };

export type DockerRunner = {
  image: string;
  // Bind-mounted at the same path inside the container, so the guard on the
  // host and the CLI in the container see one set of paths.
  mounts: Mount[];
  hostname?: string;
  network?: string;
  onArgs?: (args: string[]) => void;
};

// Runs the Claude Code CLI in a throwaway container instead of on the host.
export function dockerSpawner(runner: DockerRunner): (opts: SpawnOptions) => SpawnedProcess {
  return (opts) => {
    const name = `sild-runner-${randomUUID()}`;
    const cliArgs = stripLauncher(opts.command, opts.args);
    runner.onArgs?.(cliArgs);
    const envKeys = Object.keys(opts.env).filter((k) => opts.env[k] !== undefined);

    const args = [
      'run', '--rm', '-i', '--name', name,
      '--read-only', '--tmpfs', '/tmp', '--tmpfs', '/home/node:uid=1000,gid=1000',
      '--cap-drop', 'ALL', '--security-opt', 'no-new-privileges',
      '--pids-limit', '256', '--memory', '1g',
      // Docker Desktop resolves this name itself; Linux engines need it mapped.
      '--add-host=host.docker.internal:host-gateway',
      ...(runner.hostname ? ['--hostname', runner.hostname] : []),
      ...(runner.network ? ['--network', runner.network] : []),
      ...(opts.cwd ? ['-w', opts.cwd] : []),
      ...runner.mounts.flatMap((m) => ['-v', `${m.path}:${m.path}${m.readOnly ? ':ro' : ''}`]),
      // Names only: values come from the docker client's env, so they never sit in argv.
      ...envKeys.flatMap((k) => ['-e', k]),
      '-e', 'HOME=/home/node',
      runner.image,
      ...cliArgs,
    ];

    const child = spawn('docker', args, {
      env: { ...process.env, ...opts.env },
      stdio: ['pipe', 'pipe', 'pipe'],
    });
    child.stderr.on('data', () => {});

    // Killing the docker client does not stop the container; remove it by name.
    // Async: the model proxy shares this event loop.
    const kill = child.kill.bind(child);
    child.kill = ((signal?: NodeJS.Signals) => {
      spawn('docker', ['rm', '-f', name], { stdio: 'ignore' }).on('error', () => {});
      return kill(signal);
    }) as typeof child.kill;
    opts.signal.addEventListener('abort', () => child.kill('SIGTERM'), { once: true });

    return child as unknown as SpawnedProcess;
  };
}

// Null when name is a docker network without a route out, otherwise why not.
export async function internalNetworkProblem(name: string): Promise<string | null> {
  const out = await new Promise<{ code: number | null; text: string }>((resolve) => {
    const child = spawn('docker', ['network', 'inspect', '-f', '{{.Internal}}', name], { stdio: ['ignore', 'pipe', 'pipe'] });
    let text = '';
    child.stdout.on('data', (b: Buffer) => (text += b.toString()));
    child.on('error', () => resolve({ code: -1, text: '' }));
    child.on('close', (code) => resolve({ code, text: text.trim() }));
  });
  if (out.code !== 0) return `docker network ${name} does not exist`;
  return out.text === 'true' ? null : `docker network ${name} is not internal (create it with docker network create --internal)`;
}

// The SDK launches either the native binary or `node cli.js`; only the CLI's own args cross over.
function stripLauncher(command: string, args: string[]): string[] {
  const launcher = path.basename(command);
  if (['node', 'bun', 'deno'].includes(launcher) && args.length > 0) return args.slice(1);
  return args;
}
