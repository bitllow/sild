import { test, expect } from "../../fixtures";
import { gotoInbox, gotoSettings, switchAt, toggleSwitch } from "../../support/inbox";
import { INBOX_URL } from "../../support/env";

test.describe("settings · channels", () => {
  test("email channel shows forwarding address and verification state", async ({ page }) => {
    await gotoInbox(page);
    await gotoSettings(page, "Channels");

    await expect(page.getByText("Email", { exact: true })).toBeVisible();
    await expect(page.getByText(/Forwarding address/i)).toBeVisible();
    // Before any email arrives the channel is unverified.
    await expect(page.getByText(/Awaiting first email|Verified/)).toBeVisible();
  });

  test("copy forwarding address flips the button label", async ({ page, context }) => {
    await context.grantPermissions(["clipboard-read", "clipboard-write"], { origin: INBOX_URL });
    await gotoInbox(page);
    await gotoSettings(page, "Channels");

    const copy = page.getByRole("button", { name: /Copy/ });
    await copy.click();
    await expect(page.getByText("Copied")).toBeVisible();
  });

  test("auto-reply toggle persists across reload", async ({ page }) => {
    await gotoInbox(page);
    await gotoSettings(page, "Channels");

    const after = await toggleSwitch(page, 0);

    await page.reload();
    await gotoSettings(page, "Channels");
    await expect(switchAt(page, 0).input).toBeChecked({ checked: after });

    // Leave it as we found it.
    await switchAt(page, 0).label.click();
  });

  test("other channels are shown but not yet connectable", async ({ page }) => {
    await gotoInbox(page);
    await gotoSettings(page, "Channels");
    await expect(page.getByText("WhatsApp", { exact: true })).toBeVisible();
    await expect(page.getByRole("button", { name: "Connect" }).first()).toBeDisabled();
  });
});
