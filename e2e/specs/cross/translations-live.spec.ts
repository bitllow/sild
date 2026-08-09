import { test, expect } from "@playwright/test";
import { uid } from "../../support/env";
import {
  createWidgetPage,
  openWidgetPanel,
  widgetHomeCta,
  widgetHomeNew,
} from "../../support/widget";
import { gotoTranslations, setTranslation, translationRow } from "../../support/inbox";

// The point of the feature: what an agent publishes is what a visitor reads.
const LOCALE = "lv";
const KEY = "widget.home.cta";
// Sild's shipped Latvian for a key this test does NOT touch.
const SHIPPED_NEW_CONVERSATION = "Jauna saruna";

test.describe("cross-surface translations", () => {
  test("a published override reaches a widget running in that locale", async ({ page, browser }) => {
    const override = `Uzraksti mums ${uid("lv")}`;

    await gotoTranslations(page, LOCALE);
    await setTranslation(page, KEY, override);
    await expect(translationRow(page, KEY).getByTestId("translations-state")).toHaveText("custom");

    const before = await page.getByTestId("translations-version").textContent();
    await page.getByTestId("translations-publish").click();
    await expect(page.getByTestId("translations-version")).not.toHaveText(before ?? "");

    // A Latvian device: the widget negotiates lv and fetches the bundle once the
    // client holds a token, which only opening the panel does.
    const { context, page: widget } = await createWidgetPage(browser, {
      mode: "user",
      userId: uid("lv_user"),
      locale: LOCALE,
    });
    try {
      await openWidgetPanel(widget);
      await expect(widgetHomeCta(widget)).toHaveText(override);
      await expect(widgetHomeNew(widget)).toContainText(SHIPPED_NEW_CONVERSATION);
    } finally {
      await context.close();
    }
  });
});
