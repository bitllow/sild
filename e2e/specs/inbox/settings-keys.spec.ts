import { test, expect } from "../../fixtures";
import { gotoInbox, gotoSettings } from "../../support/inbox";

test.describe.configure({ mode: "serial" });

test.describe("settings · API keys", () => {
  test("the seeded key is listed", async ({ page }) => {
    await gotoInbox(page);
    await gotoSettings(page, "API keys");
    await expect(page.getByText("dev", { exact: true })).toBeVisible();
  });

  test("creating a key reveals the secret once, then it can be revoked", async ({ page }) => {
    await gotoInbox(page);
    await gotoSettings(page, "API keys");

    // Wait for the (async-loaded) key list to render before counting.
    await expect(page.getByText("dev", { exact: true })).toBeVisible();
    const revokeButtons = page.getByRole("button", { name: "Revoke" });
    const before = await revokeButtons.count();

    await page.getByRole("button", { name: "New key" }).click();

    // The full secret is shown exactly once in the dialog.
    await expect(page.getByText("API key created")).toBeVisible();
    await expect(page.getByText(/sild_/)).toBeVisible();
    await page.getByRole("button", { name: "Done" }).click();

    // A new revocable key row now exists.
    await expect(revokeButtons).toHaveCount(before + 1);

    // Revoke it again brings the count back down.
    await revokeButtons.last().click();
    await expect(revokeButtons).toHaveCount(before);
  });
});
