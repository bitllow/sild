import type { Page, Locator } from "@playwright/test";
import { expect } from "@playwright/test";

// The inbox is a single-page app served at /. Landing there while authed shows
// the Shell (nav rail). These helpers drive it from the agent's perspective.

export async function gotoInbox(page: Page): Promise<void> {
  await page.goto("/");
  await page.getByRole("button", { name: "Inbox" }).waitFor({ state: "visible" });
}

// ── Nav rail ────────────────────────────────────────────────────────────────
export const navInbox = (page: Page): Locator => page.getByRole("button", { name: "Inbox" });
export const navSettings = (page: Page): Locator => page.getByRole("button", { name: "Settings" });
export const navSignOut = (page: Page): Locator => page.getByRole("button", { name: "Sign out" });

// ── Conversation list ────────────────────────────────────────────────────────
export const filterTab = (page: Page, key: "you" | "unassigned" | "closed" | "all"): Locator =>
  page.getByTestId(`filter-${key}`);
export const searchInput = (page: Page): Locator => page.getByPlaceholder(/Search/);
export const openCount = (page: Page): Locator => page.getByTestId("open-count");
export const conversationRow = (page: Page, id: string): Locator =>
  page.locator(`[data-conversation="${id}"]`);
export const allRows = (page: Page): Locator => page.getByTestId("conversation-row");

// Rows are located by their (unique) message preview / participant text, since
// conversations are created through the widget and identified by content.
export const rowByText = (page: Page, text: string): Locator =>
  allRows(page).filter({ hasText: text });

// Open a specific conversation by id (must be visible in the current list).
export async function openConversation(page: Page, id: string): Promise<void> {
  const row = conversationRow(page, id);
  await row.waitFor({ state: "visible" });
  await row.click();
}

// Open a conversation by its unique text (preview or participant name).
export async function openConversationByText(page: Page, text: string): Promise<void> {
  const row = rowByText(page, text).first();
  await row.waitFor({ state: "visible" });
  await row.click();
}

// Read a conversation's id from its row (needed only for the documented
// no-UI backdoor actions).
export async function conversationIdByText(page: Page, text: string): Promise<string> {
  const row = rowByText(page, text).first();
  await row.waitFor({ state: "visible" });
  const id = await row.getAttribute("data-conversation");
  if (!id) throw new Error(`no data-conversation on row for "${text}"`);
  return id;
}

// ── Conversation view (header + composer) ────────────────────────────────────
// Header controls are scoped to the header so they never collide with
// conversation-list rows (whose accessible names include message previews).
export const conversationHeader = (page: Page): Locator => page.getByTestId("conversation-header");
export const headerStatusPill = (page: Page): Locator =>
  conversationHeader(page).getByTestId("status-pill");
export const claimButton = (page: Page): Locator =>
  conversationHeader(page).getByRole("button", { name: "Claim", exact: true });
export const closeButton = (page: Page): Locator =>
  conversationHeader(page).getByRole("button", { name: "Close conversation" });
export const detailsToggle = (page: Page): Locator =>
  conversationHeader(page).getByRole("button", { name: "Toggle details" });

// Composer controls are scoped to the composer bar for the same reason.
const composerBar = (page: Page): Locator => page.locator(".sild-composer");
export const replyTab = (page: Page): Locator =>
  composerBar(page).getByRole("button", { name: "Reply", exact: true });
export const internalTab = (page: Page): Locator =>
  composerBar(page).getByRole("button", { name: "Internal note" });
export const composer = (page: Page): Locator => composerBar(page).locator("textarea.sild-composer__input");
export const composerSend = (page: Page): Locator =>
  composerBar(page).getByRole("button", { name: "Send", exact: true });
export const closedBanner = (page: Page): Locator => page.getByText(/This conversation is closed/);
// The composer's hidden file input (attachments). Only present in the open
// conversation view, so a page-level query is unambiguous there.
export const composerFileInput = (page: Page): Locator => page.locator('input[type="file"]');

// A rendered transcript message by its body text.
export const messageWithText = (page: Page, text: string | RegExp): Locator =>
  page.getByTestId("message").filter({ hasText: text });

// Send an agent reply (or internal note) through the composer UI.
export async function sendReply(page: Page, text: string, internal = false): Promise<void> {
  await (internal ? internalTab(page) : replyTab(page)).click();
  await composer(page).fill(text);
  await composerSend(page).click();
}

// ── Switches (the checkbox input is visually hidden; click the label) ─────────
// Returns { label, input } for the nth Switch on the page.
export function switchAt(page: Page, n = 0): { label: Locator; input: Locator } {
  const label = page.locator(".sild-switch").nth(n);
  return { label, input: label.locator('input[role="switch"]') };
}

// Toggle a switch and assert it flipped. Returns the new checked state.
export async function toggleSwitch(page: Page, n = 0): Promise<boolean> {
  const { label, input } = switchAt(page, n);
  const before = await input.isChecked();
  await label.click();
  await expect(input).toBeChecked({ checked: !before });
  return !before;
}

// ── Settings ─────────────────────────────────────────────────────────────────
export async function gotoSettings(page: Page, tab: "Channels" | "Appearance" | "API keys" | "Webhooks" | "Team"): Promise<void> {
  await navSettings(page).click();
  const tabBtn = page.getByRole("button", { name: tab, exact: true });
  await tabBtn.click();
  await expect(page.getByRole("heading", { name: "Settings" })).toBeVisible();
}
