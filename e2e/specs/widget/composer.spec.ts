import { test, expect } from "../../fixtures";
import { uid } from "../../support/env";
import {
  seedDemoSession,
  openDemo,
  openWidgetPanel,
  widgetNewConversation,
  widgetComposer,
  widgetSend,
  widgetBubble,
} from "../../support/widget";

async function openDraft(page: import("@playwright/test").Page, context: import("@playwright/test").BrowserContext, tag: string) {
  await seedDemoSession(context, { mode: "user", userId: uid(tag) });
  await openDemo(page);
  await openWidgetPanel(page);
  await widgetNewConversation(page).click();
}

test.describe("widget composer", () => {
  test("Enter sends the message", async ({ page, context }) => {
    await openDraft(page, context, "enter");
    const msg = `enter-send ${uid("m")}`;
    await widgetComposer(page).fill(msg);
    await widgetComposer(page).press("Enter");
    await expect(widgetBubble(page, msg)).toBeVisible();
  });

  test("Shift+Enter inserts a newline without sending", async ({ page, context }) => {
    await openDraft(page, context, "shift");
    const ta = widgetComposer(page);
    await ta.click();
    await ta.type("line one");
    await ta.press("Shift+Enter");
    await ta.type("line two");
    await expect(ta).toHaveValue("line one\nline two");
  });

  test("send is disabled with no text or attachments", async ({ page, context }) => {
    await openDraft(page, context, "empty");
    await expect(widgetSend(page)).toBeDisabled();
  });
});
