import { test, expect } from "../../fixtures";
import { uid } from "../../support/env";
import {
  createWidgetPage,
  openWidgetPanel,
  widgetNewConversation,
  widgetComposer,
  widgetSend,
  widgetBubble,
  widgetBack,
} from "../../support/widget";
import { gotoInbox, allRows, claimButton, sendReply } from "../../support/inbox";
import { expectRealtimeVisible } from "../../support/realtime";

// A single user with two conversations: agent replies route back to the right
// widget thread, with no cross-talk — proves routing holds in the combined system.
test.describe("cross-surface multi-conversation routing", () => {
  test("agent reply to conversation A lands only in thread A", async ({ page, browser }) => {
    const name = uid("multi");
    const msgA = `conversation A ${uid("m")}`;
    const msgB = `conversation B ${uid("m")}`;

    const { context, page: widget } = await createWidgetPage(browser, {
      mode: "user",
      userId: name,
      metadata: { name },
    });
    try {
      await openWidgetPanel(widget);

      // Create conversation A.
      await widgetNewConversation(widget).click();
      await widgetComposer(widget).fill(msgA);
      await widgetSend(widget).click();
      await expect(widgetBubble(widget, msgA)).toBeVisible();
      await widgetBack(widget).click();

      // Create conversation B.
      await widgetNewConversation(widget).click();
      await widgetComposer(widget).fill(msgB);
      await widgetSend(widget).click();
      await expect(widgetBubble(widget, msgB)).toBeVisible();
      await widgetBack(widget).click();

      // Agent replies to conversation A (identified by its message preview).
      await gotoInbox(page);
      const rowA = allRows(page).filter({ hasText: msgA });
      await expectRealtimeVisible(rowA);
      await rowA.click();
      await claimButton(page).click();
      const replyA = `answer for A ${uid("m")}`;
      await sendReply(page, replyA);

      // Widget: thread A has the reply...
      await widget.getByText(msgA).click();
      await expectRealtimeVisible(widgetBubble(widget, replyA));

      // ...thread B does not.
      await widgetBack(widget).click();
      await widget.getByText(msgB).click();
      await expect(widgetBubble(widget, msgB)).toBeVisible();
      await expect(widgetBubble(widget, replyA)).toHaveCount(0);
    } finally {
      await context.close();
    }
  });
});
