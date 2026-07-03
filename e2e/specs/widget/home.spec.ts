import { test, expect } from "../../fixtures";
import { uid } from "../../support/env";
import { seedDemoSession, openDemo, openWidgetPanel, widgetNewConversation } from "../../support/widget";

test.describe("widget home", () => {
  test("home shows the brand header and new-conversation card", async ({ page, context }) => {
    await seedDemoSession(context, { mode: "user", userId: uid("home") });
    await openDemo(page);
    await openWidgetPanel(page);

    await expect(page.getByText("Send us a message")).toBeVisible();
    await expect(widgetNewConversation(page)).toBeVisible();
    // The welcome heading (config.heading) renders.
    await expect(page.getByRole("heading").first()).toBeVisible();
  });
});
