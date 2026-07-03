import { test, expect } from "../../fixtures";
import { uid } from "../../support/env";
import {
  createWidgetPage,
  openWidgetPanel,
  widgetComposer,
  widgetSend,
  widgetBubble,
  widgetBack,
} from "../../support/widget";
import {
  gotoInbox,
  filterTab,
  allRows,
  claimButton,
  sendReply,
} from "../../support/inbox";
import { expectRealtimeVisible } from "../../support/realtime";

// The same live round-trip, but for an anonymous guest (single-thread) visitor.
test.describe("cross-surface realtime flow · guest", () => {
  test("guest message reaches the inbox and the agent reply reaches the guest", async ({ page, browser }) => {
    const guestName = uid("guest");
    const guestMsg = `how do I change pickup ${uid("m")}`;

    await gotoInbox(page);
    await filterTab(page, "unassigned").click();

    const { context, page: widget } = await createWidgetPage(browser, {
      mode: "guest",
      userId: guestName,
      metadata: { name: guestName },
    });
    try {
      await openWidgetPanel(widget);
      // Guest opens straight into a single thread — no back button.
      await expect(widgetBack(widget)).toHaveCount(0);

      await widgetComposer(widget).fill(guestMsg);
      await widgetSend(widget).click();
      await expect(widgetBubble(widget, guestMsg)).toBeVisible();

      const row = allRows(page).filter({ hasText: guestName });
      await expectRealtimeVisible(row);
      await row.click();
      await claimButton(page).click();

      const agentMsg = `here's how ${uid("m")}`;
      await sendReply(page, agentMsg);
      await expectRealtimeVisible(widgetBubble(widget, agentMsg));
    } finally {
      await context.close();
    }
  });
});
