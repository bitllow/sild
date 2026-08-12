import { test, expect, type Browser, type Page } from "@playwright/test";
import { INBOX_URL } from "../../support/env";

// A translator is an ordinary tenant member with a translation-only capability
// set, so what they land on and what they are offered is the whole of their
// session. Covered here rather than in Go because the question is which nav
// entries render and which view opens.
test.describe.configure({ mode: "serial" });

const PASSWORD = "translator-pw-9f2c";

// invite creates a translator scoped to one language and gives them a password,
// through the owner's session. The UI has no password field, and inventing one
// for a test would be the wrong place to put it.
async function invite(page: Page, email: string, locales: string[]): Promise<void> {
  const created = await page.request.post(`${INBOX_URL}/v1/team`, {
    data: {
      email,
      first_name: "Tea",
      last_name: "Tõlkija",
      role: "translator",
      scope: { projects: ["all"], locales },
    },
  });
  expect(created.ok(), `invite translator: ${created.status()} ${await created.text()}`).toBeTruthy();
  const { id } = (await created.json()) as { id: string };

  const pw = await page.request.post(`${INBOX_URL}/v1/team/${id}/password`, {
    data: { password: PASSWORD },
  });
  expect(pw.ok(), `set password: ${pw.status()} ${await pw.text()}`).toBeTruthy();
}

// signIn opens a session that carries no owner cookie — the translator's own.
async function signIn(browser: Browser, email: string): Promise<Page> {
  const context = await browser.newContext({ storageState: { cookies: [], origins: [] } });
  const page = await context.newPage();
  const res = await page.request.post(`${INBOX_URL}/v1/admin/auth/password`, {
    data: { email, password: PASSWORD },
  });
  expect(res.ok(), `translator login: ${res.status()} ${await res.text()}`).toBeTruthy();
  return page;
}

test.describe("translations · a translator's own session", () => {
  test("lands on Translations and is offered no queue", async ({ page, browser }) => {
    const email = `translator-${Date.now()}@example.test`;
    await invite(page, email, ["all"]);

    const translator = await signIn(browser, email);
    await translator.goto(INBOX_URL);

    // The landing route is capability-derived: with no conversation capability
    // there is no queue to land on.
    await expect(translator.getByRole("heading", { name: "Translations" })).toBeVisible();
    await expect(translator.getByTestId("translations-row").first()).toBeVisible();
    await expect(translator.getByRole("button", { name: "Inbox" })).toHaveCount(0);
    await expect(translator.getByTestId("translations-nav")).toBeVisible();

    // Nor any of the rest of the support tool.
    await expect(translator.getByRole("button", { name: "Peer conversations" })).toHaveCount(0);
    await expect(translator.getByRole("button", { name: "Settings" })).toHaveCount(0);
    await translator.close();
  });

  test("a language they were not granted is not on the screen", async ({ page, browser }) => {
    const email = `translator-lv-${Date.now()}@example.test`;
    await invite(page, email, ["lv"]);

    const translator = await signIn(browser, email);
    await translator.goto(INBOX_URL);
    await expect(translator.getByTestId("translations-row").first()).toBeVisible();

    const options = await translator.getByTestId("translations-locale").locator("option").allInnerTexts();
    expect(options.join(" ")).toContain("lv");
    expect(options.join(" ")).not.toContain("et");
    await translator.close();
  });

  // Without the publish grant there is no button; the draft count is what tells
  // them their work is queued rather than lost.
  test("sees their drafts waiting without being able to publish", async ({ page, browser }) => {
    const email = `translator-drafts-${Date.now()}@example.test`;
    await invite(page, email, ["all"]);

    const translator = await signIn(browser, email);
    await translator.goto(INBOX_URL);
    await expect(translator.getByTestId("translations-row").first()).toBeVisible();

    const row = translator.getByTestId("translations-row").filter({ hasText: "widget.home.cta" }).first();
    const input = row.getByTestId("translations-value");
    await input.fill(`Kirjuta meile ${Date.now()}`);
    await input.press("Enter");
    await expect(row.getByText("Saved")).toBeVisible();

    await expect(translator.getByTestId("translations-publish")).toHaveCount(0);
    const drafts = translator.getByTestId("translations-drafts");
    await expect(drafts).toBeVisible();
    await expect(drafts.getByTestId("translations-draft-count")).not.toHaveText("0");
    await translator.close();
  });
});
