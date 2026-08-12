import { test, expect } from "@playwright/test";
import { pluralCategories, pluralCategory } from "../../../web/src/i18n";
import { PLURAL_CASES } from "../../../web/src/i18n/plural-cases.generated";
import { I18n } from "../../../web/src/i18n";

// The same repo table Go, Kotlin and Swift answer to
// (backend/internal/i18n/plural-cases.json), so the web drop-in cannot quietly
// disagree with the rest about what Latvian does with zero.
test.describe("plural categories", () => {
  test("every case in the shared table", () => {
    expect(PLURAL_CASES.length).toBeGreaterThan(0);
    for (const [locale, count, category] of PLURAL_CASES) {
      expect(pluralCategory(locale, count), `${locale}/${count}`).toBe(category);
    }
  });

  test("every case lands in a category the language offers", () => {
    for (const [locale, , category] of PLURAL_CASES) {
      expect(pluralCategories(locale), `${locale}`).toContain(category);
    }
  });

  test("a language with no table entry still pluralizes", () => {
    expect(pluralCategory("qq", 1)).toBe("one");
    expect(pluralCategory("qq", 7)).toBe("other");
  });

  test("a count renders in the category its language uses", () => {
    const en = new I18n({ locale: "en" });
    expect(en.tPlural("widget.home.agentsOnline", 1)).toBe("1 agent online");
    expect(en.tPlural("widget.home.agentsOnline", 3)).toBe("3 agents online");

    // Latvian's zero form covers 0 and the teens, which English has no key for.
    const lv = new I18n({ locale: "lv" });
    const zero = lv.tPlural("widget.home.agentsOnline", 0);
    expect(zero).not.toContain("agent");
    expect(lv.tPlural("widget.home.agentsOnline", 11)).toBe(zero.replace("0", "11"));

    // A category the language has but nobody wrote reads as that language's other
    // form — never English, never a raw key.
    const et = new I18n({ locale: "et" });
    expect(et.tPlural("widget.home.agentsOnline", 5)).toBe(
      et.t("widget.home.agentsOnline.other" as never, { count: 5 })
    );
  });

  test("an unknown key is blank unless debug asks for it", () => {
    const missed: string[] = [];
    const quiet = new I18n({ locale: "en", onMissingKey: (k) => missed.push(k) });
    expect(quiet.t("widget.nope" as never)).toBe("");
    expect(missed).toEqual(["widget.nope"]);

    const loud = new I18n({ locale: "en", debug: true });
    expect(loud.t("widget.nope" as never)).toBe("widget.nope");
  });
});
