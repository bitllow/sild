import { test, expect, type Page } from "@playwright/test";
import { gotoInbox, gotoSettings, switchAt } from "../../support/inbox";

// Appearance edits the tenant's single active brand, so keep this file serial.
test.describe.configure({ mode: "serial" });

const preview = (page: Page) => page.getByTestId("appearance-preview");

async function openAppearance(page: Page) {
  await gotoInbox(page);
  await gotoSettings(page, "Appearance");
  await expect(preview(page).getByRole("dialog", { name: "Support chat" })).toBeVisible();
}

test.describe("settings · appearance", () => {
  test("welcome-screen content reflects live in the preview and persists", async ({ page }) => {
    await openAppearance(page);

    const heading = `Welcome ${Date.now().toString(36)}`;
    const sub = `We reply fast ${Date.now().toString(36)}`;
    const topicA = `Track order ${Date.now().toString(36)}`;
    const topicB = `Billing question`;

    await page.getByPlaceholder("Hi there.").fill(heading);
    await page.getByPlaceholder("How can we help?").fill(sub);
    await page.getByPlaceholder(/Track my order/).fill(`${topicA}\n${topicB}`);

    // Preview (the same component the widget ships) reflects each change.
    await expect(preview(page).getByRole("heading", { name: heading })).toBeVisible();
    await expect(preview(page).getByText(sub)).toBeVisible();
    await expect(preview(page).getByRole("button", { name: topicA })).toBeVisible();
    await expect(preview(page).getByRole("button", { name: topicB })).toBeVisible();

    // Toggle team + powered-by and capture the resulting states.
    const teamOn = await toggleAndRead(page, 0);
    const poweredOn = await toggleAndRead(page, 1);
    await assertTeamAndPowered(page, teamOn, poweredOn);

    await save(page);

    // Persists across reload.
    await page.reload();
    await gotoSettings(page, "Appearance");
    await expect(page.getByPlaceholder("Hi there.")).toHaveValue(heading);
    await expect(page.getByPlaceholder("How can we help?")).toHaveValue(sub);
    await expect(page.getByPlaceholder(/Track my order/)).toHaveValue(`${topicA}\n${topicB}`);
    await assertTeamAndPowered(page, teamOn, poweredOn);
  });

  test("brand, look & feel and launcher controls are all editable and saved", async ({ page }) => {
    await openAppearance(page);

    // Brand color — pick a specific swatch; the active one shows a check.
    const color = "#1F8A5B";
    await page.locator(`[data-testid="brand-swatch"][data-color="${color}"]`).click();
    await expect(page.locator(`[data-color="${color}"] svg`)).toBeVisible();

    // Look & feel.
    await page.getByRole("button", { name: "Dark", exact: true }).click();
    await page.getByRole("combobox").selectOption("inter");
    await page.getByRole("button", { name: "Rounded", exact: true }).click();

    // Launcher — every preset icon, plus position and size.
    for (const icon of ["Speech bubble", "Message", "Question", "Sparkle"]) {
      await page.getByRole("button", { name: icon, exact: true }).click();
    }
    await page.getByRole("button", { name: "Left", exact: true }).click();
    await page.getByRole("button", { name: "Large", exact: true }).click();

    // All of the above are unsaved edits.
    await expect(page.getByText("Unsaved changes")).toBeVisible();
    await save(page);

    // The chosen brand color persists across reload (observable proof the set saved).
    await page.reload();
    await gotoSettings(page, "Appearance");
    await expect(page.locator(`[data-color="${color}"] svg`)).toBeVisible();
  });

  test("home / conversation preview toggle switches the rendered screen", async ({ page }) => {
    await openAppearance(page);
    await page.getByRole("button", { name: "Conversation", exact: true }).click();
    await expect(preview(page).getByPlaceholder(/Write a message/)).toBeVisible();
    await page.getByRole("button", { name: "Home", exact: true }).click();
    await expect(preview(page).getByRole("heading").first()).toBeVisible();
  });

  test("Reset discards unsaved edits", async ({ page }) => {
    await openAppearance(page);
    const before = await page.getByPlaceholder("Hi there.").inputValue();
    await page.getByPlaceholder("Hi there.").fill(`scratch ${Date.now().toString(36)}`);
    await expect(page.getByText("Unsaved changes")).toBeVisible();
    await page.getByRole("button", { name: "Reset" }).click();
    await expect(page.getByText("Unsaved changes")).toHaveCount(0);
    await expect(page.getByPlaceholder("Hi there.")).toHaveValue(before);
  });

  test("brand profile can be renamed", async ({ page }) => {
    await openAppearance(page);
    const name = `Brand ${Date.now().toString(36)}`;
    await page.getByRole("button", { name: "Rename profile" }).click();
    const nameInput = page.getByPlaceholder("Profile name");
    await nameInput.fill(name);
    await nameInput.press("Enter");
    await save(page);
    await page.reload();
    await gotoSettings(page, "Appearance");
    await expect(page.getByText(name)).toBeVisible();
  });
});

// ── helpers ──────────────────────────────────────────────────────────────────
async function save(page: Page) {
  await page.getByRole("button", { name: "Save changes" }).click();
  await expect(page.getByText("Unsaved changes")).toHaveCount(0);
}

async function toggleAndRead(page: Page, n: number): Promise<boolean> {
  const { label, input } = switchAt(page, n);
  const before = await input.isChecked();
  await label.click();
  await expect(input).toBeChecked({ checked: !before });
  return !before;
}

async function assertTeamAndPowered(page: Page, teamOn: boolean, poweredOn: boolean) {
  const team = preview(page).getByText(/agents online/);
  const powered = preview(page).getByText("Powered by Sild");
  await (teamOn ? expect(team).toBeVisible() : expect(team).toHaveCount(0));
  await (poweredOn ? expect(powered).toBeVisible() : expect(powered).toHaveCount(0));
}
