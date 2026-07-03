import { test, expect } from "../../fixtures";
import { uid } from "../../support/env";
import {
  seedDemoSession,
  openDemo,
  openWidgetPanel,
  widgetNewConversation,
  widgetComposer,
  widgetSend,
  widgetBack,
  widgetBubble,
} from "../../support/widget";

test.describe("widget thread", () => {
  test("start a new conversation, send a message, and go back home", async ({ page, context }) => {
    await seedDemoSession(context, { mode: "user", userId: uid("thread") });
    await openDemo(page);
    await openWidgetPanel(page);

    await widgetNewConversation(page).click();
    await expect(page.getByText("Type your message to start")).toBeVisible();

    const msg = `hi there ${uid("m")}`;
    await widgetComposer(page).fill(msg);
    await widgetSend(page).click();
    await expect(widgetBubble(page, msg)).toBeVisible();

    // Back returns to the Home screen (real-user mode shows the back button).
    await widgetBack(page).click();
    await expect(page.getByText("Send us a message")).toBeVisible();
  });
});
