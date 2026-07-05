import { test, expect } from "../../fixtures";
import {
  gotoInbox,
  filterTab,
  rowByText,
  allRows,
  openConversationByText,
  contactViewAll,
  contactHistoryRows,
  contactFilterChip,
} from "../../support/inbox";

// The Details-panel contact history. The dev seed gives "Mari Tamm" (u_mari)
// three threads — one open (claimed by the seeded admin) plus two earlier,
// closed ones — so the "Earlier from …" list and the "View all" contact filter
// have real data to render.
test.describe("contact history", () => {
  test("shows a contact's earlier threads and filters the list to them", async ({ page }) => {
    await gotoInbox(page);

    // Mari's open thread is assigned to the seeded admin → in the You scope.
    await filterTab(page, "you").click();
    await openConversationByText(page, "Mari Tamm");

    // Details panel lists her earlier threads (the two closed ones).
    await expect(page.getByText(/Earlier from Mari/)).toBeVisible();
    await expect(contactHistoryRows(page)).toHaveCount(2);

    // "View all" scopes the whole list to Mari and shows the dismissible chip.
    await contactViewAll(page).click();
    await expect(contactFilterChip(page)).toBeVisible();
    await expect(contactFilterChip(page)).toContainText("Mari Tamm");
    // All three of Mari's threads are now listed.
    await expect(allRows(page)).toHaveCount(3);

    // Clearing the chip restores the normal queue.
    await contactFilterChip(page).getByRole("button", { name: "Clear contact filter" }).click();
    await expect(contactFilterChip(page)).toHaveCount(0);
    await expect(rowByText(page, "Mari Tamm")).toBeVisible();
  });
});
