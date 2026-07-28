import { test, expect } from "../../fixtures";
import { gotoInbox, openConversation, conversationIdByText } from "../../support/inbox";
import { seedManyMessages } from "../../support/backdoor";
import { createConversation } from "../../support/appdata";
import { uid } from "../../support/env";

// A thread is fetched with ?limit=100 and the endpoint returns a cursor for the rest.
// Nothing consumed that cursor, so every conversation past 100 messages was
// permanently truncated to its newest 100 — on a healthy connection, with no outage
// involved.
//
// The thread is deliberately ten pages deep: two would pass on an implementation that
// pages exactly once, and draining this one only works if the cursor advances every
// time. The opening message is the anchor because it is the only one whose position is
// guaranteed — it was sent through the widget before any seeding.
const PAGE_SIZE = 100;
const PAGES = 10;

test.describe("inbox · thread history", () => {
  test("scrolling up drains a ten-page thread to its first message", async ({
    page,
    request,
    browser,
  }) => {
    test.slow(); // seeding a thousand messages, then paging back through them

    const label = uid("hist");
    const opening = `${label} opening`;
    const conv = await createConversation(browser, { body: opening });

    await gotoInbox(page);
    const id = await conversationIdByText(page, opening);
    await seedManyMessages(request, id, PAGE_SIZE * PAGES, label);

    // By id, not by text: seeding moved the queue row's preview off the opening body.
    await page.reload();
    await openConversation(page, id);

    // Scope to the transcript — a body also renders in the queue row's preview.
    const transcript = page.getByTestId("thread-transcript");
    const openingBubble = transcript.getByText(opening, { exact: true });
    const older = page.getByTestId("thread-older");

    // The truncation itself: the first message is nowhere near the first page.
    await expect(older, "earlier messages are offered").toBeVisible();
    await expect(openingBubble).toHaveCount(0);

    // The thread opens at the newest message, so scrolling up is the real gesture.
    // Each scroll to the top prepends one page; repeat until the opening message is
    // reachable, which can only happen if the cursor advanced every time.
    await expect
      .poll(
        async () => {
          await transcript.evaluate((el) => {
            el.scrollTop = 0;
          });
          return openingBubble.count();
        },
        {
          message: `scrolling up reaches the first of ${PAGE_SIZE * PAGES} messages`,
          timeout: 120_000,
        }
      )
      .toBeGreaterThan(0);

    // Nothing older is left — the drain finished rather than stalling mid-thread.
    await expect(older, "the thread is whole").toHaveCount(0);

    await conv.context.close();
  });
});
