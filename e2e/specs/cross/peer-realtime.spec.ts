import { test, expect } from "@playwright/test";
import { composer, composerSend, composerFileInput } from "../../support/inbox";
import { widgetComposer, widgetSend, widgetBubble } from "../../support/widget";
import { expectRealtimeVisible } from "../../support/realtime";
import { uid } from "../../support/env";
import { riderPostsAndAgentObserves, closePeerConversation } from "../../support/peer";

// The flagship peer-conversations flow: a rider↔driver chat (no support agent)
// observed in the inbox and stepped into, round-tripping over realtime in BOTH
// directions across two surfaces — the agent inbox and the end-user widget.
// Correlation is by unique message text (parallel tests share the peer surface).
test("peer chat: rider (widget) → agent observes → step-in reply → widget, both ways", async ({
  page,
  browser,
}) => {
  // Rider opens the driver chat straight from the host "Open chat" card, posts a
  // message, and the agent finds the conversation live on the peer surface (the
  // host backend creates the peer conversation — the dev server stands in).
  const { context, widget, row, riderMsg } = await riderPostsAndAgentObserves(browser, page, "where are you");
  try {
    await row.click();
    await expect(page.getByTestId("peer-message").filter({ hasText: riderMsg })).toBeVisible();

    // Agent steps in (implicit join) — reply reaches the widget, and the inbox row
    // gains the "You joined" marker.
    const agentMsg = `two minutes away ${uid("m")}`;
    // The peer composer reuses the shared ComposerBar (same selectors as support).
    await composer(page).fill(agentMsg);
    await composerSend(page).click();
    await expectRealtimeVisible(widgetBubble(widget, agentMsg));
    await expect(page.getByTestId("peer-row").filter({ hasText: agentMsg }).getByText("You joined")).toBeVisible();

    // Reverse leg: a rider follow-up reaches the open inbox peer view live.
    const followUp = `thank you ${uid("m")}`;
    await widgetComposer(widget).fill(followUp);
    await widgetSend(widget).click();
    await expectRealtimeVisible(page.getByTestId("peer-message").filter({ hasText: followUp }));

    // Attachments cross both ways in a peer chat, same as an assigned conversation.
    const riderFile = `${uid("ridecard")}.txt`;
    await widget.locator('input[type="file"]').setInputFiles({
      name: riderFile,
      mimeType: "text/plain",
      buffer: Buffer.from("rider attachment"),
    });
    await expect(widget.getByText(riderFile)).toBeVisible();
    await widgetSend(widget).click();
    await expectRealtimeVisible(page.getByText(riderFile)); // rendered in the peer view

    const agentFile = `${uid("dispatch")}.txt`;
    await composerFileInput(page).setInputFiles({
      name: agentFile,
      mimeType: "text/plain",
      buffer: Buffer.from("agent attachment"),
    });
    await expect(page.getByText(agentFile)).toBeVisible(); // pending chip in the peer composer
    await composerSend(page).click();
    await expectRealtimeVisible(widget.getByText(agentFile));
  } finally {
    await context.close();
  }
});

// Review fix: a peer conversation closed elsewhere must drop off an observing
// operator's list LIVE (the peer store handles conversation.closed on the peer
// channel), AND when it was the OPEN one the observer must land on another live
// thread rather than an empty pane (auto-select). This covers both: it opens the
// conversation (making it active), closes it out-of-band, and asserts the row
// disappears and a different conversation becomes active.
test("peer chat: closing the active conversation auto-selects another", async ({
  page,
  browser,
  request,
}) => {
  const { context, rider, row } = await riderPostsAndAgentObserves(browser, page, "wrapping up");
  try {
    // OPEN the rider's conversation (make it active) — the header then shows the
    // rider's (unique) name. There are other (dev-seeded) peers to fall back to.
    await row.click();
    await expect(page.getByTestId("peer-header")).toContainText(rider);
    expect(await page.getByTestId("peer-row").count()).toBeGreaterThan(1);

    // Close the active conversation out-of-band.
    await closePeerConversation(request, rider);

    // The row drops live AND a different conversation becomes active — the header
    // stays visible (never the "Select a peer conversation." empty state) and no
    // longer names the rider.
    await expect(row).toHaveCount(0);
    const header = page.getByTestId("peer-header");
    await expect(header).toBeVisible();
    await expect(header).not.toContainText(rider);
  } finally {
    await context.close();
  }
});
