import { test, expect } from "../../fixtures";
import { uid } from "../../support/env";
import { gotoInbox, openConversationByText, messageWithText } from "../../support/inbox";
import { widgetComposer, widgetSend, widgetBubble } from "../../support/widget";
import { expectRealtimeVisible, expectNeverAppears } from "../../support/realtime";

// The negative half is what makes this an outage test: without it the whole thing
// passes on a socket that never dropped.
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

    // Chromium closes the open WebSocket with the network; the widget's context is
    // untouched and stays live.
    await page.context().setOffline(true);

    const duringOutage = `${label} while-offline`;
    await widgetComposer(conv.page).fill(duringOutage);
    await widgetSend(conv.page).click();
    await expect(widgetBubble(conv.page, duringOutage), "the user's send succeeded").toBeVisible();

    // Kept short: the window is the outage, and every second offline pushes the
    // socket's reconnect backoff further out.
    await expectNeverAppears(messageWithText(page, duringOutage), 1000);

    await page.context().setOffline(false);

    await expectRealtimeVisible(messageWithText(page, duringOutage));

    // The resubscribed socket delivers again — not just the gap.
    const afterReconnect = `${label} after-reconnect`;
    await widgetComposer(conv.page).fill(afterReconnect);
    await widgetSend(conv.page).click();
    await expectRealtimeVisible(messageWithText(page, afterReconnect));
  });
});
