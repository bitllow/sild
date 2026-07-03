import { test, expect } from "../../fixtures";
import { uid } from "../../support/env";
import {
  createWidgetPage,
  openWidgetPanel,
  widgetNewConversation,
  widgetComposer,
  widgetSend,
} from "../../support/widget";
import {
  gotoInbox,
  filterTab,
  rowByText,
  openConversationByText,
  claimButton,
  headerStatusPill,
} from "../../support/inbox";
import { expectRealtimeVisible } from "../../support/realtime";

// Additional realtime coverage beyond the core round-trip (realtime-flow):
// unread propagation to a non-active row, and assignment updates fanning out to
// a second agent tab. (The widget/inbox do not emit typing or read receipts, so
// those aren't app-reachable and are intentionally not tested.)
test.describe("cross-surface realtime scenarios", () => {
  test("a message to a non-active conversation updates its row live", async ({ page, browser }) => {
    const nameA = uid("uA");
    const nameB = uid("uB");
    const bodyA = `alpha ${uid("m")}`;
    const bodyB = `beta ${uid("m")}`;

    await gotoInbox(page);
    await filterTab(page, "unassigned").click();

    const A = await createWidgetPage(browser, { mode: "user", userId: nameA, metadata: { name: nameA } });
    const B = await createWidgetPage(browser, { mode: "user", userId: nameB, metadata: { name: nameB } });
    try {
      await openWidgetPanel(A.page);
      await widgetNewConversation(A.page).click();
      await widgetComposer(A.page).fill(bodyA);
      await widgetSend(A.page).click();

      await openWidgetPanel(B.page);
      await widgetNewConversation(B.page).click();
      await widgetComposer(B.page).fill(bodyB);
      await widgetSend(B.page).click();

      await expectRealtimeVisible(rowByText(page, bodyA));
      await expectRealtimeVisible(rowByText(page, bodyB));

      // Make A the active conversation, then B (non-active) receives a message.
      await openConversationByText(page, bodyA);
      const follow = `beta again ${uid("m")}`;
      await widgetComposer(B.page).fill(follow);
      await widgetSend(B.page).click();

      // B's row preview updates live over the websocket, even though it isn't the
      // active conversation.
      await expectRealtimeVisible(rowByText(page, follow));
    } finally {
      await A.context.close();
      await B.context.close();
    }
  });

  test("claiming in one agent tab updates another agent tab live", async ({ page, context, browser }) => {
    const name = uid("sync");
    const body = `claim sync ${uid("m")}`;

    await gotoInbox(page);
    await filterTab(page, "unassigned").click();

    const W = await createWidgetPage(browser, { mode: "user", userId: name, metadata: { name } });
    try {
      await openWidgetPanel(W.page);
      await widgetNewConversation(W.page).click();
      await widgetComposer(W.page).fill(body);
      await widgetSend(W.page).click();
      await expectRealtimeVisible(rowByText(page, body));

      // Second agent tab (same authenticated session).
      const page2 = await context.newPage();
      await gotoInbox(page2);
      await filterTab(page2, "unassigned").click();
      await openConversationByText(page2, body);
      await expect(headerStatusPill(page2)).toHaveAttribute("data-status", "queued");

      // Claim in tab 1 → tab 2 reflects "assigned" live over the websocket.
      await openConversationByText(page, body);
      await claimButton(page).click();
      await expect(headerStatusPill(page2)).toHaveAttribute("data-status", "assigned", { timeout: 15_000 });
      await page2.close();
    } finally {
      await W.context.close();
    }
  });
});
