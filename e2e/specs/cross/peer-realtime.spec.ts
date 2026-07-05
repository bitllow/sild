import { test, expect } from "@playwright/test";
import { gotoInbox } from "../../support/inbox";
import {
  createWidgetPage,
  widgetComposer,
  widgetSend,
  widgetBubble,
} from "../../support/widget";
import { expectRealtimeVisible } from "../../support/realtime";
import { uid } from "../../support/env";

// The flagship peer-conversations flow: a rider↔driver chat (no support agent)
// observed in the inbox and stepped into, round-tripping over realtime in BOTH
// directions across two surfaces — the agent inbox and the end-user widget.
// Correlation is by unique message text (parallel tests share the peer surface).
test("peer chat: rider (widget) → agent observes → step-in reply → widget, both ways", async ({
  page,
  browser,
}) => {
  const rider = uid("rider");
  const { context, page: widget } = await createWidgetPage(browser, {
    mode: "user",
    userId: rider,
    metadata: { name: rider },
  });
  try {
    // Rider opens the driver chat straight from the host "Open chat" card (the
    // host backend creates the peer conversation — the dev server stands in).
    await widget.getByRole("button", { name: "Open chat" }).click();
    await widgetComposer(widget).waitFor({ state: "visible" });
    const riderMsg = `where are you ${uid("m")}`;
    await widgetComposer(widget).fill(riderMsg);
    await widgetSend(widget).click();
    await expect(widgetBubble(widget, riderMsg)).toBeVisible();

    // Agent opens the peer surface and finds the conversation by that message.
    await gotoInbox(page);
    await page.getByRole("button", { name: "Peer conversations" }).click();
    const row = page.getByTestId("peer-row").filter({ hasText: riderMsg });
    await expectRealtimeVisible(row);
    await row.click();
    await expect(page.getByTestId("peer-message").filter({ hasText: riderMsg })).toBeVisible();

    // Agent steps in (implicit join) — reply reaches the widget, and the inbox row
    // gains the "You joined" marker.
    const agentMsg = `two minutes away ${uid("m")}`;
    await page.getByTestId("peer-composer").fill(agentMsg);
    await page.getByTestId("peer-send").click();
    await expectRealtimeVisible(widgetBubble(widget, agentMsg));
    await expect(page.getByTestId("peer-row").filter({ hasText: agentMsg }).getByText("You joined")).toBeVisible();

    // Reverse leg: a rider follow-up reaches the open inbox peer view live.
    const followUp = `thank you ${uid("m")}`;
    await widgetComposer(widget).fill(followUp);
    await widgetSend(widget).click();
    await expectRealtimeVisible(page.getByTestId("peer-message").filter({ hasText: followUp }));
  } finally {
    await context.close();
  }
});
