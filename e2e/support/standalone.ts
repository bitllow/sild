import { execFileSync } from "node:child_process";
import os from "node:os";
import path from "node:path";

// Configuration for the `sild-standalone` deployment suite, shared by
// playwright.config.ts (which boots the binary) and the specs (which run
// sild-admin against the same database). One source of truth, so the CLI in the
// test can never end up talking to a different store than the server.

// The tenant standalone bootstraps on an empty database. A production binary has
// no dev seed, so this is the suite's only way in.
export const BOOTSTRAP = {
  tenant: "E2E Standalone",
  email: "owner@standalone.test",
  password: "standalone-owner-pw",
} as const;

const DB_DSN =
  process.env.SILD_E2E_DB_DSN ||
  "host=localhost port=5432 user=sild password=sild dbname=sild sslmode=disable";

// sild-standalone requires Postgres (or MySQL) and Redis and refuses to start
// without them — that is the point of the binary, so the suite provides them
// rather than working around them. Local storage is allowed here only because one
// process with one temp dir genuinely is shared storage.
export const STANDALONE_ENV: Record<string, string> = {
  SILD_ENV: "production",
  DB_DRIVER: "postgres",
  DB_DSN,
  SILD_BROKER: "redis",
  SILD_REDIS_URL: process.env.SILD_E2E_REDIS_URL || "redis://localhost:6379",
  STORAGE_BACKEND: "local",
  STORAGE_LOCAL_SHARED: "true",
  STORAGE_SIGNING_KEY: "e2e-standalone-signing-key",
  STORAGE_LOCAL_DIR: path.join(os.tmpdir(), `sild-e2e-standalone-uploads-${process.pid}`),
  SILD_BOOTSTRAP_TENANT: BOOTSTRAP.tenant,
  SILD_BOOTSTRAP_ADMIN_EMAIL: BOOTSTRAP.email,
  SILD_BOOTSTRAP_ADMIN_NAME: "Standalone Owner",
  SILD_BOOTSTRAP_ADMIN_PASSWORD: BOOTSTRAP.password,
};

// runAdmin invokes the operator CLI the way an operator would — a separate
// process against the same database as the running server.
export function runAdmin(args: string[], stdin?: string): string {
  return execFileSync("go", ["run", "./cmd/sild-admin", ...args], {
    cwd: "../backend",
    env: { ...process.env, ...STANDALONE_ENV },
    encoding: "utf8",
    input: stdin,
    stdio: stdin === undefined ? ["ignore", "pipe", "pipe"] : ["pipe", "pipe", "pipe"],
  });
}
