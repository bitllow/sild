import { expect, type Page, type Locator } from "@playwright/test";

// Realtime propagation (widget SSE / inbox WS through the in-memory broker) is
// fast but not instant. Give realtime-driven assertions a generous ceiling —
// Playwright polls until it passes, so a healthy path resolves in well under a
// second; the ceiling only bites on genuine failures.
export const REALTIME_TIMEOUT = 15_000;

// Wait for a locator to become visible via a realtime update (no reload).
export async function expectRealtimeVisible(locator: Locator): Promise<void> {
  await expect(locator).toBeVisible({ timeout: REALTIME_TIMEOUT });
}

// Assert that something does NOT appear over realtime within a short, bounded
// window (used for negative checks like "internal note must not reach the
// widget"). Kept short so the suite stays fast while still being meaningful.
export async function expectNeverAppears(locator: Locator, within = 2500): Promise<void> {
  await expect(locator).toHaveCount(0);
  // eslint-disable-next-line playwright/no-wait-for-timeout
  await locator.page().waitForTimeout(within);
  await expect(locator).toHaveCount(0);
}

// Poll an async predicate until true (for conditions not tied to a single DOM
// node — e.g. "the queue count went up").
export async function pollUntil(
  page: Page,
  predicate: () => Promise<boolean>,
  message = "condition"
): Promise<void> {
  await expect
    .poll(async () => predicate(), { message, timeout: REALTIME_TIMEOUT })
    .toBeTruthy();
}
