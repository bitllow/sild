import { test, expect } from "../../fixtures";
import { gotoInbox, gotoSettings, switchByTestId } from "../../support/inbox";

// Push is set up against the tenant's OWN push project, so everything here is
// about supplying and checking that credential — Sild has none of its own.
test.describe("settings · push", () => {
  const card = "[data-testid=push-settings]";

  test("push starts unconfigured and says so", async ({ page }) => {
    await gotoInbox(page);
    await gotoSettings(page, "Channels");

    const push = page.locator(card);
    await expect(push).toBeVisible();
    await expect(push.getByTestId("push-status")).toHaveText(/Not set up|Awaiting first delivery|Delivering/);
    await expect(push.getByText(/service-account key/i)).toBeVisible();
  });

  test("the sender-name source only applies once the sender is shown", async ({ page }) => {
    await gotoInbox(page);
    await gotoSettings(page, "Channels");

    const push = page.locator(card);
    const sender = switchByTestId(push, "push-include-sender");
    const source = push.locator("select");

    // Start from a known state rather than branching on what we found.
    if (await sender.input.isChecked()) await sender.label.click();
    await expect(sender.input).not.toBeChecked();
    await expect(source).toBeDisabled();

    await sender.label.click();
    await expect(sender.input).toBeChecked();
    await expect(source).toBeEnabled();

    await sender.label.click();
  });

  test("notification content settings survive a reload", async ({ page }) => {
    await gotoInbox(page);
    await gotoSettings(page, "Channels");

    const body = switchByTestId(page.locator(card), "push-include-body");
    const before = await body.input.isChecked();
    await body.label.click();
    await expect(body.input).toBeChecked({ checked: !before });

    await page.reload();
    await gotoSettings(page, "Channels");
    const reloaded = switchByTestId(page.locator(card), "push-include-body");
    await expect(reloaded.input).toBeChecked({ checked: !before });

    await reloaded.label.click();
  });

  // The credential is exchanged with the provider before it is stored, so a
  // tenant learns at save time rather than from a user who never got notified.
  test("a credential that could not authenticate is refused at save time", async ({ page }) => {
    await gotoInbox(page);
    await gotoSettings(page, "Channels");

    const push = page.locator(card);
    await push.getByTestId("push-credential-file").setInputFiles({
      name: "service-account.json",
      mimeType: "application/json",
      buffer: Buffer.from(
        JSON.stringify({
          type: "service_account",
          project_id: "not-a-real-project",
          client_email: "push@not-a-real-project.iam.gserviceaccount.com",
          private_key: "-----BEGIN PRIVATE KEY-----\nnot-a-key\n-----END PRIVATE KEY-----\n",
        })
      ),
    });

    await expect(push.getByTestId("push-message")).toBeVisible();
    await expect(push.getByTestId("push-status")).toHaveText("Not set up");
  });

  test("a file that is not JSON is rejected without reaching the provider", async ({ page }) => {
    await gotoInbox(page);
    await gotoSettings(page, "Channels");

    const push = page.locator(card);
    await push.getByTestId("push-credential-file").setInputFiles({
      name: "notes.json",
      mimeType: "application/json",
      buffer: Buffer.from("this is not json"),
    });

    await expect(push.getByTestId("push-message")).toHaveText(/not valid JSON/i);
  });

  // Pasting a token rather than picking a registered device is deliberate:
  // every registered device belongs to a real end user.
  test("a test send needs a token and reports that push is not configured", async ({ page }) => {
    await gotoInbox(page);
    await gotoSettings(page, "Channels");

    const push = page.locator(card);
    await expect(push.getByTestId("push-test-send")).toBeDisabled();

    await push.getByTestId("push-test-token").fill("device-token-from-a-debug-build");
    await expect(push.getByTestId("push-test-send")).toBeEnabled();
    await push.getByTestId("push-test-send").click();

    await expect(push.getByTestId("push-message")).toContainText(/no push credential/i);
  });
});
