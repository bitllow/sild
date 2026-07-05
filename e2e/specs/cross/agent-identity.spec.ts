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
import {
  gotoInbox,
  filterTab,
  allRows,
  rowByText,
  openConversationByText,
  claimButton,
  headerStatusPill,
  sendReply,
  navAttention,
} from "../../support/inbox";
import { expectRealtimeVisible } from "../../support/realtime";

// The seeded admin is "Eva Marleen", so their replies carry the first name "Eva".
test.describe("agent identity in the widget", () => {
  test("the agent's first name shows in the widget instead of 'Support'", async ({ page, browser }) => {
    const convName = uid("rider");
    const userMsg = `need help ${uid("m")}`;

    await gotoInbox(page);
    await filterTab(page, "unassigned").click();

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

      // Agent claims and replies.
      const row = allRows(page).filter({ hasText: convName });
      await expectRealtimeVisible(row);
      await row.click();
      await claimButton(page).click();
      await expect(headerStatusPill(page)).toHaveAttribute("data-status", "assigned");
      const agentMsg = `on it ${uid("m")}`;
      await sendReply(page, agentMsg);

      // The reply arrives, attributed to "Eva" (the agent's first name).
      await expectRealtimeVisible(widgetBubble(widget, agentMsg));
      await expect(widget.getByText("Eva", { exact: true }).first()).toBeVisible();
      // The generic fallback is gone from the thread header.
      await expect(widget.getByText("Sild support")).toHaveCount(0);

      // Back on the Home list, the conversation's Recent row is labeled with the
      // agent's name too (not the generic "Support").
      await widgetBack(widget).click();
      await expect(widget.getByText("Send us a message")).toBeVisible();
      const recentRow = widget.locator(".row").filter({ hasText: agentMsg });
      await expect(recentRow).toContainText("Eva");
    } finally {
      await context.close();
    }
  });

  test("unread messages in a background conversation light the nav attention badge", async ({ page, browser, app }) => {
    // Conversation A — the one the agent is looking at (so it stays read).
    const a = await app.conversation({ body: `foreground ${uid("m")}` });
    await gotoInbox(page);
    await filterTab(page, "all").click();
    await openConversationByText(page, a.body);

    // Conversation B — a second user the agent is NOT looking at.
    const bName = uid("rider");
    const { context, page: widgetB } = await createWidgetPage(browser, {
      mode: "user",
      userId: bName,
      metadata: { name: bName },
    });
    try {
      await openWidgetPanel(widgetB);
      await widgetNewConversation(widgetB).click();
      await widgetComposer(widgetB).fill(`hello ${uid("m")}`);
      await widgetSend(widgetB).click();

      // Wait until B is in the agent's queue (its realtime channel is now subscribed).
      await expectRealtimeVisible(rowByText(page, bName));

      // B sends a follow-up while the agent is still on A → B gains an unread,
      // which surfaces as the coral attention badge on the nav-rail inbox icon.
      await widgetComposer(widgetB).fill(`are you there ${uid("m")}`);
      await widgetSend(widgetB).click();

      await expectRealtimeVisible(navAttention(page));
    } finally {
      await context.close();
    }
  });
});
