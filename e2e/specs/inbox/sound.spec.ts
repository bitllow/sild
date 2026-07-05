import { test, expect } from "../../fixtures";
import { gotoInbox, soundToggle } from "../../support/inbox";

// The new-conversation sound toggle in the inbox list header. Muted state flips
// aria-pressed and the accessible label; the two-tone chime itself (Web Audio)
// isn't asserted here — only the control's state, which is what the UI exposes.
test.describe("inbox sound toggle", () => {
  test("mutes and unmutes new-conversation sounds", async ({ page }) => {
    await gotoInbox(page);
    const btn = soundToggle(page);
    await expect(btn).toBeVisible();

    // Starts on (unmuted).
    await expect(btn).toHaveAttribute("aria-pressed", "true");
    await expect(btn).toHaveAttribute("aria-label", /Mute/);

    // Click → muted.
    await btn.click();
    await expect(btn).toHaveAttribute("aria-pressed", "false");
    await expect(btn).toHaveAttribute("aria-label", /Unmute/);

    // Click → unmuted again.
    await btn.click();
    await expect(btn).toHaveAttribute("aria-pressed", "true");
  });

  test("the mute preference survives a reload", async ({ page }) => {
    await gotoInbox(page);
    await soundToggle(page).click();
    await expect(soundToggle(page)).toHaveAttribute("aria-pressed", "false");

    await page.reload();
    await page.getByRole("button", { name: "Inbox" }).waitFor({ state: "visible" });
    await expect(soundToggle(page)).toHaveAttribute("aria-pressed", "false");
  });
});
