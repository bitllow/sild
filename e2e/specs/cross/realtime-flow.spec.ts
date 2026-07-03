import { test, expect } from "../../fixtures";
import { uid } from "../../support/env";
import {
  createWidgetPage,
  openWidgetPanel,
  widgetNewConversation,
  widgetComposer,
  widgetSend,
  widgetBubble,
} from "../../support/widget";
import {
  gotoInbox,
  filterTab,
  allRows,
  claimButton,
  headerStatusPill,
  sendReply,
  messageWithText,
} from "../../support/inbox";
import { expectRealtimeVisible } from "../../support/realtime";

// THE primary goal: the inbox and widget working together, end-to-end, live.
// Two contexts (agent inbox on :3000, end-user widget on :8080), one backend.
test.describe("cross-surface realtime flow", () => {
  test("user message → inbox (WS) → agent reply → widget (SSE), both directions", async ({ page, browser }) => {
    const convName = uid("rider"); // unique member name → find it in the inbox
    const userMsg = `driver hasn't arrived ${uid("m")}`;

    // Agent has the Unassigned queue open BEFORE the request exists, so its
    // arrival must be delivered over the websocket (no reload).
    await gotoInbox(page);
    await filterTab(page, "unassigned").click();

    // End user opens the widget and sends the first message.
    const { context, page: widget } = await createWidgetPage(browser, {
      mode: "user",
      userId: convName,
      metadata: { name: convName },
    });
    try {
      await openWidgetPanel(widget);
      await widgetNewConversation(widget).click();
      await widgetComposer(widget).fill(userMsg);
      await widgetSend(widget).click();
      await expect(widgetBubble(widget, userMsg)).toBeVisible();

      // The new request appears in the agent's queue over WS.
      const row = allRows(page).filter({ hasText: convName });
      await expectRealtimeVisible(row);
      await row.click();

      // Agent claims and replies.
      await claimButton(page).click();
      await expect(headerStatusPill(page)).toHaveAttribute("data-status", "assigned");
      const agentMsg = `on it — checking now ${uid("m")}`;
      await sendReply(page, agentMsg);

      // The reply reaches the user's widget over SSE.
      await expectRealtimeVisible(widgetBubble(widget, agentMsg));

      // Reverse leg: the user's follow-up reaches the (open) inbox thread live.
      const followUp = `thank you ${uid("m")}`;
      await widgetComposer(widget).fill(followUp);
      await widgetSend(widget).click();
      await expectRealtimeVisible(messageWithText(page, followUp));
    } finally {
      await context.close();
    }
  });
});
