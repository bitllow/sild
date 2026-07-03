import { test, expect } from "../../fixtures";
import { uid } from "../../support/env";
import {
  gotoInbox,
  filterTab,
  searchInput,
  openConversationByText,
  headerStatusPill,
  claimButton,
  closeButton,
  sendReply,
  messageWithText,
  closedBanner,
  conversationHeader,
  allRows,
  detailsToggle,
} from "../../support/inbox";

test.describe("inbox conversation view", () => {
  test("claim, reply, internal note, and close a conversation", async ({ page, app }) => {
    const c = await app.conversation({ body: `my card keeps failing ${uid("b")}` });
    await gotoInbox(page);
    await filterTab(page, "unassigned").click();
    await openConversationByText(page, c.body);

    // Queued → Claim available.
    await expect(headerStatusPill(page)).toHaveAttribute("data-status", "queued");
    await expect(claimButton(page)).toBeVisible();
    await claimButton(page).click();
    await expect(headerStatusPill(page)).toHaveAttribute("data-status", "assigned");
    await expect(claimButton(page)).toHaveCount(0);

    // Public reply renders as a normal (non-internal) message.
    const reply = `on it ${uid("m")}`;
    await sendReply(page, reply);
    await expect(messageWithText(page, reply)).toBeVisible();
    await expect(messageWithText(page, reply)).toHaveAttribute("data-internal", "false");

    // Internal note renders with the internal marker.
    const note = `escalate ${uid("m")}`;
    await sendReply(page, note, true);
    await expect(messageWithText(page, note)).toHaveAttribute("data-internal", "true");
    await expect(messageWithText(page, note).getByText("Internal note")).toBeVisible();

    // Close is terminal: composer replaced by the closed banner.
    await closeButton(page).click();
    await expect(closedBanner(page)).toBeVisible();
    await expect(closeButton(page)).toHaveCount(0);
  });

  test("email conversation shows the Email badge", async ({ page }) => {
    await gotoInbox(page);
    // The dev seed includes an email-channel conversation (reference order_5512).
    await searchInput(page).fill("order_5512");
    await allRows(page).filter({ hasText: "order_5512" }).first().click();
    await expect(conversationHeader(page).getByText("Email")).toBeVisible();
  });

  test("details panel can be toggled", async ({ page, app }) => {
    const c = await app.conversation({ body: `toggle details ${uid("b")}` });
    await gotoInbox(page);
    await filterTab(page, "unassigned").click();
    await openConversationByText(page, c.body);
    const toggle = detailsToggle(page);
    await toggle.click();
    await toggle.click();
    await expect(conversationHeader(page)).toBeVisible();
  });
});
