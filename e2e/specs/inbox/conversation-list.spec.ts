import { test, expect } from "../../fixtures";
import { uid } from "../../support/env";
import {
  gotoInbox,
  filterTab,
  rowByText,
  openConversationByText,
  conversationIdByText,
  claimButton,
  headerStatusPill,
  openCount,
} from "../../support/inbox";
import { closeAssignmentForConversation } from "../../support/backdoor";

test.describe("inbox conversation list", () => {
  test("filters scope the queue to unassigned / you / closed", async ({ page, app, request }) => {
    const un = await app.conversation({ body: `unassigned ${uid("b")}` });
    const mine = await app.conversation({ body: `mine ${uid("b")}` });
    const done = await app.conversation({ body: `closed ${uid("b")}` });

    await gotoInbox(page);

    // Claim `mine` through the inbox UI.
    await openConversationByText(page, mine.body);
    await claimButton(page).click();
    await expect(headerStatusPill(page)).toHaveAttribute("data-status", "assigned");

    // Move `done` to the Closed queue. Claiming is via the UI; closing the
    // ASSIGNMENT has no inbox control, so use the documented backdoor.
    const doneId = await conversationIdByText(page, done.body);
    await closeAssignmentForConversation(request, doneId);

    // Unassigned: the queued request is here; the closed one is not.
    await filterTab(page, "unassigned").click();
    await expect(filterTab(page, "unassigned")).toHaveAttribute("aria-pressed", "true");
    await expect(rowByText(page, un.body)).toBeVisible();
    await expect(rowByText(page, done.body)).toHaveCount(0);

    // You: the one I claimed.
    await filterTab(page, "you").click();
    await expect(rowByText(page, mine.body)).toBeVisible();

    // Closed: the one whose assignment was closed.
    await filterTab(page, "closed").click();
    await expect(rowByText(page, done.body)).toBeVisible();
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
