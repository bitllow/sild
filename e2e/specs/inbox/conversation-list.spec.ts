import { test, expect } from "../../fixtures";
import { uid } from "../../support/env";
import {
  gotoInbox,
  filterTab,
  rowByText,
  openConversationByText,
  claimButton,
  closeButton,
  headerStatusPill,
  closedBanner,
  openCount,
  showClosedToggle,
} from "../../support/inbox";

// Read a scope tab's inline count pill as a number (0 when the pill is absent).
async function tabCount(page: import("@playwright/test").Page, key: "you" | "unassigned"): Promise<number> {
  const pill = filterTab(page, key).locator("span");
  if ((await pill.count()) === 0) return 0;
  const text = (await pill.first().textContent())?.trim() ?? "0";
  return Number.parseInt(text, 10) || 0;
}

test.describe("inbox conversation list", () => {
  test("scope tabs (you / unassigned / all) + Show closed toggle", async ({ page, app }) => {
    const un = await app.conversation({ body: `unassigned ${uid("b")}` });
    const mine = await app.conversation({ body: `mine ${uid("b")}` });
    const done = await app.conversation({ body: `closed ${uid("b")}` });

    await gotoInbox(page);

    // Claim `mine` through the inbox UI.
    await openConversationByText(page, mine.body);
    await claimButton(page).click();
    await expect(headerStatusPill(page)).toHaveAttribute("data-status", "assigned");

    // Close the `done` CONVERSATION through the real inbox control.
    await openConversationByText(page, done.body);
    await closeButton(page).click();
    await expect(closedBanner(page)).toBeVisible();

    // A closed row's preview becomes "Conversation closed", so locate it by the
    // (stable) participant name rather than the original message body.
    await filterTab(page, "unassigned").click();
    await expect(filterTab(page, "unassigned")).toHaveAttribute("aria-pressed", "true");
    await expect(rowByText(page, un.body)).toBeVisible();
    await expect(rowByText(page, done.name)).toHaveCount(0);

    // You: the one I claimed.
    await filterTab(page, "you").click();
    await expect(rowByText(page, mine.body)).toBeVisible();

    // All (with closed hidden by default): the closed one is not shown…
    await filterTab(page, "all").click();
    await expect(rowByText(page, mine.body)).toBeVisible();
    await expect(rowByText(page, done.name)).toHaveCount(0);

    // …until "Show closed" is toggled on, which reveals it.
    await expect(showClosedToggle(page)).toBeVisible();
    await showClosedToggle(page).click();
    await expect(showClosedToggle(page)).toHaveAttribute("aria-pressed", "true");
    await expect(rowByText(page, done.name)).toBeVisible();

    // Toggling back off hides it again.
    await showClosedToggle(page).click();
    await expect(rowByText(page, done.name)).toHaveCount(0);
  });

  // Guards the bug where the tab counters over-counted: closed conversations
  // (hidden from the list) must NOT inflate You/Unassigned. Asserts the count
  // TRACKS reality — up by one when a queued request arrives, back down (and not
  // stuck) once it's claimed and then closed.
  test("scope counters track the list as conversations move and close", async ({ page, app }) => {
    await gotoInbox(page);
    await filterTab(page, "unassigned").click();
    const un0 = await tabCount(page, "unassigned");
    const you0 = await tabCount(page, "you");

    const c = await app.conversation({ body: `counted ${uid("b")}` });
    await expect(rowByText(page, c.body)).toBeVisible();
    await expect.poll(() => tabCount(page, "unassigned")).toBe(un0 + 1);

    // Claim → moves from Unassigned to You: unassigned back down, you up.
    await openConversationByText(page, c.body);
    await claimButton(page).click();
    await expect(headerStatusPill(page)).toHaveAttribute("data-status", "assigned");
    await expect.poll(() => tabCount(page, "you")).toBe(you0 + 1);
    await expect.poll(() => tabCount(page, "unassigned")).toBe(un0);

    // Close the conversation → leaves You entirely (not counted as closed-in-you).
    await closeButton(page).click();
    await expect(closedBanner(page)).toBeVisible();
    await expect.poll(() => tabCount(page, "you")).toBe(you0);
  });

  test("a new conversation from the widget appears in Unassigned", async ({ page, app }) => {
    const c = await app.conversation({ body: `pickup change ${uid("b")}` });
    await gotoInbox(page);
    await filterTab(page, "unassigned").click();
    await expect(rowByText(page, c.body)).toBeVisible();
  });

  test("the open-count badge reflects open conversations", async ({ page }) => {
    await gotoInbox(page);
    await expect(openCount(page)).toHaveText(/\d+ open/);
  });

  test("sort direction can be toggled", async ({ page }) => {
    await gotoInbox(page);
    const desc = page.getByRole("button", { name: "Sort descending" });
    const asc = page.getByRole("button", { name: "Sort ascending" });
    await expect(desc).toBeVisible();
    await desc.click();
    await expect(asc).toBeVisible();
  });
});
