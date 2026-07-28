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

// 3. Bulk thread history — a thread long enough to page needs hundreds of messages,
//    and the only UI path is typing them one at a time. Posts them as the agent, in
//    bounded-concurrency batches so a ten-page thread costs seconds, not minutes —
//    kept modest because SQLite serializes writers and a wide fan-out just produces
//    lock contention.
//    Order is NOT guaranteed across a batch, so callers anchor on a message they
//    created themselves (the conversation's opening one) rather than on an index.
export async function seedManyMessages(
  admin: APIRequestContext,
  conversationId: string,
  count: number,
  label: string,
  concurrency = 8
): Promise<void> {
  let next = 0;
  const post = async () => {
    for (let i = next++; i < count; i = next++) {
      const res = await admin.post(`/v1/conversations/${conversationId}/messages`, {
        data: { body: `${label} #${i}`, client_msg_id: `${label}-${i}` },
      });
      expect(res.ok(), `seed message ${i}`).toBeTruthy();
    }
  };
  await Promise.all(Array.from({ length: concurrency }, post));
}

export async function closeAssignmentForConversation(
  admin: APIRequestContext,
  conversationId: string
): Promise<void> {
  const conv = (await (await admin.get(`/v1/conversations/${conversationId}`)).json()) as {
    assignment?: { id: string };
  };
  expect(conv.assignment?.id, "conversation has an assignment").toBeTruthy();
  const res = await admin.patch(`/v1/assignments/${conv.assignment!.id}`, { data: { status: "closed" } });
  expect(res.ok(), "close assignment").toBeTruthy();
}

export async function createWebhook(admin: APIRequestContext, url: string, events: string[]): Promise<void> {
  const res = await admin.post("/v1/webhooks", { data: { url, events } });
  expect(res.ok(), "create webhook").toBeTruthy();
}

export async function deleteAllWebhooks(admin: APIRequestContext): Promise<void> {
  const list = (await (await admin.get("/v1/webhooks")).json()) as { items: { id: string }[] };
  for (const w of list.items) await admin.delete(`/v1/webhooks/${w.id}`);
}
