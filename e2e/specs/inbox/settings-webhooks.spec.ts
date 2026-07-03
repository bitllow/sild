import { test, expect } from "../../fixtures";
import { gotoInbox, gotoSettings, toggleSwitch } from "../../support/inbox";
import { createWebhook, deleteAllWebhooks } from "../../support/backdoor";

test.describe.configure({ mode: "serial" });

test.describe("settings · webhooks", () => {
  test("a webhook renders with its events and can be toggled and deleted", async ({ page, request }) => {
    // The Webhooks tab has no "add endpoint" control, so seed via the backdoor;
    // start clean so page-level locators are unambiguous.
    await deleteAllWebhooks(request);
    const url = `https://example.com/hook/${Date.now().toString(36)}`;
    await createWebhook(request, url, ["message.created", "conversation.closed"]);

    await gotoInbox(page);
    await gotoSettings(page, "Webhooks");

    // The endpoint URL and its event tags render.
    await expect(page.getByText(url)).toBeVisible();
    await expect(page.getByText("message.created")).toBeVisible();

    // Toggle active state (single webhook on the page).
    await toggleSwitch(page, 0);

    // Delete removes it (UI action).
    await page.getByRole("button", { name: "Delete webhook" }).first().click();
    await expect(page.getByText(url)).toHaveCount(0);
  });
});
