import type { Page } from "@playwright/test";
import { test, expect } from "../../fixtures";
import { uid } from "../../support/env";
import {
  gotoInbox,
  openConversation,
  openConversationByText,
  conversationIdByText,
  messageWithText,
  rowByText,
} from "../../support/inbox";
import { widgetComposer, widgetSend, widgetBubble } from "../../support/widget";
import { seedManyMessages } from "../../support/backdoor";
import { expectRealtimeVisible, expectNeverAppears, REALTIME_TIMEOUT } from "../../support/realtime";

// One full catch-up page plus one, so the drain only finishes if it asks again from
// the last id it received.
const GAP = 101;

// Collect the catch-up calls: the 30s safety reconcile recovers the same messages
// without a ?since=, so recovery has to be attributed, not inferred from it arriving.
function recordCatchUps(page: Page): string[] {
  const urls: string[] = [];
  page.on("request", (r) => {
    if (r.url().includes("since=")) urls.push(r.url());
  });
  return urls;
}

// Each test proves the outage before it proves the recovery: without that negative
// half they would all pass on a socket that never dropped.
test.describe("cross-surface reconnect catch-up", () => {
  test("messages sent during a socket outage arrive when the inbox reconnects", async ({
    page,
    app,
  }) => {
    const label = uid("outage");
    const conv = await app.conversation({ body: `${label} opening` });

    await gotoInbox(page);
    await openConversationByText(page, conv.body);
    // Catch-up resumes from the last id the thread holds, so it must hold one.
    await expect(messageWithText(page, conv.body)).toBeVisible();

    const catchUps = recordCatchUps(page);

    // Chromium closes the open WebSocket with the network; the widget's context is
    // untouched and stays live.
    await page.context().setOffline(true);

    const duringOutage = `${label} while-offline`;
    await widgetComposer(conv.page).fill(duringOutage);
    await widgetSend(conv.page).click();
    await expect(widgetBubble(conv.page, duringOutage), "the user's send succeeded").toBeVisible();

    // Kept short: the client drops the socket on the browser's offline event, so a
    // longer window only costs the suite time.
    await expectNeverAppears(messageWithText(page, duringOutage), 1000);

    await page.context().setOffline(false);

    await expectRealtimeVisible(messageWithText(page, duringOutage));
    expect(catchUps.length, "the gap arrived over ?since=, not the safety reconcile").toBeGreaterThan(0);

    // The resubscribed socket delivers again — not just the gap.
    const afterReconnect = `${label} after-reconnect`;
    await widgetComposer(conv.page).fill(afterReconnect);
    await widgetSend(conv.page).click();
    await expectRealtimeVisible(messageWithText(page, afterReconnect));
  });

  test("a gap longer than one page is drained to its end", async ({ page, request, app }) => {
    test.slow(); // the gap is seeded one message at a time while the socket is down

    const label = uid("gap");
    const conv = await app.conversation({ body: `${label} opening` });

    await gotoInbox(page);
    const id = await conversationIdByText(page, conv.body);
    await openConversation(page, id);
    await expect(messageWithText(page, conv.body)).toBeVisible();

    const catchUps = recordCatchUps(page);

    const seeded = messageWithText(page, `${label} #`);
    await page.context().setOffline(true);
    // The admin context is a separate one, so the inbox's outage does not touch it.
    await seedManyMessages(request, id, GAP, label);
    await expect(seeded, "the gap is invisible while the socket is down").toHaveCount(0);

    await page.context().setOffline(false);

    await expect(seeded, "every page of the gap lands").toHaveCount(GAP, {
      timeout: REALTIME_TIMEOUT,
    });
    expect(catchUps.length, "the drain asked again from the first page's last id").toBeGreaterThan(1);
  });

  test("a conversation started during the outage is in the queue after reconnect", async ({
    page,
    app,
  }) => {
    const label = uid("queue");

    await gotoInbox(page);
    await page.context().setOffline(true);

    // The widget context is untouched, so the visitor opens a thread the agent's
    // socket will never be told about.
    const conv = await app.conversation({ body: `${label} opening` });
    await expect(rowByText(page, conv.body), "the queue cannot know about it yet").toHaveCount(0);

    await page.context().setOffline(false);

    // Nothing else refetches the queue, so the row can only come from the reconnect.
    await expectRealtimeVisible(rowByText(page, conv.body).first());
  });
});
