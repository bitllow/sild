import { test, expect } from "../../fixtures";
import { uid } from "../../support/env";
import { seedDemoSession, openDemo, openWidgetPanel, widgetLauncherEl, widgetPanelEl } from "../../support/widget";

test.describe("widget launcher", () => {
  test("launcher opens and closes the chat panel", async ({ page, context }) => {
    await seedDemoSession(context, { mode: "user", userId: uid("launch") });
    await openDemo(page);

    await expect(widgetLauncherEl(page)).toBeVisible();
    await expect(widgetPanelEl(page)).toHaveCount(0);

    await openWidgetPanel(page);
    await expect(widgetPanelEl(page)).toBeVisible();

    // Clicking the launcher again closes it.
    await widgetLauncherEl(page).click();
    await expect(widgetPanelEl(page)).toHaveCount(0);
  });
});
