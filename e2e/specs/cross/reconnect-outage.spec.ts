import { test, expect } from "../../fixtures";
import { uid } from "../../support/env";
import { createConversation } from "../../support/appdata";
import { gotoInbox, openConversationByText, messageWithText, sendReply } from "../../support/inbox";
import { widgetComposer, widgetSend, widgetBubble } from "../../support/widget";
import { expectRealtimeVisible, expectNeverAppears } from "../../support/realtime";

// The ?since= catch-up on reconnect is covered at the API layer on both sides, but
// nothing exercised the TRIGGER: every realtime spec asserts delivery over a live
// socket, which is the publication path, not recovery. A regression that stopped
// catch-up firing at all would have passed the whole suite.
//
// The outage is proven, not assumed: while the context is offline the message must
// NOT appear (so it cannot have arrived live), and only after the socket returns must
// it show up. Without the negative half this test could pass on a socket that never
// dropped, which is exactly the false pass it exists to prevent.
test.describe("cross-surface reconnect catch-up", () => {
  test("messages sent during a socket outage arrive when the inbox reconnects", async ({
    page,
    browser,
  }) => {
    const label = uid("outage");
    const conv = await createConversation(browser, { body: `${label} opening` });

    try {
      await gotoInbox(page);
      await openConversationByText(page, `${label} opening`);
      await expect(messageWithText(page, `${label} opening`)).toBeVisible();

      // Cut the agent's network. Chromium closes the open WebSocket with it; the
      // widget's context is untouched and stays live.
      await page.context().setOffline(true);

      const duringOutage = `${label} while-offline`;
      await widgetComposer(conv.page).fill(duringOutage);
      await widgetSend(conv.page).click();
      await expect(widgetBubble(conv.page, duringOutage), "the user's send succeeded").toBeVisible();

      // The outage is real: this cannot reach the agent live.
      await expectNeverAppears(messageWithText(page, duringOutage));

      await page.context().setOffline(false);

      // Reconnect fires catch-up, which resumes from the last id the thread holds —
      // no reload, no re-open.
      await expectRealtimeVisible(messageWithText(page, duringOutage));

      // And the recovered thread is live again, in both directions.
      const afterReconnect = `${label} after-reconnect`;
      await widgetComposer(conv.page).fill(afterReconnect);
      await widgetSend(conv.page).click();
      await expectRealtimeVisible(messageWithText(page, afterReconnect));

      const agentReply = `${label} agent-reply`;
      await sendReply(page, agentReply);
      await expectRealtimeVisible(widgetBubble(conv.page, agentReply));
    } finally {
      await page.context().setOffline(false);
      await conv.context.close();
    }
  });
});
