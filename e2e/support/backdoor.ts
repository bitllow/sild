import type { APIRequestContext } from "@playwright/test";
import { expect } from "@playwright/test";

// The ONLY non-UI actions in the suite. Both correspond to states the inbox has
// no control for, so there is genuinely no app-driven path:
//
//  1. Closing an ASSIGNMENT — the "Close conversation" button closes the
//     conversation, not its assignment, and nothing in the inbox closes an
//     assignment. The "Closed" queue filters on assignment status, so reaching
//     it requires the admin API. (Worth revisiting as a product gap.)
//  2. Creating a WEBHOOK — the Webhooks tab only lists / toggles / deletes;
//     there is no "add endpoint" control.
//
// `admin` is the authenticated agent request context (carries the session cookie
// via storageState), i.e. the same credentials the inbox uses.

export async function closeAssignmentForConversation(
  admin: APIRequestContext,
  conversationId: string
): Promise<void> {
  const conv = (await (await admin.get(`/v1/conversations/${conversationId}`)).json()) as {
    assignment?: { id: string };
  };
  expect(conv.assignment?.id, "conversation has an assignment").toBeTruthy();
  const res = await admin.post(`/v1/admin/assignments/${conv.assignment!.id}/close`);
  expect(res.ok(), "close assignment").toBeTruthy();
}

export async function createWebhook(admin: APIRequestContext, url: string, events: string[]): Promise<void> {
  const res = await admin.post("/v1/admin/webhooks", { data: { url, events } });
  expect(res.ok(), "create webhook").toBeTruthy();
}

export async function deleteAllWebhooks(admin: APIRequestContext): Promise<void> {
  const list = (await (await admin.get("/v1/admin/webhooks")).json()) as { id: string }[];
  for (const w of list) await admin.delete(`/v1/admin/webhooks/${w.id}`);
}
