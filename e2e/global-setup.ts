import { execFile } from "node:child_process";
import { promisify } from "node:util";
import path from "node:path";

const run = promisify(execFile);

// Build the drop-in widget bundle before the backend compiles, so the embedded
// /widget.js the tests exercise reflects the current web/ source. Skippable via
// SILD_E2E_SKIP_BUILD=1 when iterating and the bundle is already fresh.
export default async function globalSetup() {
  if (process.env.SILD_E2E_SKIP_BUILD === "1") return;
  const webDir = path.resolve(__dirname, "../web");
  try {
    await run(process.execPath, ["build.mjs"], { cwd: webDir });
    // eslint-disable-next-line no-console
    console.log("[e2e] built widget bundle → backend/internal/webasset/widget.js");
  } catch (e) {
    // eslint-disable-next-line no-console
    console.error(
      "[e2e] widget build failed — is web/ installed? (cd web && npm ci)\n",
      e instanceof Error ? e.message : e
    );
    throw e;
  }
}
