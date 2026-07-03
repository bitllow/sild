import { rm } from "node:fs/promises";
import os from "node:os";
import path from "node:path";

// Remove the per-run temp SQLite files + local upload dir. The backend process
// itself is torn down by Playwright's webServer manager.
export default async function globalTeardown() {
  const db = process.env.SILD_E2E_DB_DSN;
  const targets: string[] = [];
  if (db && path.isAbsolute(db)) {
    targets.push(db, `${db}-wal`, `${db}-shm`, `${db}-journal`);
  }
  targets.push(path.join(os.tmpdir(), `sild-e2e-uploads-${process.pid}`));
  await Promise.all(targets.map((t) => rm(t, { recursive: true, force: true }).catch(() => {})));
}
