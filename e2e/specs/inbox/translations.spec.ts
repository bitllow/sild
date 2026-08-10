import { test, expect, type Page } from "@playwright/test";
import { uid } from "../../support/env";
import {
  gotoTranslations,
  setTranslation,
  translationRow,
  translationRows,
} from "../../support/inbox";

// One tenant, one platform project: overrides and releases are shared state.
test.describe.configure({ mode: "serial" });

// Sild's shipped Estonian, the default this file edits away from and back to.
const LOCALE = "et";
const KEY = "widget.home.cta";
const SHIPPED = "Saada meile sõnum";
const SOURCE = "Send us a message";

test.describe("translations · editor", () => {
  test("lists keys with their English source", async ({ page }) => {
    await gotoTranslations(page, LOCALE);

    await expect(translationRows(page).first()).toBeVisible();
    const row = translationRow(page, KEY);
    await expect(row.getByTestId("translations-source")).toHaveText(SOURCE);
    await expect(row.getByTestId("translations-value")).toHaveValue(SHIPPED);
    await expect(row.getByTestId("translations-state")).toHaveText("default");
  });

  test("an edited value persists across a reload and reads custom", async ({ page }) => {
    await gotoTranslations(page, LOCALE);

    const value = `Kirjuta meile ${uid("et")}`;
    await setTranslation(page, KEY, value);
    await expect(translationRow(page, KEY).getByTestId("translations-state")).toHaveText("custom");

    await page.reload();
    await gotoTranslations(page, LOCALE);
    const row = translationRow(page, KEY);
    await expect(row.getByTestId("translations-value")).toHaveValue(value);
    await expect(row.getByTestId("translations-state")).toHaveText("custom");
  });

  test("Reset restores the shipped default", async ({ page }) => {
    await gotoTranslations(page, LOCALE);

    const row = translationRow(page, KEY);
    await expect(row.getByTestId("translations-value")).not.toHaveValue(SHIPPED);
    await row.getByTestId("translations-reset").click();

    await expect(row.getByTestId("translations-value")).toHaveValue(SHIPPED);
    await expect(row.getByTestId("translations-state")).toHaveText("default");
    await expect(row.getByTestId("translations-reset")).toHaveCount(0);
  });

  test("the state filter and the search narrow the list", async ({ page }) => {
    await gotoTranslations(page, LOCALE);
    const all = await translationRows(page).count();
    expect(all).toBeGreaterThan(1);

    await setTranslation(page, KEY, `Ainus muudatus ${uid("et")}`);
    try {
      // Only the one key this test customised comes back.
      await page.getByTestId("translations-filter").selectOption("custom");
      await expect(translationRows(page)).toHaveCount(1);
      await expect(translationRow(page, KEY)).toBeVisible();

      await page.getByTestId("translations-filter").selectOption("");
      await expect(translationRows(page)).toHaveCount(all);

      // Search matches on the key as well as the text. The count is whatever the
      // catalog holds — pinning it would fail on the next key Sild adds.
      await page.getByTestId("translations-search").fill("widget.composer");
      await expect(translationRow(page, "widget.composer.send")).toBeVisible();
      await expect(translationRow(page, KEY)).toHaveCount(0);
      const matched = await translationRows(page).evaluateAll((rows) =>
        rows.map((r) => r.getAttribute("data-key"))
      );
      expect(matched.length).toBeLessThan(all);
      for (const key of matched) {
        expect(key).toContain("widget.composer");
      }

      await page.getByTestId("translations-search").fill("");
      await expect(translationRows(page)).toHaveCount(all);
    } finally {
      await translationRow(page, KEY).getByTestId("translations-reset").click();
      await expect(translationRow(page, KEY).getByTestId("translations-state")).toHaveText("default");
    }
  });

  test("publishing cuts a release and advances the version", async ({ page }) => {
    await gotoTranslations(page, LOCALE);

    const next = (await currentVersion(page)) + 1;
    await page.getByTestId("translations-publish").click();

    await expect(page.getByTestId("translations-version")).toHaveText(`Published v${next}`);
    await expect(page.getByTestId("translations-releases").locator(`[data-version="${next}"]`)).toBeVisible();
  });
});

// 0 while nothing is published, so the first publish is v1.
async function currentVersion(page: Page): Promise<number> {
  const label = await page.getByTestId("translations-version").textContent();
  const match = /v(\d+)/.exec(label ?? "");
  return match ? Number(match[1]) : 0;
}
