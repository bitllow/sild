import { test, expect, type Page } from "@playwright/test";
import { gotoInbox } from "../../support/inbox";

// The peer-conversations surface in the inbox: nav access, the derived list, the
// role-filter autocomplete, and the details panel. Read-only against the seeded
// peer conversations (backend devSeed). Seeded refs trip_8830 / trip_7788 are
// unique to the seed, so they stay stable under parallel runs.

const openPeer = async (page: Page) => {
  await gotoInbox(page);
  await page.getByRole("button", { name: "Peer conversations" }).click();
  await expect(page.getByRole("heading", { name: "Peer conversations" })).toBeVisible();
};

// The seeded rider↔driver "lost item" chat, uniquely identified.
const seededRow = (page: Page, ref: string) =>
  page.getByTestId("peer-row").filter({ hasText: ref });

test("peer nav opens the surface with the seeded direct chats", async ({ page }) => {
  await openPeer(page);
  await expect(seededRow(page, "trip_8830")).toBeVisible(); // rider↔rider shared ride
  // The lost-item chat was stepped into during seeding → "You joined" marker.
  await expect(seededRow(page, "trip_7788").getByText("You joined")).toBeVisible();
});

test("opening a peer chat shows participants + role, no claim/close", async ({ page }) => {
  await openPeer(page);
  await seededRow(page, "trip_7788").click();

  // Header shows participants + reference; there is no claim/close control.
  const header = page.getByTestId("peer-header");
  await expect(header).toContainText("trip_7788");
  await expect(header.getByRole("button", { name: "Claim" })).toHaveCount(0);
  await expect(header.getByRole("button", { name: "Close conversation" })).toHaveCount(0);

  // Details panel lists both parties with roles + metadata (scoped to the panel).
  const panel = page.getByTestId("peer-panel");
  await expect(panel.getByText("Pille Saar")).toBeVisible();
  await expect(panel.getByText("Andres Laan")).toBeVisible();
  await expect(panel.getByText("Black hatchback · 118 TRE")).toBeVisible();
});

test("role autocomplete filters the list by a derived role", async ({ page }) => {
  await openPeer(page);
  // The rider↔rider chat is present before filtering.
  await expect(seededRow(page, "trip_8830")).toBeVisible();

  await page.getByTestId("peer-search").click();
  const driver = page.getByTestId("peer-role-suggestion").filter({ hasText: "driver" });
  await expect(driver).toBeVisible();
  await driver.click();

  // A removable role pill appears; a rider↔driver chat stays, the rider↔rider one drops.
  await expect(page.getByText("role: driver")).toBeVisible();
  await expect(seededRow(page, "trip_7788")).toBeVisible();
  await expect(seededRow(page, "trip_8830")).toHaveCount(0);
});
