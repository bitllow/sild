import { test, expect, type Page } from "@playwright/test";
import { uid } from "../../support/env";
import { gotoInbox, translationRow, translationRows } from "../../support/inbox";

// One tenant: the projects this file creates are shared state, and the last test
// removes what the first one made.
test.describe.configure({ mode: "serial" });

const KEY = "checkout.pay";
const SOURCE = "Pay now";
const LATVIAN = "Maksāt tagad";

// A project id is per-run, so a re-run against a surviving tenant does not collide.
const PROJECT = uid("shop").toLowerCase().replace(/[^a-z0-9-]/g, "");

async function gotoTranslations(page: Page): Promise<void> {
  await gotoInbox(page);
  await page.getByTestId("translations-nav").click();
  await expect(page.getByRole("heading", { name: "Translations" })).toBeVisible();
  await expect(translationRows(page).first()).toBeVisible();
}

// Select the tenant's own project and wait for its (separate) key list.
async function selectProject(page: Page): Promise<void> {
  await page.getByTestId("translations-project").selectOption(PROJECT);
  await expect(page.getByTestId("translations-new-key")).toBeVisible();
}

test.describe("translations · a tenant's own project", () => {
  test("a created project starts empty and declares its own keys", async ({ page }) => {
    await gotoTranslations(page);

    await page.getByTestId("translations-new-project").click();
    await page.getByTestId("translations-project-id").fill(PROJECT);
    await page.getByTestId("translations-project-name").fill("Shop");
    await page.getByTestId("translations-create-project").click();

    await expect(page.getByTestId("translations-new-key")).toBeVisible();
    await expect(translationRows(page)).toHaveCount(0);

    await page.getByTestId("translations-new-key").fill(KEY);
    await page.getByTestId("translations-new-source").fill(SOURCE);
    await page.getByTestId("translations-declare").click();

    const row = translationRow(page, KEY);
    await expect(row.getByTestId("translations-source")).toHaveText(SOURCE);
    // Nothing is translated yet, so the source is what the key renders as.
    await expect(row.getByTestId("translations-value")).toHaveValue(SOURCE);
  });

  test("a translation lands against the declared source and moves completion", async ({ page }) => {
    await gotoTranslations(page);
    await selectProject(page);

    // Latvian is not on for a new project; turning it on is what offers the market.
    // The switch's input is visually hidden, so the track is what a user clicks.
    await page
      .locator('[data-testid="translations-language"][data-locale="lv"] .sild-switch__track')
      .click();
    await page.getByTestId("translations-locale").selectOption("lv");
    await expect(translationRow(page, KEY)).toBeVisible();

    await expect(page.locator('[data-testid="translations-completion"][data-locale="lv"]')).toHaveText("0%");

    const input = translationRow(page, KEY).getByTestId("translations-value");
    await input.fill(LATVIAN);
    await input.press("Enter");
    await expect(translationRow(page, KEY).getByText("Saved")).toBeVisible();

    await page.reload();
    await gotoTranslations(page);
    await selectProject(page);
    await page.getByTestId("translations-locale").selectOption("lv");
    await expect(translationRow(page, KEY).getByTestId("translations-value")).toHaveValue(LATVIAN);
    await expect(page.locator('[data-testid="translations-completion"][data-locale="lv"]')).toHaveText("100%");
  });

  test("rewording the source flags the translation and it keeps rendering", async ({ page }) => {
    await gotoTranslations(page);
    await selectProject(page);
    await page.getByTestId("translations-locale").selectOption("lv");
    await expect(translationRow(page, KEY)).toBeVisible();

    // Re-declaring the same key with new English is how a source is reworded.
    await page.getByTestId("translations-new-key").fill(KEY);
    await page.getByTestId("translations-new-source").fill("Pay securely");
    await page.getByTestId("translations-declare").click();

    const row = translationRow(page, KEY);
    await expect(row.getByTestId("translations-state")).toHaveText("needs review");
    await expect(row.getByTestId("translations-value")).toHaveValue(LATVIAN);
  });

  test("removing a key and the project takes their strings with them", async ({ page }) => {
    await gotoTranslations(page);
    await selectProject(page);

    await translationRow(page, KEY).getByTestId("translations-undeclare").click();
    await expect(translationRow(page, KEY)).toHaveCount(0);

    await page
      .locator(`[data-project="${PROJECT}"]`)
      .getByTestId("translations-delete-project")
      .click();
    await expect(page.locator(`[data-project="${PROJECT}"]`)).toHaveCount(0);
    // Sild's own project is in every tenant, so the section still has strings.
    await expect(translationRow(page, "widget.home.cta")).toBeVisible();
  });
});
