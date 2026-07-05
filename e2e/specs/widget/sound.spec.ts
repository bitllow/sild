import { test, expect } from "../../fixtures";
import { uid } from "../../support/env";
import { seedDemoSession, openDemo, openWidgetPanel, widgetNewConversation } from "../../support/widget";

const soundBtn = (page: import("@playwright/test").Page) =>
  page.getByRole("button", { name: /reply notifications/ });

// The widget's reply-notification sound toggle lives in BOTH the home header and
// the conversation (thread) header, sharing one state — so muting on Home stays
// muted when you open a thread, and vice versa.
test.describe("widget sound toggle", () => {
  test("toggles on the home header and carries the state into the thread", async ({ page, context }) => {
    await seedDemoSession(context, { mode: "user", userId: uid("wsound") });
    await openDemo(page);
    await openWidgetPanel(page);

    // Home header: on by default.
    await expect(soundBtn(page)).toBeVisible();
    await expect(soundBtn(page)).toHaveAttribute("aria-pressed", "true");

    // Mute it on Home.
    await soundBtn(page).click();
    await expect(soundBtn(page)).toHaveAttribute("aria-pressed", "false");

    // Open a conversation — the thread header's toggle reflects the same (muted) state.
    await widgetNewConversation(page).click();
    await expect(soundBtn(page)).toBeVisible();
    await expect(soundBtn(page)).toHaveAttribute("aria-pressed", "false");

    // Unmute mid-conversation from the thread header.
    await soundBtn(page).click();
    await expect(soundBtn(page)).toHaveAttribute("aria-pressed", "true");
  });
});
