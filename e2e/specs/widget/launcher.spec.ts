import { test, expect } from "../../fixtures";
import { uid } from "../../support/env";
import { seedDemoSession, openDemo, openWidgetPanel, widgetLauncher, widgetPanel } from "../../support/widget";

test.describe("widget launcher", () => {
  test("launcher opens and closes the chat panel", async ({ page, context }) => {
    await seedDemoSession(context, { mode: "user", userId: uid("launch") });
    await openDemo(page);

    await expect(widgetLauncher(page)).toBeVisible();
    await expect(widgetPanel(page)).toHaveCount(0);

    await openWidgetPanel(page);
    await expect(widgetPanel(page)).toBeVisible();

    // Clicking the launcher again closes it.
    await widgetLauncher(page).click();
    await expect(widgetPanel(page)).toHaveCount(0);
  });
});
