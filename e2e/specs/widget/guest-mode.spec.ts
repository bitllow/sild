import { test, expect } from "../../fixtures";
import { uid } from "../../support/env";
import {
  seedDemoSession,
  openDemo,
  openWidgetPanel,
  widgetComposer,
  widgetSend,
  widgetBack,
  widgetBubble,
} from "../../support/widget";

test.describe("widget guest mode", () => {
  test("guest opens straight to a single thread and persists across reload", async ({ page, context }) => {
    const guestId = uid("guest");
    await seedDemoSession(context, { mode: "guest", userId: guestId });
    await openDemo(page);
    await openWidgetPanel(page);

    // Guest = single thread: no back-to-list button.
    await expect(widgetBack(page)).toHaveCount(0);

    const msg = `guest says ${uid("m")}`;
    await widgetComposer(page).fill(msg);
    await widgetSend(page).click();
    await expect(widgetBubble(page, msg)).toBeVisible();

    // The same guest conversation is restored after a reload.
    await page.reload();
    await openWidgetPanel(page);
    await expect(widgetBubble(page, msg)).toBeVisible();
    await expect(widgetBack(page)).toHaveCount(0);
  });
});
