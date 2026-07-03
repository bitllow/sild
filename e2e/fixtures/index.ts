import { test as base, expect, type BrowserContext } from "@playwright/test";
import { createConversation, type AppConversation } from "../support/appdata";

export interface AppFactory {
  // Create a conversation by driving the real widget (see support/appdata).
  // Contexts are tracked and closed automatically at the end of the test.
  conversation(opts?: {
    body?: string;
    metadata?: Record<string, unknown>;
    mode?: "user" | "guest";
  }): Promise<AppConversation>;
}

type Fixtures = {
  app: AppFactory;
};

const COVERAGE = process.env.E2E_COVERAGE === "1";

export const test = base.extend<Fixtures>({
  // When E2E_COVERAGE=1, collect V8 JS coverage from the inbox page and feed it
  // to monocart-reporter, which source-maps it back to inbox/src. No-op (and
  // zero overhead) otherwise, and only on Chromium (page.coverage).
  page: async ({ page }, use, testInfo) => {
    const canCover = COVERAGE && typeof page.coverage?.startJSCoverage === "function";
    if (canCover) {
      await page.coverage.startJSCoverage({ resetOnNavigation: false });
    }
    await use(page);
    if (canCover) {
      const entries = await page.coverage.stopJSCoverage();
      const { addCoverageReport } = await import("monocart-reporter");
      await addCoverageReport(entries, testInfo);
    }
  },

  app: async ({ browser }, use) => {
    const contexts: BrowserContext[] = [];
    await use({
      async conversation(opts = {}) {
        const c = await createConversation(browser, opts);
        contexts.push(c.context);
        return c;
      },
    });
    await Promise.all(contexts.map((c) => c.close().catch(() => {})));
  },
});

export { expect };
