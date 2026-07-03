import { test, expect } from "../../fixtures";
import { uid } from "../../support/env";
import {
  seedDemoSession,
  openDemo,
  openWidgetPanel,
  widgetNewConversation,
  widgetSend,
} from "../../support/widget";

// A 1x1 transparent PNG.
const PNG = Buffer.from(
  "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg==",
  "base64"
);

async function openDraft(page: import("@playwright/test").Page, context: import("@playwright/test").BrowserContext, tag: string) {
  await seedDemoSession(context, { mode: "user", userId: uid(tag) });
  await openDemo(page);
  await openWidgetPanel(page);
  await widgetNewConversation(page).click();
}

const fileInput = (page: import("@playwright/test").Page) => page.locator('input[type="file"]');

test.describe("widget attachments", () => {
  test("a non-image file shows a pending chip that can be removed, then sends", async ({ page, context }) => {
    await openDraft(page, context, "file");
    const name = `note-${uid("f")}.txt`;

    await fileInput(page).setInputFiles({ name, mimeType: "text/plain", buffer: Buffer.from("hello world") });
    // Pending chip appears once the upload resolves.
    await expect(page.getByText(name)).toBeVisible();

    // Remove clears it.
    await page.getByRole("button", { name: "Remove" }).click();
    await expect(page.getByText(name)).toHaveCount(0);

    // Re-attach and send → it renders in the transcript.
    await fileInput(page).setInputFiles({ name, mimeType: "text/plain", buffer: Buffer.from("hello world") });
    await expect(page.getByText(name)).toBeVisible();
    await widgetSend(page).click();
    await expect(page.getByText(name)).toBeVisible();
  });

  test("an image attachment renders inline after sending", async ({ page, context }) => {
    await openDraft(page, context, "img");
    await fileInput(page).setInputFiles({ name: "pixel.png", mimeType: "image/png", buffer: PNG });
    // Wait for the upload to queue, then send.
    await expect(widgetSend(page)).toBeEnabled();
    await widgetSend(page).click();
    await expect(page.locator("img.att-img")).toBeVisible();
  });
});
