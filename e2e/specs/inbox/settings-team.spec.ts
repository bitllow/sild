import { test, expect } from "../../fixtures";
import { gotoInbox, gotoSettings, switchAt } from "../../support/inbox";
import { ADMIN } from "../../support/env";

test.describe.configure({ mode: "serial" });

test.describe("settings · team", () => {
  test("a member's access reads as the roles they hold", async ({ page }) => {
    await gotoInbox(page);
    await gotoSettings(page, "Team");

    await expect(page.getByText(ADMIN.email)).toBeVisible();
    // The dev seed gives the first member owner plus the agent role that reaches
    // peer conversations — two chips, not one role in a dropdown.
    await expect(page.getByTestId("assignment-owner")).toBeVisible();
    await expect(page.getByTestId("assignment-agent")).toContainText("peer conversations");
  });

  test("the last owner cannot step down", async ({ page }) => {
    await gotoInbox(page);
    await gotoSettings(page, "Team");

    await page.getByTestId("assignment-owner").click();
    await page.getByTestId("remove-role").click();

    // The server refuses and the dialog stays open with the reason on the page.
    await expect(page.getByTestId("remove-role")).toBeVisible();
    await page.getByRole("button", { name: "Done" }).click();
    await expect(page.getByTestId("team-error")).toContainText("owner");

    await page.reload();
    await gotoSettings(page, "Team");
    await expect(page.getByTestId("assignment-owner")).toBeVisible();
  });

  test("peer access is a dimension of the agent role, and losing it hides the surface", async ({ page }) => {
    await gotoInbox(page);
    await gotoSettings(page, "Team");

    const peerNav = page.getByRole("button", { name: "Peer conversations", exact: true });
    await expect(peerNav).toHaveCount(1);

    await page.getByTestId("assignment-agent").click();
    const peer = switchAt(page, 0);
    await expect(peer.input).toBeChecked();
    await peer.label.click();
    await page.getByRole("button", { name: "Done" }).click();

    await expect(page.getByTestId("assignment-agent")).toContainText("no peer conversations");
    await expect(peerNav).toHaveCount(0);

    // Restore it, so the surface is there for the specs that follow.
    await page.getByTestId("assignment-agent").click();
    await switchAt(page, 0).label.click();
    await page.getByRole("button", { name: "Done" }).click();
    await expect(page.getByTestId("assignment-agent")).not.toContainText("no peer");
    await expect(peerNav).toHaveCount(1);
  });

  test("a new member joins in the role they were hired for", async ({ page }) => {
    await gotoInbox(page);
    await gotoSettings(page, "Team");

    const email = `translator-${Date.now()}@example.test`;
    await page.getByTestId("invite-member").click();
    await page.getByTestId("invite-email").fill(email);
    await page.getByTestId("invite-role-translator").click();
    await page.getByTestId("invite-send").click();

    // The invite lands in the scope dialog for the role they just got.
    await page.getByTestId("scope-locales-all").click();
    await page.getByRole("button", { name: "Done" }).click();

    const row = page.getByTestId("team-row").filter({ hasText: email });
    await expect(row.getByTestId("assignment-translator")).toContainText("all languages");
  });

  test("a second role adds to what a member can do", async ({ page }) => {
    await gotoInbox(page);
    await gotoSettings(page, "Team");

    // The signed-in owner's own row — the roster may hold other members by now.
    const mine = page.getByTestId("team-row").filter({ hasText: ADMIN.email });
    await mine.getByTestId("add-role").click();
    await page.getByTestId("add-role-translator").click();

    // Adding lands straight in the scope dialog: an unscoped role reaches nothing.
    await expect(page.getByTestId("scope-locales-all")).toBeVisible();
    await page.getByTestId("scope-projects-all").click();
    await page.getByTestId("scope-locales-all").click();
    await page.getByRole("button", { name: "Done" }).click();

    await expect(mine.getByTestId("assignment-translator")).toContainText("all projects");

    await page.reload();
    await gotoSettings(page, "Team");
    await expect(mine.getByTestId("assignment-translator")).toContainText("all languages");

    // And it takes nothing away: the owner still owns the tenant.
    await expect(mine.getByTestId("assignment-owner")).toBeVisible();

    // Put the roster back as the other specs expect it.
    await mine.getByTestId("assignment-translator").click();
    await page.getByTestId("remove-role").click();
    await expect(mine.getByTestId("assignment-translator")).toHaveCount(0);
  });
});
