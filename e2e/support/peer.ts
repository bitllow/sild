import { expect, type APIRequestContext, type Browser, type Locator, type Page } from "@playwright/test";
import { gotoInbox } from "./inbox";
import { createWidgetPage, widgetComposer, widgetSend, widgetBubble } from "./widget";
import { expectRealtimeVisible } from "./realtime";
import { uid } from "./env";

// riderPostsAndAgentObserves spins up a widget as a fresh `rider`, opens the
// driver (peer) chat, posts a uniquely-identifiable message, then opens the inbox
// peer surface and waits for that conversation's row to appear live. Returns the
// widget context (for teardown), the rider id, the agent-side row locator, and the
// message text. Shared by the peer realtime specs, which all start this way.
export async function riderPostsAndAgentObserves(
  browser: Browser,
  page: Page,
  label: string
): Promise<{ context: Awaited<ReturnType<typeof createWidgetPage>>["context"]; widget: Page; rider: string; row: Locator; riderMsg: string }> {
  const rider = uid("rider");
  const { context, page: widget } = await createWidgetPage(browser, {
    mode: "user",
    userId: rider,
    metadata: { name: rider },
  });
  await widget.getByRole("button", { name: "Open chat" }).click();
  await widgetComposer(widget).waitFor({ state: "visible" });
  const riderMsg = `${label} ${uid("m")}`;
  await widgetComposer(widget).fill(riderMsg);
  await widgetSend(widget).click();
  await expect(widgetBubble(widget, riderMsg)).toBeVisible();

  await gotoInbox(page);
  await page.getByRole("button", { name: "Peer conversations" }).click();
  const row = page.getByTestId("peer-row").filter({ hasText: riderMsg });
  await expectRealtimeVisible(row);

  return { context, widget, rider, row, riderMsg };
}

// closePeerConversation resolves the peer conversation for a given end-user id via
// the admin API and closes it out-of-band — a host/back-office action the inbox
// peer view has no button for, so the observer never took the close themselves.
export async function closePeerConversation(request: APIRequestContext, externalUserId: string): Promise<void> {
  const list = (await (await request.get("/v1/admin/peer-conversations")).json()) as {
    conversations: { id: string; members: { external_user_id?: string }[] }[];
  };
  const convId = list.conversations.find((c) =>
    c.members.some((m) => m.external_user_id === externalUserId)
  )?.id;
  expect(convId, "found the peer conversation to close").toBeTruthy();
  const res = await request.post(`/v1/conversations/${convId}/close`);
  expect(res.ok(), "close peer conversation").toBeTruthy();
}
