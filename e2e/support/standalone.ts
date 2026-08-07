import { execFileSync } from "node:child_process";
import os from "node:os";
import path from "node:path";

// Config for the `sild-standalone` deployment suite, shared by playwright.config.ts
// (which boots the binary) and the specs (which run sild-admin against the same
// database) — so the CLI in a test can never talk to a different store.

// The tenant standalone bootstraps on an empty database. A production binary has
// no dev seed, so this is the suite's only way in.
export const BOOTSTRAP = {
  tenant: "E2E Standalone",
  email: "owner@standalone.test",
  password: "standalone-owner-pw",
} as const;

export const STANDALONE_DB_DSN =
  process.env.SILD_E2E_DB_DSN ||
  "host=localhost port=5432 user=sild password=sild dbname=sild sslmode=disable";

const BACKEND_DIR = "../backend";

// One id per run, fixed by whichever process loads this first (the config, in the
// main process) and inherited by the workers. Plain process.pid would differ
// between them, and global-teardown would delete paths nothing created.
const RUN_ID = (process.env.SILD_E2E_RUN_ID ??= String(process.pid));

const ADMIN_BIN = path.join(os.tmpdir(), `sild-admin-e2e-${RUN_ID}`);
const UPLOAD_DIR = path.join(os.tmpdir(), `sild-e2e-standalone-uploads-${RUN_ID}`);

// What this suite leaves in the temp dir — global-teardown removes them.
export const STANDALONE_TEMP_PATHS = [ADMIN_BIN, UPLOAD_DIR];

// sild-standalone requires Postgres (or MySQL) and Redis and refuses to start
// without them. Local storage is allowed only because one process with one temp
// dir genuinely is shared storage.
export const STANDALONE_ENV: Record<string, string> = {
  SILD_ENV: "production",
  DB_DRIVER: "postgres",
  DB_DSN: STANDALONE_DB_DSN,
  SILD_BROKER: "redis",
  SILD_REDIS_URL: process.env.SILD_E2E_REDIS_URL || "redis://localhost:6379",
  STORAGE_BACKEND: "local",
  STORAGE_LOCAL_SHARED: "true",
  STORAGE_SIGNING_KEY: "e2e-standalone-signing-key",
  STORAGE_LOCAL_DIR: UPLOAD_DIR,
  SILD_BOOTSTRAP_TENANT: BOOTSTRAP.tenant,
  SILD_BOOTSTRAP_ADMIN_EMAIL: BOOTSTRAP.email,
  SILD_BOOTSTRAP_ADMIN_NAME: "Standalone Owner",
  SILD_BOOTSTRAP_ADMIN_PASSWORD: BOOTSTRAP.password,
};

let built = false;

// runAdmin invokes the operator CLI the way an operator would — a separate process
// against the same database as the running server. Built once: `go run` would
// re-link it on every call, on the worker thread.
export function runAdmin(args: string[], stdin?: string): string {
  if (!built) {
    execFileSync("go", ["build", "-o", ADMIN_BIN, "./cmd/sild-admin"], {
      cwd: BACKEND_DIR,
      stdio: ["ignore", "pipe", "inherit"],
    });
    built = true;
  }
  return execFileSync(ADMIN_BIN, args, {
    cwd: BACKEND_DIR,
    env: { ...process.env, ...STANDALONE_ENV },
    encoding: "utf8",
    input: stdin,
    stdio: stdin === undefined ? ["ignore", "pipe", "pipe"] : ["pipe", "pipe", "pipe"],
  });
}

// createTenant runs `tenant create` and reads back what it printed. The CLI's
// output is a contract, so it is parsed here once rather than per assertion.
export function createTenant(name: string, email: string) {
  const out = runAdmin(["tenant", "create", "--name", name, "--admin-email", email]);
  const field = (label: string) => new RegExp(`${label}\\s+(\\S+)`).exec(out)?.[1];
  return {
    out,
    tenantId: field("tenant_id"),
    apiKey: field("api_key"),
    forwardingAddress: field("forwarding_address"),
  };
}
