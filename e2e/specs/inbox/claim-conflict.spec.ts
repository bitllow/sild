import { test, expect } from "../../fixtures";
import { uid } from "../../support/env";
import {
  gotoInbox,
  filterTab,
  openConversationByText,
  conversationIdByText,
  headerStatusPill,
  claimButton,
} from "../../support/inbox";
import { closeAssignmentForConversation } from "../../support/backdoor";

test.describe("inbox claim conflict", () => {
  // The guarded claim carries the expected status, so a claim on an assignment
  // that moved on is refused rather than silently overwriting it. Closing is the
  // one such move a single agent can reach (nothing in the inbox closes an
  // assignment — see support/backdoor).
  test("a claim on an assignment that moved on is refused", async ({ page, request, app }) => {
    const c = await app.conversation({ body: `contested ${uid("b")}` });

    await gotoInbox(page);
    await filterTab(page, "unassigned").click();
    await openConversationByText(page, c.body);
    const conversationId = await conversationIdByText(page, c.body);

    await expect(headerStatusPill(page)).toHaveAttribute("data-status", "queued");
    await claimButton(page).click();
    await expect(headerStatusPill(page)).toHaveAttribute("data-status", "assigned");

    // Re-claiming what this agent already holds is a retry, not a contest.
    const conv = (await (await request.get(`/v1/conversations/${conversationId}`)).json()) as {
      assignment?: { id: string };
    };
    expect(conv.assignment?.id, "conversation has an assignment").toBeTruthy();
    const retry = await request.patch(`/v1/assignments/${conv.assignment!.id}`, {
      data: { assignee_actor_id: "me" },
    });
    expect(retry.status(), "the holder's own retry stays successful").toBe(200);

    await closeAssignmentForConversation(request, conversationId);
    const afterClose = await request.patch(`/v1/assignments/${conv.assignment!.id}`, {
      data: { assignee_actor_id: "me" },
    });
    expect(afterClose.status(), "claiming a closed assignment must lose").toBe(409);
    expect(await afterClose.text()).toContain("assignment_already_closed");
  });
});
