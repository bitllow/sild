import { test, expect } from "../../fixtures";
import { uid } from "../../support/env";
import {
  createWidgetPage,
  openWidgetPanel,
  widgetNewConversation,
  widgetComposer,
  widgetSend,
  widgetBubble,
  widgetClosedBanner,
} from "../../support/widget";
import {
  gotoInbox,
  filterTab,
  allRows,
  claimButton,
  closeButton,
  sendReply,
  replyTab,
  composerSend,
  composerFileInput,
} from "../../support/inbox";
import { expectRealtimeVisible, expectNeverAppears } from "../../support/realtime";

test.describe("cross-surface edge behaviors", () => {
  test("internal notes stay private; attachments cross both ways; close propagates live", async ({ page, browser }) => {
    const name = uid("edge");
    const userMsg = `refund please ${uid("m")}`;
    const userFile = `receipt-${uid("f")}.txt`;
    const agentFile = `reply-${uid("f")}.txt`;

    await gotoInbox(page);
    await filterTab(page, "unassigned").click();

    const { context, page: widget } = await createWidgetPage(browser, {
      mode: "user",
      userId: name,
      metadata: { name },
    });
    try {
      await openWidgetPanel(widget);
      await widgetNewConversation(widget).click();
      await widgetComposer(widget).fill(userMsg);
      await widgetSend(widget).click();

      const row = allRows(page).filter({ hasText: name });
      await expectRealtimeVisible(row);
      await row.click();
      await claimButton(page).click();

      // 1) An internal note must NOT reach the widget.
      const note = `internal only ${uid("m")}`;
      await sendReply(page, note, true);
      await expectNeverAppears(widgetBubble(widget, note));

      // 2) Attachment from the WIDGET → appears in the agent's transcript (WS).
      await widget.locator('input[type="file"]').setInputFiles({
        name: userFile,
        mimeType: "text/plain",
        buffer: Buffer.from("receipt contents"),
      });
      await expect(widget.getByText(userFile)).toBeVisible();
      await widgetSend(widget).click();
      await expectRealtimeVisible(page.getByText(userFile));

      // 3) Attachment from the AGENT → appears in the widget thread (SSE).
      // Switch back to Reply first (step 1 left the composer in internal mode).
      await replyTab(page).click();
      await composerFileInput(page).setInputFiles({
        name: agentFile,
        mimeType: "text/plain",
        buffer: Buffer.from("agent reply attachment"),
      });
      await expect(page.getByText(agentFile)).toBeVisible(); // pending chip in composer
      await composerSend(page).click();
      await expectRealtimeVisible(widget.getByText(agentFile));

      // 4) Closing the conversation propagates to the widget over SSE.
      await closeButton(page).click();
      await expectRealtimeVisible(widgetClosedBanner(widget));
      await expect(widgetComposer(widget)).toBeDisabled();
    } finally {
      await context.close();
    }
  });
});
