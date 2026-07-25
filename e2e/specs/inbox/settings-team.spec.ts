import { test, expect } from "../../fixtures";
import { gotoInbox, gotoSettings, switchAt } from "../../support/inbox";
import { ADMIN } from "../../support/env";

test.describe.configure({ mode: "serial" });

test.describe("settings · team", () => {
  test("the seeded owner is listed and cannot demote themselves as the only owner", async ({ page }) => {
    await gotoInbox(page);
    await gotoSettings(page, "Team");

    await expect(page.getByText(ADMIN.email)).toBeVisible();

    const roleSelect = page.getByRole("combobox").first();
    await expect(roleSelect).toHaveValue("owner");

    // A tenant must keep an owner: they are the only principal who can grant peer
    // access or appoint another owner, so the server rejects demoting the last one.
    // The store applies role changes optimistically and rolls back on failure, so the
    // select flips and returns to "owner" — and the change must not survive a reload.
    await roleSelect.selectOption("admin");
    await expect(roleSelect).toHaveValue("owner");

    await page.reload();
    await gotoSettings(page, "Team");
    await expect(page.getByRole("combobox").first()).toHaveValue("owner");
  });

  test("peer access is the owner's grant to give", async ({ page }) => {
    await gotoInbox(page);
    await gotoSettings(page, "Team");

    // The seeded owner has peer access (dev seed), and as the owner they may toggle
    // it. Revoking hides the peer nav live; restore it so later specs keep the surface.
    const { label, input } = switchAt(page, 0);
    await expect(input).toBeChecked();
    await expect(input).toBeEnabled();

    await label.click();
    await expect(input).not.toBeChecked();
    await expect(page.getByRole("button", { name: "Peer conversations" })).toHaveCount(0);

    await label.click();
    await expect(input).toBeChecked();
    await expect(page.getByRole("button", { name: "Peer conversations" })).toHaveCount(1);
  });
});
