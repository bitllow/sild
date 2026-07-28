import { defineConfig, devices } from "@playwright/test";
import os from "node:os";
import path from "node:path";
import { STANDALONE_ENV } from "./support/standalone";

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

// Opt-in: run the deployment suite against `sild-standalone` instead of the
// product suites against `sild-dev`. Separate because the binary is a production
// one — it requires Postgres AND Redis, refuses to start without them, and has no
// dev seed. Needs both services running; CI gives it its own job.
const STANDALONE = process.env.SILD_E2E_STANDALONE === "1";

// The widget bundle is embedded into the Go backend at COMPILE time
// (`//go:embed widget.js`), so it must be rebuilt BEFORE `go run` compiles.
// Playwright starts `webServer` plugins before `globalSetup`, so the build
// can't live in globalSetup — it would run after the backend was already
// compiled, serving the previous run's bundle. So it's the first step of the
// backend command below. CI pre-builds and sets SILD_E2E_SKIP_BUILD=1; a
// prebuilt SILD_E2E_BACKEND_CMD also skips it (the bundle is baked into it).
const SKIP_BUILD = process.env.SILD_E2E_SKIP_BUILD === "1";

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
  // The widget bundle is (re)built as the first step of the backend command
  // (see SKIP_BUILD above) — it has to happen before `go run` compiles, which
  // is earlier than any globalSetup could run.
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

  projects: STANDALONE
    ? [
        {
          name: "standalone",
          testDir: "./specs/standalone",
          use: { ...devices["Desktop Chrome"], baseURL: BACKEND_URL },
        },
      ]
    : [
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

  webServer: STANDALONE
    ? [
        {
          // sild-migrate owns the schema, always and everywhere (§4) — standalone
          // refuses to migrate, so it has to run first. The widget bundle is built
          // ahead of `go run` for the same reason as below (it is embedded at
          // compile time). No inbox here: this project asserts the deployment
          // shape, not the UI.
          command:
            (SKIP_BUILD ? "" : "(cd ../web && node build.mjs) && ") +
            "go run ./cmd/sild-migrate && go run ./cmd/sild-standalone",
          cwd: "../backend",
          url: `${BACKEND_URL}/healthz`,
          reuseExistingServer: !isCI,
          timeout: 180_000,
          stdout: "pipe",
          stderr: "pipe",
          env: { ...process.env, ...STANDALONE_ENV },
        },
      ]
    : [
    {
      // Zero-infra dev backend: SQLite (fresh temp file) + in-memory broker +
      // in-process worker/SMTP. NOT `make dev` (that uses Postgres/Redis).
      // Override the command via SILD_E2E_BACKEND_CMD (e.g. a prebuilt binary)
      // if `go run` is unavailable locally. The widget bundle is rebuilt first
      // (unless SKIP_BUILD) so `go run` embeds the current web/ source; the
      // build runs in ../web then `go run` in this cwd (../backend).
      command:
        process.env.SILD_E2E_BACKEND_CMD ||
        (SKIP_BUILD
          ? "go run ./cmd/sild-dev"
          : "(cd ../web && node build.mjs) && go run ./cmd/sild-dev"),
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
