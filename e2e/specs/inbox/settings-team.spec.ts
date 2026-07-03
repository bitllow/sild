import { test, expect } from "../../fixtures";
import { gotoInbox, gotoSettings } from "../../support/inbox";
import { ADMIN } from "../../support/env";

test.describe.configure({ mode: "serial" });

test.describe("settings · team", () => {
  test("the seeded owner is listed and their role can be changed", async ({ page }) => {
    await gotoInbox(page);
    await gotoSettings(page, "Team");

    await expect(page.getByText(ADMIN.email)).toBeVisible();

    const roleSelect = page.getByRole("combobox").first();
    await expect(roleSelect).toHaveValue("owner");

    // Change → persists across reload.
    await roleSelect.selectOption("admin");
    await expect(roleSelect).toHaveValue("admin");
    await page.reload();
    await gotoSettings(page, "Team");
    await expect(page.getByRole("combobox").first()).toHaveValue("admin");

    // Restore owner so other specs keep full privileges.
    await page.getByRole("combobox").first().selectOption("owner");
    await expect(page.getByRole("combobox").first()).toHaveValue("owner");
  });
});
