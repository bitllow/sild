import { test, expect } from "../../fixtures";
import { uid } from "../../support/env";
import { gotoInbox, gotoSettings } from "../../support/inbox";
import { createWidgetPage, openWidgetPanel } from "../../support/widget";

// Appearance edits the tenant's single active brand; keep serial.
test.describe.configure({ mode: "serial" });

// Proves preview === production: a brand change saved in the inbox is what real
// visitors see in the live widget.
test.describe("appearance parity", () => {
  test("a heading saved in the inbox appears in the live widget", async ({ page, browser }) => {
    const heading = `Parity ${uid("h")}`;

    await gotoInbox(page);
    await gotoSettings(page, "Appearance");
    await page.getByPlaceholder("Hi there.").fill(heading);
    await page.getByRole("button", { name: "Save changes" }).click();
    await expect(page.getByText("Unsaved changes")).toHaveCount(0);

    // Load the real widget on the demo host and confirm the saved heading shows.
    const { context, page: widget } = await createWidgetPage(browser, {
      mode: "user",
      userId: uid("parity"),
    });
    try {
      await openWidgetPanel(widget);
      await expect(widget.getByRole("heading", { name: heading })).toBeVisible();
    } finally {
      await context.close();
    }
  });
});
