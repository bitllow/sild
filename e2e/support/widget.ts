import type { Page, Locator, BrowserContext, Browser } from "@playwright/test";
import { BACKEND_URL } from "./env";

export type WidgetMode = "user" | "guest";

// The demo host page (web/public/demo.html, served at /sild-demo) reads these
// localStorage keys BEFORE booting the widget, so we seed them via an init
// script that runs before any page script.
export async function seedDemoSession(
  context: BrowserContext,
  opts: { mode: WidgetMode; userId: string; metadata?: Record<string, unknown> }
): Promise<void> {
  const meta = JSON.stringify(opts.metadata ?? { name: opts.userId }, null, 2);
  await context.addInitScript(
    ([mode, userId, metaText]) => {
      localStorage.setItem("sild_demo_mode", mode);
      localStorage.setItem("sild_demo_uid", userId);
      localStorage.setItem("sild_demo_meta", metaText);
    },
    [opts.mode, opts.userId, meta] as const
  );
}

// Open the demo host page and return the widget root element. Playwright
// locators pierce the open shadow DOM automatically, so we can query widget
// internals by role/label/text off the returned page or via the helpers below.
export async function openDemo(page: Page): Promise<void> {
  await page.goto(`${BACKEND_URL}/sild-demo`);
  await widgetLauncherEl(page).waitFor({ state: "visible" });
}

// ── Widget element locators (pierce shadow DOM) ─────────────────────────────
// The launcher's and panel's accessible names are themselves translated, so a
// widget running in another locale is reached by class instead.
export const widgetLauncherEl = (page: Page): Locator => page.locator("button.launcher");
export const widgetPanelEl = (page: Page): Locator => page.locator("div.panel");
export const widgetHomeCta = (page: Page): Locator => widgetPanelEl(page).locator(".card h2");
export const widgetHomeNew = (page: Page): Locator => widgetPanelEl(page).locator(".card .btn");
export const widgetComposer = (page: Page): Locator => page.getByPlaceholder(/Write a message/);
export const widgetSend = (page: Page): Locator => page.getByLabel("Send");
export const widgetAttach = (page: Page): Locator => page.getByLabel("Attach a file");
export const widgetBack = (page: Page): Locator => page.getByLabel("Back");
export const widgetNewConversation = (page: Page): Locator =>
  page.getByRole("button", { name: /New conversation/ });
export const widgetClosedBanner = (page: Page): Locator =>
  page.getByText("This conversation is closed.");

// A widget message bubble by its text (out = user-sent, in = agent reply). The
// bubble text lives inside the shadow DOM; getByText pierces it.
export const widgetBubble = (page: Page, text: string | RegExp): Locator => page.getByText(text);

// Open the launcher panel (first open triggers client.start()).
export async function openWidgetPanel(page: Page): Promise<void> {
  await widgetLauncherEl(page).click();
  await widgetPanelEl(page).waitFor({ state: "visible" });
}

// Spin up an isolated end-user widget in its own browser context (a second
// "tab"/browser) — used by the cross-surface flows that drive the widget and the
// inbox simultaneously against the one shared backend.
export async function createWidgetPage(
  browser: Browser,
  opts: { mode: WidgetMode; userId: string; metadata?: Record<string, unknown>; locale?: string }
): Promise<{ context: BrowserContext; page: Page }> {
  // The demo host names no locale, so the widget negotiates the device's — which
  // is what the context locale emulates.
  const context = await browser.newContext({ baseURL: BACKEND_URL, locale: opts.locale });
  await seedDemoSession(context, opts);
  const page = await context.newPage();
  await openDemo(page);
  return { context, page };
}
