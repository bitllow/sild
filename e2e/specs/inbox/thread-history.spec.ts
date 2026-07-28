import { test, expect } from "../../fixtures";
import { gotoInbox, openConversation, conversationIdByText } from "../../support/inbox";
import { seedManyMessages } from "../../support/backdoor";
import { createConversation } from "../../support/appdata";
import { uid } from "../../support/env";

// A thread is fetched with ?limit=100 and the endpoint returns a cursor for the rest.
// Nothing consumed that cursor, so every conversation past 100 messages was
// permanently truncated to its newest 100 — on a healthy connection, with no outage
// involved. This drives the real scroll-up path that fixes it.
test.describe("inbox · thread history", () => {
  test("a thread past the first page can be read back to its oldest message", async ({ page, request, browser }) => {
    const label = uid("hist");
    const conv = await createConversation(browser, { body: `${label} opening` });

    await gotoInbox(page);
    const id = await conversationIdByText(page, `${label} opening`);

    // 100 is the page size, so 110 puts the oldest messages — and the opening one —
    // beyond the first page.
    const seeded = await seedManyMessages(request, id, 110, label);
    const oldestSeeded = seeded[0];
    const newestSeeded = seeded[seeded.length - 1];

    // By id, not by text: seeding moved the queue row's preview to the newest
    // message, so the opening body no longer identifies the row.
    await page.reload();
    await openConversation(page, id);

    // Scope to the transcript: a body also renders in the queue row's preview.
    const transcript = page.getByTestId("thread-transcript");
    await expect(transcript.getByText(newestSeeded, { exact: true }), "the newest page renders").toBeVisible();

    // The truncation itself: the oldest messages are NOT in the first page.
    await expect(transcript.getByText(oldestSeeded, { exact: true })).toHaveCount(0);
    const loadOlder = page.getByTestId("thread-load-older");
    await expect(loadOlder, "the thread offers to load earlier messages").toBeVisible();

    // Each click prepends one page; the control disappears once the thread is whole.
    for (let i = 0; i < 8; i++) {
      if (await transcript.getByText(oldestSeeded, { exact: true }).count()) break;
      if (!(await loadOlder.count())) break;
      await loadOlder.click();
      // Settle: the label reverts, or the control disappears because the thread is
      // now whole. Either way the in-flight page has been applied.
      await expect(page.getByText("Loading earlier messages…")).toHaveCount(0);
    }

    await expect(
      transcript.getByText(oldestSeeded, { exact: true }),
      "the oldest seeded message is reachable by scrolling up"
    ).toBeVisible();
    await expect(
      transcript.getByText(`${label} opening`, { exact: true }),
      "and so is the message that opened the conversation"
    ).toBeVisible();

    await conv.context.close();
  });
});
