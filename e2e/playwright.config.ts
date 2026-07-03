import { defineConfig, devices } from "@playwright/test";
import os from "node:os";
import path from "node:path";

// The two product surfaces both talk to one zero-infra `sild-dev` backend.
export const INBOX_URL = process.env.SILD_E2E_INBOX_URL || "http://localhost:3000";
export const BACKEND_URL = process.env.SILD_E2E_BACKEND_URL || "http://localhost:8080";

// The backend DB is configurable so the suite can run against the production
// target (Postgres, with full trigram search) as well as the zero-infra default
// (SQLite). Default: a fresh temp SQLite file per run so the first-run devSeed
// always fires with deterministic data and no stray backend/sild.db interferes.
const DB_DRIVER = process.env.SILD_E2E_DB_DRIVER || "sqlite";
const DB_DSN =
  process.env.SILD_E2E_DB_DSN ||
  (DB_DRIVER === "sqlite"
    ? path.join(os.tmpdir(), `sild-e2e-${Date.now()}-${process.pid}.db`)
    : "host=localhost port=5432 user=sild password=sild dbname=sild sslmode=disable");
process.env.SILD_E2E_DB_DSN = DB_DSN;
process.env.SILD_E2E_DB_DRIVER = DB_DRIVER;

const isCI = !!process.env.CI;
// Opt-in browser-code coverage (inbox client JS) via monocart. See fixtures.
const COVERAGE = process.env.E2E_COVERAGE === "1";

const reporters: import("@playwright/test").ReporterDescription[] = [["html", { open: "never" }]];
reporters.push(isCI ? ["line"] : ["list"]);
if (COVERAGE) {
  reporters.push([
    "monocart-reporter",
    {
      name: "Sild E2E coverage",
      outputFile: "./monocart/index.html",
      coverage: {
        outputDir: "./coverage",
        reports: [["v8"], ["console-summary"]],
        // Only report on the app's own source (inbox client + widget), skipping
        // framework/node_modules and Next internals.
        sourceFilter: (path: string) =>
          (path.includes("/inbox/src/") || path.includes("/web/src/")) &&
          !path.includes("node_modules"),
      },
    },
  ]);
}

export default defineConfig({
  testDir: "./specs",
  // globalSetup builds the widget bundle so the embedded /widget.js is current
  // before `go run` compiles the backend.
  globalSetup: require.resolve("./global-setup"),
  globalTeardown: require.resolve("./global-teardown"),
  // One shared backend + DB; isolation is by unique per-test ids, not DB reset.
  fullyParallel: true,
  forbidOnly: isCI,
  retries: isCI ? 2 : 0,
  // Free GitHub runners are 2 vCPU — the backend, Next dev server and Chromium
  // already contend for them, so run specs serially in CI to stay stable and
  // keep realtime timing deterministic. Locally, use the default worker count.
  workers: isCI ? 1 : undefined,
  reporter: reporters,
  timeout: 45_000,
  expect: { timeout: 10_000 },

  use: {
    baseURL: INBOX_URL,
    trace: "on-first-retry",
    screenshot: "only-on-failure",
    video: "retain-on-failure",
    actionTimeout: 15_000,
  },

  projects: [
    // Logs the admin in once and saves the session cookie for the inbox/cross
    // projects. Runs first via `dependencies`.
    { name: "setup", testDir: "./setup", testMatch: /admin\.setup\.ts/ },

    {
      name: "inbox",
      testDir: "./specs/inbox",
      dependencies: ["setup"],
      use: { ...devices["Desktop Chrome"], baseURL: INBOX_URL, storageState: ".auth/agent.json" },
    },
    {
      name: "widget",
      testDir: "./specs/widget",
      use: { ...devices["Desktop Chrome"], baseURL: BACKEND_URL },
    },
    {
      // Cross-surface flows open a second (widget) context themselves; the base
      // context is the authed agent inbox.
      name: "cross",
      testDir: "./specs/cross",
      dependencies: ["setup"],
      use: { ...devices["Desktop Chrome"], baseURL: INBOX_URL, storageState: ".auth/agent.json" },
    },
  ],

  webServer: [
    {
      // Zero-infra dev backend: SQLite (fresh temp file) + in-memory broker +
      // in-process worker/SMTP. NOT `make dev` (that uses Postgres/Redis).
      // Override the command via SILD_E2E_BACKEND_CMD (e.g. a prebuilt binary)
      // if `go run` is unavailable locally.
      command: process.env.SILD_E2E_BACKEND_CMD || "go run ./cmd/sild-dev",
      cwd: "../backend",
      url: `${BACKEND_URL}/sild-demo`,
      reuseExistingServer: !isCI,
      timeout: 180_000,
      stdout: "pipe",
      stderr: "pipe",
      env: {
        ...process.env,
        DB_DRIVER,
        DB_DSN,
        SILD_ENV: "development",
        SILD_BROKER: "memory",
        STORAGE_BACKEND: "local",
        // Keep uploads out of the repo working tree.
        STORAGE_LOCAL_DIR: path.join(os.tmpdir(), `sild-e2e-uploads-${process.pid}`),
      },
    },
    {
      command: "npm run dev",
      cwd: "../inbox",
      url: INBOX_URL,
      reuseExistingServer: !isCI,
      timeout: 180_000,
      env: { ...process.env, SILD_API_URL: BACKEND_URL, SILD_DISABLE_DEV_INDICATOR: "1" },
    },
  ],
});
