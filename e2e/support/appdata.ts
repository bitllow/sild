import type { Browser, BrowserContext, Page } from "@playwright/test";
import { expect } from "@playwright/test";
import { uid } from "./env";
import {
  createWidgetPage,
  openWidgetPanel,
  widgetNewConversation,
  widgetComposer,
  widgetSend,
  widgetBubble,
} from "./widget";

// App-driven test data: conversations are created by DRIVING THE REAL WIDGET
// (the end-user's app), exactly as a visitor would — open the launcher, start a
// conversation, type, send. No backend requests are forged. The widget's demo
// host mints its own token via the tokenProvider, so this is end-to-end.

export interface AppConversation {
  /** Unique participant name (also the token subject) — findable in the inbox. */
  name: string;
  /** The opening message body — unique, used to locate the row in the inbox. */
  body: string;
  context: BrowserContext;
  page: Page;
}

export async function createConversation(
  browser: Browser,
  opts: { body?: string; metadata?: Record<string, unknown>; mode?: "user" | "guest" } = {}
): Promise<AppConversation> {
  const name = (opts.metadata?.name as string) ?? uid("c");
  const body = opts.body ?? `message ${uid("m")}`;
  const mode = opts.mode ?? "user";
  const { context, page } = await createWidgetPage(browser, {
    mode,
    userId: name,
    metadata: { name, ...(opts.metadata || {}) },
  });
  await openWidgetPanel(page);
  if (mode === "user") await widgetNewConversation(page).click();
  await widgetComposer(page).fill(body);
  await widgetSend(page).click();
  await expect(widgetBubble(page, body)).toBeVisible();
  return { name, body, context, page };
}
