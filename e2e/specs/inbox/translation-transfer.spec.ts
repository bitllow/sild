import { test, expect } from "@playwright/test";
import { uid } from "../../support/env";
import { gotoTranslations, selectTranslationLocale, translationRow } from "../../support/inbox";

// One tenant, one platform project: overrides are shared state.
test.describe.configure({ mode: "serial" });

const LOCALE = "et";
// Not widget.home.cta: the editor spec asserts the shipped default on that one, and
// a tenant is shared across these files.
const KEY = "widget.status.loading";
const SHIPPED = "Laadimine…";

test.describe("translations · import and export", () => {
  test("a file is previewed before it is applied, and the preview is what lands", async ({ page }) => {
    await gotoTranslations(page, LOCALE);
    const value = `Kirjuta meile ${uid("imp")}`;

    await page.getByTestId("translations-import-format").selectOption("json");
    await page.getByTestId("translations-import-file").setInputFiles({
      name: "et.json",
      mimeType: "application/json",
      buffer: Buffer.from(JSON.stringify({ [KEY]: value })),
    });

    // Nothing is written yet: the report is a dry run of the same call.
    const report = page.getByTestId("translations-import-report");
    await expect(report).toContainText("1 new");
    await expect(translationRow(page, KEY).getByTestId("translations-value")).toHaveValue(SHIPPED);

    await page.getByTestId("translations-import-apply").click();
    const row = translationRow(page, KEY);
    await expect(row.getByTestId("translations-value")).toHaveValue(value);
    await expect(row.getByTestId("translations-state")).toHaveText("custom");

    // And it is a draft, so it is waiting to be published like any other edit.
    await expect(page.getByTestId("translations-drafts").getByTestId("translations-draft-count")).not.toHaveText("0");

    // Put it back: the tenant is shared with every other spec in this project.
    await row.getByTestId("translations-reset").click();
    await expect(row.getByTestId("translations-value")).toHaveValue(SHIPPED);
  });

  // A preview says what a file would do to the language it was taken against, so
  // switching language has to take the approval with it.
  test("switching language drops a preview rather than applying it elsewhere", async ({ page }) => {
    await gotoTranslations(page, LOCALE);
    await page.getByTestId("translations-import-file").setInputFiles({
      name: "et.json",
      mimeType: "application/json",
      buffer: Buffer.from(JSON.stringify({ [KEY]: `Laadin ${uid("sw")}` })),
    });
    await expect(page.getByTestId("translations-import-report")).toContainText("1 new");
    await expect(page.getByTestId("translations-import-apply")).toContainText("Apply to et");

    await selectTranslationLocale(page, "lv");
    await expect(page.getByTestId("translations-import-report")).toHaveCount(0);
    await expect(page.getByTestId("translations-import-apply")).toHaveCount(0);
  });

  test("a key nobody declared is reported and skipped", async ({ page }) => {
    await gotoTranslations(page, LOCALE);

    await page.getByTestId("translations-import-file").setInputFiles({
      name: "et.json",
      mimeType: "application/json",
      buffer: Buffer.from(JSON.stringify({ "shop.checkout.pay": "Maksa" })),
    });

    const report = page.getByTestId("translations-import-report");
    await expect(report).toContainText("1 skipped");
    await expect(report.getByTestId("translations-import-row")).toContainText("unknown key");
    // Nothing to apply, so the button refuses rather than writing an empty change.
    await expect(page.getByTestId("translations-import-apply")).toBeDisabled();
  });

  test("an export downloads the language on screen", async ({ page }) => {
    await gotoTranslations(page, LOCALE);
    await page.getByTestId("translations-export-format").selectOption("android");

    const download = page.waitForEvent("download");
    await page.getByTestId("translations-export").click();
    const file = await download;
    expect(file.suggestedFilename()).toBe("strings.xml");
  });

  test("the From CI panel documents the calls a build makes", async ({ page }) => {
    await gotoTranslations(page, LOCALE);
    const panel = page.getByTestId("translations-from-ci");
    await expect(panel).toContainText("/export?locale=et");
    await expect(panel).toContainText("/import?locale=et");
    await expect(panel).toContainText("Authorization: Bearer");
  });

  test("a plural key is grouped by its base and labelled by category", async ({ page }) => {
    await gotoTranslations(page, LOCALE);
    await page.getByTestId("translations-search").fill("agentsOnline");

    const rows = page.getByTestId("translations-row");
    await expect(rows.first()).toBeVisible();
    // Estonian has one and other, and no zero form to fill in.
    await expect(rows).toHaveCount(2);
    await expect(rows.first().getByTestId("translations-plural-category")).toHaveText("one");
    await expect(rows.first().getByTestId("translations-placeholders")).toContainText("{count}");
  });
});
